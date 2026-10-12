package places

import (
	"fmt"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

// night is where you spent the night after a local day, and the photo that says so.
type night struct {
	proto *wherewasiv1.Night
	photo photo
}

// workOutNights decides, for each day with photos, where the night after it was spent. If the evening's last photo or
// the next day's first photo was at a home or family place, that is where. Otherwise the next morning's first photo
// (before noon) says where you woke up, and failing that the evening's last photo says where you ended up.
func (x *Index) workOutNights() {
	x.nights = map[Date]*night{}
	for _, day := range x.days {
		photos := x.byDay[day]
		evening := photos[len(photos)-1]
		var next *photo
		if morning := x.byDay[day.Next()]; len(morning) > 0 {
			next = &morning[0]
		}
		chosen, source := evening, wherewasiv1.NightSource_NIGHT_SOURCE_LAST_EVENING
		switch {
		case x.atHomeOrFamily(evening):
			source = wherewasiv1.NightSource_NIGHT_SOURCE_HOME_OR_FAMILY
		case next != nil && x.atHomeOrFamily(*next):
			chosen, source = *next, wherewasiv1.NightSource_NIGHT_SOURCE_HOME_OR_FAMILY
		case next != nil && next.local.Hour() < MorningEndsAt:
			chosen, source = *next, wherewasiv1.NightSource_NIGHT_SOURCE_NEXT_MORNING
		}
		n := &wherewasiv1.Night{Evening: day.Proto(), Place: TownLevel(chosen.proto.Place), KnownPlace: x.knownLabelAt(chosen),
			KnownPlaceKind: x.knownKindAt(chosen), Away: x.isAway(chosen), HomeKnown: x.homeKnown(chosen), Source: source,
			LastEveningPlace: TownLevel(evening.proto.Place)}
		if next != nil {
			n.NextMorningPlace = TownLevel(next.proto.Place)
			n.Travelling = kmBetween(evening.proto.Location, next.proto.Location) > TravellingNightKm
		}
		x.nights[day] = &night{photo: chosen, proto: n}
	}
}

type trip struct {
	proto *wherewasiv1.Trip
	left  Date
	back  Date
}

// workOutTrips finds runs of nights away. A night at or near a home or family place ends a trip; up to
// MaxNightsWithoutPhotosInTrip nights without photos may sit inside one.
func (x *Index) workOutTrips() {
	if len(x.days) == 0 {
		return
	}
	var awayEvenings []Date
	withoutPhotos := 0
	flush := func() {
		if len(awayEvenings) > 0 {
			x.trips = append(x.trips, x.newTrip(awayEvenings[0], awayEvenings[len(awayEvenings)-1].Next()))
		}
		awayEvenings, withoutPhotos = nil, 0
	}
	last := x.days[len(x.days)-1]
	for day := x.days[0]; !day.After(last.Time); day = day.Next() {
		n, has := x.nights[day]
		switch {
		case !has:
			if len(awayEvenings) > 0 {
				withoutPhotos++
				if withoutPhotos > MaxNightsWithoutPhotosInTrip {
					flush()
				}
			}
		case n.proto.Away:
			withoutPhotos = 0
			awayEvenings = append(awayEvenings, day)
		default:
			flush()
		}
	}
	flush()
}

func (x *Index) newTrip(left, back Date) *trip {
	t := &wherewasiv1.Trip{Link: "trip:" + left.Format("2006-01-02"), Left: left.Proto(), Back: back.Proto(), Nights: int32(left.DaysUntil(back))}
	for day := left; day.Before(back.Time); day = day.Next() {
		stop := &wherewasiv1.Stop{FirstEvening: day.Proto(), Nights: 1}
		n, has := x.nights[day]
		switch {
		case !has:
			t.NightsWithoutPhotos++
			stop.NoPhotos = true
		case n.proto.Travelling:
			stop.Travelling = true
		default:
			stop.Place, stop.KnownPlace, stop.KnownPlaceKind = n.proto.Place, n.proto.KnownPlace, n.proto.KnownPlaceKind
			t.Countries = appendNew(t.Countries, n.proto.Place.Country)
		}
		if has && n.proto.Source == wherewasiv1.NightSource_NIGHT_SOURCE_NEXT_MORNING {
			t.NightsConfirmedByMorning++
		}
		if stops := t.Stops; len(stops) > 0 && sameStop(stops[len(stops)-1], stop) {
			stops[len(stops)-1].Nights++
		} else {
			t.Stops = append(t.Stops, stop)
		}
	}
	for day := left; !day.After(back.Time); day = day.Next() {
		t.PhotoCount += int32(len(x.byDay[day]))
		for _, title := range x.appleTrips[day] {
			if day.Before(back.Time) {
				t.AppleTripTitles = appendNew(t.AppleTripTitles, title)
			}
		}
	}
	return &trip{proto: t, left: left, back: back}
}

func sameStop(a, b *wherewasiv1.Stop) bool {
	if a.Travelling || b.Travelling || a.NoPhotos || b.NoPhotos {
		return a.Travelling == b.Travelling && a.NoPhotos == b.NoPhotos
	}
	return townKey(a.Place) == townKey(b.Place)
}

// tripOn is the trip a day belongs to, from the day you left to the day you got back.
func (x *Index) tripOn(day Date) *trip {
	for _, t := range x.trips {
		if !day.Before(t.left.Time) && !day.After(t.back.Time) {
			return t
		}
	}
	return nil
}

// FindTrip resolves "trip:2024-05-03", or any day inside a trip.
func (x *Index) FindTrip(day Date) (*wherewasiv1.Itinerary, error) {
	t := x.tripOn(day)
	if t == nil {
		return nil, fmt.Errorf("no trip on %s", day.Format("2 Jan 2006"))
	}
	itinerary := &wherewasiv1.Itinerary{Trip: t.proto}
	for d := t.left; !d.After(t.back.Time); d = d.Next() {
		row := x.itineraryDay(d)
		if !d.Before(t.back.Time) {
			row.Night = nil
		}
		itinerary.Days = append(itinerary.Days, row)
	}
	return itinerary, nil
}

func (x *Index) itineraryDay(day Date) *wherewasiv1.ItineraryDay {
	row := &wherewasiv1.ItineraryDay{LocalDay: day.Proto()}
	seenOffsets := map[int32]bool{}
	for _, p := range x.byDay[day] {
		row.PhotoCount++
		if towns := row.TownsInOrder; len(towns) == 0 || townKey(towns[len(towns)-1]) != townKey(p.proto.Place) {
			row.TownsInOrder = append(row.TownsInOrder, TownLevel(p.proto.Place))
		}
		if poi := p.proto.Place.PointOfInterest; poi != p.proto.Place.Town {
			row.PointsOfInterest = appendNew(row.PointsOfInterest, poi)
		}
		if offset := p.proto.Time.UtcOffsetSeconds; !seenOffsets[offset] {
			seenOffsets[offset] = true
			row.UtcOffsetsSeenSeconds = append(row.UtcOffsetsSeenSeconds, offset)
		}
	}
	if n, has := x.nights[day]; has {
		row.Night = n.proto
	}
	return row
}

// Trips are the trips that overlap the range, newest first, up to limit. Unless all is set, trips with no night
// confirmed by a morning photo away are left out and counted: they are often a long day out with no photo at home.
func (x *Index) Trips(within Range, limit int, all bool) *wherewasiv1.Trips {
	out := &wherewasiv1.Trips{}
	for i := len(x.trips) - 1; i >= 0; i-- {
		t := x.trips[i]
		if t.back.Before(within.First.Time) || t.left.After(within.Last.Time) {
			continue
		}
		if !all && t.proto.NightsConfirmedByMorning == 0 {
			out.UnconfirmedTripsLeftOut++
			continue
		}
		out.TotalTrips++
		if limit <= 0 || len(out.Trips) < limit {
			out.Trips = append(out.Trips, t.proto)
		}
	}
	return out
}
