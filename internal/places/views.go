package places

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

// Overview is what the bare command shows: how much there is, the current home and the latest trips.
func (x *Index) Overview(latest int, today Date) *wherewasiv1.Overview {
	out := &wherewasiv1.Overview{LocatedPhotos: int32(len(x.photos))}
	if len(x.days) > 0 {
		out.FirstDay, out.LastDay = x.days[0].Proto(), x.days[len(x.days)-1].Proto()
	}
	for _, k := range x.known {
		if k.proto.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME && !today.Before(k.first.Time) && !today.After(k.last.Time) {
			out.CurrentHome = k.proto
		}
	}
	trips := x.Trips(Everything, latest, false)
	out.LatestTrips, out.TotalTrips = trips.Trips, trips.TotalTrips
	return out
}

// Day is one local day, stretch by stretch: consecutive photos in one town and neighbourhood, and at the same known
// place or none.
func (x *Index) Day(day Date) *wherewasiv1.DayTimeline {
	out := &wherewasiv1.DayTimeline{LocalDay: day.Proto()}
	if t := x.tripOn(day); t != nil {
		out.TripLink = t.proto.Link
	}
	if n, has := x.nights[day]; has {
		out.Night = n.proto
	}
	photos := x.byDay[day]
	names := placeNames(photos)
	spotsOf := photoSpots(photos, names)
	moving := inTransit(photos)
	var current []int
	flush := func() {
		if len(current) == 0 {
			return
		}
		first, last := photos[current[0]], photos[current[len(current)-1]]
		visit := &wherewasiv1.Visit{Start: first.proto.Time, End: last.proto.Time, Place: TownLevel(first.proto.Place),
			PlaceName: names[current[0]], KnownPlace: x.knownLabelAt(first), KnownPlaceKind: x.knownKindAt(first),
			PhotoCount: int32(len(current)), Travelling: moving[current[0]]}
		if visit.Travelling {
			visit.EndPlaceName = names[current[len(current)-1]]
		}
		if km, ok := x.kmFromHome(first); ok {
			visit.KmFromHome = &km
		}
		byName := map[string]*wherewasiv1.Spot{}
		for _, i := range current {
			visit.Neighbourhoods = appendNew(visit.Neighbourhoods, photos[i].proto.Place.Neighbourhood)
			for _, found := range spotsOf[i] {
				spot := byName[found.Name]
				if spot == nil {
					spot = found
					byName[found.Name] = spot
					visit.Spots = append(visit.Spots, spot)
				}
				spot.PhotoCount++
			}
		}
		out.Visits = append(out.Visits, visit)
		current = nil
	}
	for i, p := range photos {
		out.Photos = append(out.Photos, p.proto)
		if km, ok := x.kmFromHome(p); ok && (out.MaxKmFromHome == nil || km > *out.MaxKmFromHome) {
			out.MaxKmFromHome = &km
		}
		if len(current) > 0 {
			before := current[len(current)-1]
			previous := photos[before]
			changed := names[before] != names[i] || previous.proto.Place.Neighbourhood != p.proto.Place.Neighbourhood ||
				x.knownLabelAt(previous) != x.knownLabelAt(p)
			if moving[before] != moving[i] || (!moving[i] && changed) {
				flush()
			}
		}
		current = append(current, i)
	}
	flush()
	return out
}

// Days are the days with photos in a range, newest first, up to limit.
func (x *Index) Days(within Range, limit int) *wherewasiv1.Days {
	out := &wherewasiv1.Days{}
	for i := len(x.days) - 1; i >= 0; i-- {
		if !within.Contains(x.days[i]) {
			continue
		}
		out.TotalDays++
		if limit <= 0 || len(out.Days) < limit {
			out.Days = append(out.Days, x.itineraryDay(x.days[i]))
		}
	}
	return out
}

// placeFields are Apple's place fields that a place name can match, from largest to smallest.
func placeFields(p *wherewasiv1.Place) [][2]string {
	fields := [][2]string{{"country", p.Country}, {"country", p.CountryCode}, {"state", p.State}}
	for _, region := range p.Regions {
		fields = append(fields, [2]string{"region", region})
	}
	return append(fields, [2]string{"county", p.County}, [2]string{"town", p.Town}, [2]string{"neighbourhood", p.Neighbourhood},
		[2]string{"point of interest", p.PointOfInterest}, [2]string{"water", p.Water}, [2]string{"island", p.Island})
}

// sameName compares place names ignoring case and accents, so "Malaga" finds "Málaga" and "Cote d'Azur" finds "Côte d'Azur".
func sameName(a, b string) bool { return strings.EqualFold(withoutAccents(a), withoutAccents(b)) }

func withoutAccents(s string) string {
	decomposed := norm.NFD.String(s)
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, decomposed)
}

// InPlace groups the days on which a photo's place has a field equal to the name asked for, ignoring case and
// accents, into stays: consecutive days, bridging the same gaps a trip may. Newest first. When no field anywhere is
// equal, it takes fields that contain the name's words, so "Kruger" finds "Kruger National Park".
func (x *Index) InPlace(name string, within Range, limit int) *wherewasiv1.PlaceDays {
	if out := x.inPlace(name, within, limit, sameName); out.TotalDays > 0 {
		return out
	}
	return x.inPlace(name, within, limit, containsWords)
}

