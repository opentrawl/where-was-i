package places

import (
	"fmt"
	"sort"
	"time"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

// ProposedWorkplace keeps the display evidence beside the entry the user can confirm.
type ProposedWorkplace struct {
	Work       *wherewasiv1.KnownPlace
	Place      *wherewasiv1.Place
	Days       int32
	PhotoCount int32
}

type workplaceGroup struct {
	photos              []photo
	latitude, longitude float64
}

func (group workplaceGroup) location() *wherewasiv1.LatLng {
	return &wherewasiv1.LatLng{Latitude: group.latitude / float64(len(group.photos)), Longitude: group.longitude / float64(len(group.photos))}
}

// Workplaces proposes repeated weekday daytime locations, excluding known places and proposed homes. Dates bound
// the observed evidence; photos cannot establish the actual first or last day of employment.
func (x *Index) Workplaces(proposedHomes Homes) []ProposedWorkplace {
	var groups []workplaceGroup
	for _, p := range x.photos {
		weekday, hour := p.local.Weekday(), p.local.Hour()
		if weekday == time.Saturday || weekday == time.Sunday || hour < WorkplaceDayStartsAt || hour >= WorkplaceDayEndsAt || x.workplaceExcluded(p, proposedHomes) {
			continue
		}
		nearestGroup, nearestKm := -1, WorkplaceRadiusKm
		for i, group := range groups {
			if distance := kmBetween(p.proto.Location, group.location()); distance <= nearestKm {
				nearestGroup, nearestKm = i, distance
			}
		}
		if nearestGroup == -1 {
			nearestGroup = len(groups)
			groups = append(groups, workplaceGroup{})
		}
		group := &groups[nearestGroup]
		group.photos = append(group.photos, p)
		group.latitude += p.proto.Location.Latitude
		group.longitude += p.proto.Location.Longitude
	}
	var proposals []ProposedWorkplace
	for _, group := range groups {
		start := 0
		for end := 1; end <= len(group.photos); end++ {
			if end < len(group.photos) && group.photos[end-1].day().DaysUntil(group.photos[end].day()) <= WorkplaceMaximumGapDays {
				continue
			}
			if proposal, ok := proposeWorkplace(group.photos[start:end]); ok {
				proposals = append(proposals, proposal)
			}
			start = end
		}
	}
	sort.SliceStable(proposals, func(i, j int) bool {
		return DateFromProto(proposals[i].Work.ValidFrom).Before(DateFromProto(proposals[j].Work.ValidFrom).Time)
	})
	return proposals
}

func (x *Index) workplaceExcluded(p photo, homes Homes) bool {
	day := p.day()
	homeKnown := false
	for _, known := range x.known {
		if !kindIn(known.proto.Kind, []wherewasiv1.KnownPlaceKind{wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME, wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_WORK}) || day.Before(known.first.Time) || day.After(known.last.Time) {
			continue
		}
		if known.proto.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME {
			homeKnown = true
		}
		if kmBetween(p.proto.Location, known.proto.Location) <= KnownPlaceRadiusKm {
			return true
		}
	}
	for _, proposal := range homes.Proposals {
		home := proposal.Home
		if day.Before(dateFromProto(home.ValidFrom, earliestDate).Time) || day.After(dateFromProto(home.ValidTo, latestDate).Time) {
			continue
		}
		homeKnown = true
		if kmBetween(p.proto.Location, home.Location) <= KnownPlaceRadiusKm {
			return true
		}
	}
	return !homeKnown
}

func proposeWorkplace(photos []photo) (ProposedWorkplace, bool) {
	photos = sustainedWorkplacePhotos(photos)
	if len(photos) == 0 {
		return ProposedWorkplace{}, false
	}
	days := map[Date]bool{}
	weeks := map[[2]int]bool{}
	var latitude, longitude float64
	evidence := make([]*wherewasiv1.LocatedPhoto, 0, len(photos))
	for _, p := range photos {
		days[p.day()] = true
		year, week := p.local.ISOWeek()
		weeks[[2]int{year, week}] = true
		latitude += p.proto.Location.Latitude
		longitude += p.proto.Location.Longitude
		evidence = append(evidence, p.proto)
	}
	first, last := photos[0].day(), photos[len(photos)-1].day()
	location := &wherewasiv1.LatLng{Latitude: latitude / float64(len(photos)), Longitude: longitude / float64(len(photos))}
	place := proposalPlace(evidence, location)
	work := &wherewasiv1.KnownPlace{Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_WORK,
		Label: proposalLabel(place, "Possible workplace"), Location: location, ValidFrom: first.Proto(), ValidTo: last.Proto(),
		Evidence: fmt.Sprintf("Proposed from %d weekdays and %d photos in %d calendar weeks; weekday photos from %02d:00 to %02d:00, grouped within %.0f metres, at least %d days in %d weeks spanning %d days within a %d-day window, with no gap over %d days. Dates are the first and last supporting photos. Confirm whether this was work and the dates.", len(days), len(photos), len(weeks), WorkplaceDayStartsAt, WorkplaceDayEndsAt, WorkplaceRadiusKm*1000, WorkplaceMinimumDays, WorkplaceMinimumWeeks, WorkplaceMinimumSpanDays, WorkplaceWindowDays, WorkplaceMaximumGapDays)}
	return ProposedWorkplace{Work: work, Place: place, Days: int32(len(days)), PhotoCount: int32(len(photos))}, true
}

// sustainedWorkplacePhotos bounds a period to windows with repeated visits, so a few scattered visits cannot
// establish a workplace or stretch its dates indefinitely before or after its sustained evidence.
func sustainedWorkplacePhotos(photos []photo) []photo {
	first, last := -1, -1
	for start := 0; start < len(photos); {
		days := map[Date]bool{}
		weeks := map[[2]int]bool{}
		end := start
		for end < len(photos) && photos[start].day().DaysUntil(photos[end].day()) < WorkplaceWindowDays {
			p := photos[end]
			days[p.day()] = true
			year, week := p.local.ISOWeek()
			weeks[[2]int{year, week}] = true
			end++
		}
		if len(days) >= WorkplaceMinimumDays && len(weeks) >= WorkplaceMinimumWeeks && photos[start].day().DaysUntil(photos[end-1].day()) >= WorkplaceMinimumSpanDays {
			if first == -1 {
				first = start
			}
			last = end
		}
		day := photos[start].day()
		for start < len(photos) && photos[start].day() == day {
			start++
		}
	}
	if first == -1 {
		return nil
	}
	return photos[first:last]
}
