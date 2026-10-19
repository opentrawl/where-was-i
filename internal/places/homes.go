package places

import (
	"fmt"
	"strings"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

// ProposedHome keeps the display evidence beside the KnownPlace that can be pasted into the user's file.
type ProposedHome struct {
	Home       *wherewasiv1.KnownPlace
	Place      *wherewasiv1.Place
	Nights     int32
	PhotoCount int32
}

type Homes struct {
	Proposals     []ProposedHome
	UnknownNights int32
}

func (homes Homes) KnownPlaces() *wherewasiv1.KnownPlaces {
	out := &wherewasiv1.KnownPlaces{}
	for _, proposal := range homes.Proposals {
		out.Places = append(out.Places, proposal.Home)
	}
	return out
}

type homeNight struct {
	day   Date
	night *night
	group int
}

type homeGroup struct {
	location *wherewasiv1.LatLng
}

// Homes proposes homes only for dates whose nights have no known home. Around each observed night, the most
// frequent location in a six-month window is a candidate only if its nights span at least three months. This
// permits holidays inside a home period while keeping short stays from becoming homes.
func (x *Index) Homes() Homes {
	out := Homes{}
	var nights []homeNight
	var groups []homeGroup
	for _, day := range x.days {
		n := x.nights[day]
		if n == nil || n.proto.HomeKnown {
			continue
		}
		out.UnknownNights++
		if n.proto.Travelling {
			continue
		}
		group, nearest := -1, KnownPlaceRadiusKm
		for i, candidate := range groups {
			if km := kmBetween(candidate.location, n.photo.proto.Location); km <= nearest {
				group, nearest = i, km
			}
		}
		if group < 0 {
			group = len(groups)
			groups = append(groups, homeGroup{location: n.photo.proto.Location})
		}
		nights = append(nights, homeNight{day, n, group})
	}
	if len(nights) == 0 {
		return out
	}

	type period struct {
		first, last Date
		group       int
	}
	var periods []period
	for i, current := range nights {
		counts := make([]int, len(groups))
		first := make([]Date, len(groups))
		last := make([]Date, len(groups))
		for _, other := range nights {
			if dayOffset := current.day.DaysUntil(other.day); dayOffset < -HomeWindowDays/2 || dayOffset >= HomeWindowDays/2 || x.homeBetween(current.day, other.day) {
				continue
			}
			counts[other.group]++
			if first[other.group].IsZero() {
				first[other.group] = other.day
			}
			last[other.group] = other.day
		}
		winner, most := -1, 0
		for group, count := range counts {
			if count >= HomeMinimumNights && first[group].DaysUntil(last[group]) >= HomeMinimumSpanDays && count > most {
				winner, most = group, count
			}
		}
		if winner < 0 {
			continue
		}
		if len(periods) > 0 && periods[len(periods)-1].group == winner && !x.homeBetween(periods[len(periods)-1].last, current.day) {
			periods[len(periods)-1].last = current.day
			continue
		}
		start := current.day
		if len(periods) == 0 || x.homeBetween(periods[len(periods)-1].last, current.day) {
			// A first home covers all unknown nights before its evidence, including early trips.
			for _, day := range x.days {
				if day.After(current.day.Time) {
					break
				}
				if n := x.nights[day]; n != nil && !n.proto.HomeKnown && !x.homeBetween(day, current.day) {
					start = day
					break
				}
			}
		} else {
			// The change is the first night at the new home, rather than the middle of the window that detected it.
			for j := i; j < len(nights) && !x.homeBetween(current.day, nights[j].day); j++ {
				if nights[j].group == winner {
					start = nights[j].day
					break
				}
			}
		}
		if len(periods) > 0 && !start.After(periods[len(periods)-1].first.Time) {
			continue
		}
		periods = append(periods, period{start, current.day, winner})
	}
	// A window winner is only a candidate. Its own period must have sustained evidence too: nearby window
	// boundaries can otherwise produce a few days at a place that was frequent months earlier or later.
	var sustained []period
	for i, candidate := range periods {
		end := x.today
		if i+1 < len(periods) {
			end = periods[i+1].first
		}
		count := 0
		var first, last Date
		for _, n := range nights {
			if n.group != candidate.group || n.day.Before(candidate.first.Time) || !n.day.Before(end.Time) || x.homeBetween(candidate.first, n.day) {
				continue
			}
			if count == 0 {
				first = n.day
			}
			last = n.day
			count++
		}
		if count < HomeMinimumNights || first.DaysUntil(last) < HomeMinimumSpanDays {
			continue
		}
		if len(sustained) > 0 && sustained[len(sustained)-1].group == candidate.group && !x.homeBetween(sustained[len(sustained)-1].last, candidate.first) {
			sustained[len(sustained)-1].last = candidate.last
			continue
		}
		if len(sustained) == 0 || x.homeBetween(sustained[len(sustained)-1].last, candidate.first) {
			for _, day := range x.days {
				if day.After(candidate.first.Time) {
					break
				}
				if n := x.nights[day]; n != nil && !n.proto.HomeKnown && !x.homeBetween(day, candidate.first) {
					candidate.first = day
					break
				}
			}
		}
		sustained = append(sustained, candidate)
	}
	periods = sustained
	for i, period := range periods {
		end := x.today
		bounded := false
		for _, known := range x.known {
			if known.proto.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME && known.first.After(period.first.Time) && !known.first.After(end.Time) {
				end = known.first
				bounded = true
			}
		}
		if i+1 < len(periods) && periods[i+1].first.Before(end.Time) {
			end = periods[i+1].first
			bounded = true
		}
		photos := map[*wherewasiv1.LocatedPhoto]bool{}
		var count int32
		for _, n := range nights {
			if n.group != period.group || n.day.Before(period.first.Time) || !n.day.Before(end.Time) {
				continue
			}
			count++
			// The same photo can support the previous night and its own evening. Average each photo once.
			photos[n.night.photo.proto] = true
			evening := x.byDay[n.day]
			for _, p := range evening[len(evening)-1:] {
				if kmBetween(groups[period.group].location, p.proto.Location) <= KnownPlaceRadiusKm {
					photos[p.proto] = true
				}
			}
			morning := x.byDay[n.day.Next()]
			if len(morning) > 1 {
				morning = morning[:1]
			}
			for _, p := range morning {
				if p.local.Hour() < MorningEndsAt && kmBetween(groups[period.group].location, p.proto.Location) <= KnownPlaceRadiusKm {
					photos[p.proto] = true
				}
			}
		}
		if count == 0 {
			continue
		}
		var latitude, longitude float64
		byName := map[string]int{}
		var place *wherewasiv1.Place
		most := 0
		// Preserve photo order for deterministic sums and Apple name ties.
		for _, p := range x.photos {
			if !photos[p.proto] {
				continue
			}
			latitude += p.proto.Location.Latitude
			longitude += p.proto.Location.Longitude
			key := townKey(p.proto.Place) + "\x00" + p.proto.Place.Neighbourhood
			byName[key]++
			if byName[key] > most {
				place, most = p.proto.Place, byName[key]
			}
		}
		label := strings.Join(nonEmpty(place.GetTown(), place.GetNeighbourhood()), " · ")
		if label == "" {
			label = firstNonEmptyString(place.GetCounty(), place.GetState(), place.GetCountry(), "Earlier home")
		}
		home := &wherewasiv1.KnownPlace{Label: label, Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
			Location: &wherewasiv1.LatLng{Latitude: latitude / float64(len(photos)), Longitude: longitude / float64(len(photos))}, ValidFrom: period.first.Proto(),
			Evidence: fmt.Sprintf("Proposed from %d nights and %d photos within %.1f km; most nights in a %d-day window, at least %d nights spanning %d days. Confirm the home and move dates.", count, len(photos), KnownPlaceRadiusKm, HomeWindowDays, HomeMinimumNights, HomeMinimumSpanDays)}
		if bounded {
			home.ValidTo = Date{end.AddDate(0, 0, -1)}.Proto()
		}
		out.Proposals = append(out.Proposals, ProposedHome{home, place, count, int32(len(photos))})
	}
	return out
}

// homeBetween keeps proposal evidence on the same side of any already known home period.
func (x *Index) homeBetween(a, b Date) bool {
	if a.After(b.Time) {
		a, b = b, a
	}
	for _, known := range x.known {
		if known.proto.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME && !known.first.After(b.Time) && !known.last.Before(a.Time) {
			return true
		}
	}
	return false
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