func containsWords(field, name string) bool {
	return strings.Contains(" "+strings.ToLower(withoutAccents(field))+" ", " "+strings.ToLower(withoutAccents(name))+" ")
}

func (x *Index) inPlace(name string, within Range, limit int, matches func(field, name string) bool) *wherewasiv1.PlaceDays {
	out := &wherewasiv1.PlaceDays{PlaceAskedFor: name}
	var stay *wherewasiv1.PlaceStay
	towns := map[string]bool{}
	finish := func() {
		if stay != nil {
			out.TotalStays++
			if limit <= 0 || len(out.Stays) < limit {
				out.Stays = append(out.Stays, stay)
			}
		}
		stay, towns = nil, map[string]bool{}
	}
	for i := len(x.days) - 1; i >= 0; i-- { // newest first, so a stay grows backwards
		day := x.days[i]
		if !within.Contains(day) {
			continue
		}
		var dayPhotos []photo
		field, value := "", ""
		for _, p := range x.byDay[day] {
			for _, f := range placeFields(p.proto.Place) {
				if f[1] != "" && matches(f[1], name) {
					dayPhotos = append(dayPhotos, p)
					if field == "" {
						field, value = f[0], f[1]
					}
					break
				}
			}
		}
		if len(dayPhotos) == 0 {
			continue
		}
		out.TotalDays++
		if stay != nil && day.DaysUntil(DateFromProto(stay.FirstDay))-1 > MaxNightsWithoutPhotosInTrip {
			finish()
		}
		if stay == nil {
			stay = &wherewasiv1.PlaceStay{LastDay: day.Proto(), MatchedField: field, MatchedValue: value}
			if t := x.tripOn(day); t != nil {
				stay.TripLink = t.proto.Link
			}
		}
		stay.FirstDay = day.Proto()
		stay.DaysWithPhotos++
		stay.PhotoCount += int32(len(dayPhotos))
		var dayTowns []*wherewasiv1.Place
		for _, p := range dayPhotos {
			if key := townKey(p.proto.Place); !towns[key] {
				towns[key] = true
				dayTowns = append(dayTowns, TownLevel(p.proto.Place))
			}
		}
		stay.TownsInOrder = append(dayTowns, stay.TownsInOrder...)
	}
	finish()
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Years counts every night of every year with photos, up to today, by where it was spent.
func (x *Index) Years(today Date) *wherewasiv1.Years {
	out := &wherewasiv1.Years{}
	if len(x.days) == 0 {
		return out
	}
	byYear := map[int]*wherewasiv1.Year{}
	countries := map[int]map[string]int32{}
	start := Date{time.Date(x.days[0].Year(), 1, 1, 0, 0, 0, 0, time.UTC)}
	for day := start; !day.After(today.Time); day = day.Next() {
		year := byYear[day.Year()]
		if year == nil {
			year = &wherewasiv1.Year{Year: int32(day.Year())}
			byYear[day.Year()], countries[day.Year()] = year, map[string]int32{}
		}
		n, has := x.nights[day]
		switch {
		case !has:
			year.NightsWithoutPhotos++
		case !n.proto.HomeKnown:
			year.NightsHomeUnknown++
		case n.proto.Away:
			year.NightsAway++
			if n.proto.Travelling {
				countries[day.Year()]["travelling"]++ // as in a trip's route: a night on the move has no country
			} else {
				countries[day.Year()][n.proto.Place.Country]++
			}
		case x.atHomeOrFamily(n.photo) && x.knownPlaceAt(n.photo).proto.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_FAMILY:
			year.NightsWithFamily++
		case x.atHomeOrFamily(n.photo):
			year.NightsAtHome++
		default:
			year.NightsNearHome++
		}
	}
	for y := start.Year(); y <= today.Year(); y++ {
		year := byYear[y]
		for country, nights := range countries[y] {
			year.AwayByCountry = append(year.AwayByCountry, &wherewasiv1.CountryNights{Country: country, Nights: nights})
		}
		sort.Slice(year.AwayByCountry, func(i, j int) bool { return year.AwayByCountry[i].Nights > year.AwayByCountry[j].Nights })
		out.Years = append(out.Years, year)
	}
	return out
}

// inTransit marks the photos taken on the move: reached from the photo a minute or more before, and left for the photo
// a minute or more after, faster than InTransitKmPerHour, as from a plane or a fast train. Measuring over a minute
// keeps a burst of photos sharing one position on the move. The photo at the arrival gate is not on the move.
func inTransit(photos []photo) []bool {
	kmPerHour := func(a, b photo) float64 {
		return kmBetween(a.proto.Location, b.proto.Location) / b.local.Sub(a.local).Hours()
	}
	moving := make([]bool, len(photos))
	for i := range photos {
		before, after := -1, -1
		for j := i - 1; j >= 0; j-- {
			if photos[i].local.Sub(photos[j].local) >= time.Minute {
				before = j
				break
			}
		}
		for k := i + 1; k < len(photos); k++ {
			if photos[k].local.Sub(photos[i].local) >= time.Minute {
				after = k
				break
			}
		}
		moving[i] = before >= 0 && after >= 0 &&
			kmPerHour(photos[before], photos[i]) > InTransitKmPerHour && kmPerHour(photos[i], photos[after]) > InTransitKmPerHour
	}
	return moving
}
