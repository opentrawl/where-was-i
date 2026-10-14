package places

import (
	"fmt"
	"slices"

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
		n := &wherewasiv1.Night{Evening: day.Proto(), Place: TownLevel(chosen.proto.Place), PointOfInterest: chosen.proto.Place.PointOfInterest, KnownPlace: x.knownLabelAt(chosen),
			KnownPlaceKind: x.knownKindAt(chosen), Away: x.isAway(chosen), HomeKnown: x.homeKnown(chosen), Source: source,
			LastEveningPlace: TownLevel(evening.proto.Place), LastEveningTime: evening.proto.Time}
		if next != nil {
			km := kmBetween(evening.proto.Location, next.proto.Location)
			n.NextMorningPlace, n.NextMorningTime, n.KmBetweenEveningAndMorning = TownLevel(next.proto.Place), next.proto.Time, &km
			n.Travelling = km > TravellingNightKm
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
	var places namedPlaceCounter
	for day := left; !day.After(back.Time); day = day.Next() {
		t.PhotoCount += int32(len(x.byDay[day]))
		row := x.itineraryDay(day)
		for _, place := range row.Places {
			if place.WentTo {
				places.addPhotos(place.Name, place.PhotoCount)
			}
		}
		for _, title := range x.appleTrips[day] {
			if day.Before(back.Time) {
				t.AppleTripTitles = appendNew(t.AppleTripTitles, title)
			}
		}
	}
	t.PlacesInOrder = places.inOrder
	t.Route = withMostPhotos(places.inOrder, 10, 0)
	return &trip{proto: t, left: left, back: back}
}

// spreadKm is, for each name, how far apart the photos with that name are: the diagonal of the box around them.
// Photos more than PointOfInterestIsAnAreaKm apart mean you moved through the place, as through a national park or
// along a highway, rather than stopping at one spot in it.
func spreadKm(photos []photo, nameOf func(*wherewasiv1.Place) string) map[string]float64 {
	type corners struct{ south, west, north, east float64 }
	boxes := map[string]*corners{}
	for _, p := range photos {
		name, at := nameOf(p.proto.Place), p.proto.Location
		if name == "" {
			continue
		}
		if c := boxes[name]; c == nil {
			boxes[name] = &corners{at.Latitude, at.Longitude, at.Latitude, at.Longitude}
		} else {
			c.south, c.west = min(c.south, at.Latitude), min(c.west, at.Longitude)
			c.north, c.east = max(c.north, at.Latitude), max(c.east, at.Longitude)
		}
	}
	spread := map[string]float64{}
	for name, c := range boxes {
		spread[name] = kmBetween(&wherewasiv1.LatLng{Latitude: c.south, Longitude: c.west}, &wherewasiv1.LatLng{Latitude: c.north, Longitude: c.east})
	}
	return spread
}

// placeNames are where each of a day's photos was taken, at the scale of a town: the point of interest when you
// moved through it that day, as through a national park, and otherwise the town, so a hotel or a shop stays a spot.
func placeNames(photos []photo) []string {
	spread := spreadKm(photos, (*wherewasiv1.Place).GetPointOfInterest)
	names := make([]string, len(photos))
	for i, p := range photos {
		place := p.proto.Place
		names[i] = firstNonEmptyString(place.GetTown(), place.GetCounty(), place.GetState(), place.GetCountry())
		if poi := place.GetPointOfInterest(); poi != "" && (names[i] == "" || spread[poi] > PointOfInterestIsAnAreaKm) {
			names[i] = poi
		}
	}
	return names
}

// photoSpots are, for each of a day's photos, the spots it was taken at: somewhere you stopped, meaning a road or
// point of interest whose photos that day are within PointOfInterestIsAnAreaKm of each other, as a trail, a viewpoint
// or a museum is. A road you drove along, or a park you moved through, is not a spot. Photo counts are left at zero.
func photoSpots(photos []photo, names []string) [][]*wherewasiv1.Spot {
	roadSpread := spreadKm(photos, (*wherewasiv1.Place).GetRoad)
	poiSpread := spreadKm(photos, (*wherewasiv1.Place).GetPointOfInterest)
	spots := make([][]*wherewasiv1.Spot, len(photos))
	for i, p := range photos {
		place := p.proto.Place
		if place.PointOfInterest != "" && place.Road != "" && roadSpread[place.Road] <= PointOfInterestIsAnAreaKm {
			spots[i] = append(spots[i], &wherewasiv1.Spot{Name: place.Road, Kind: wherewasiv1.SpotKind_SPOT_KIND_ROAD, Neighbourhood: place.Neighbourhood})
		}
		if place.PointOfInterest != "" && place.PointOfInterest != names[i] && poiSpread[place.PointOfInterest] <= PointOfInterestIsAnAreaKm {
			spots[i] = append(spots[i], &wherewasiv1.Spot{Name: place.PointOfInterest, Kind: wherewasiv1.SpotKind_SPOT_KIND_POINT_OF_INTEREST,
				Neighbourhood: place.Neighbourhood})
		}
	}
	return spots
}

// namedPlaceCounter counts photos by name, keeping the order names were first seen.
type namedPlaceCounter struct {
	inOrder []*wherewasiv1.NamedPlace
	byName  map[string]*wherewasiv1.NamedPlace
}

func (c *namedPlaceCounter) add(name string) { c.addPhotos(name, 1) }

func (c *namedPlaceCounter) addPhotos(name string, photos int32) {
	if name == "" {
		return
	}
	if c.byName == nil {
		c.byName = map[string]*wherewasiv1.NamedPlace{}
	}
	place, seen := c.byName[name]
	if !seen {
		place = &wherewasiv1.NamedPlace{Name: name}
		c.byName[name] = place
		c.inOrder = append(c.inOrder, place)
	}
	place.PhotoCount += photos
}

// withMostPhotos are the places holding at least share of the photos, or the one with most if none does, at most max
// of them, in the order you went.
func withMostPhotos(places []*wherewasiv1.NamedPlace, max int, share float64) []string {
	var total int32
	for _, p := range places {
		total += p.PhotoCount
	}
	busiest := slices.Clone(places)
	slices.SortStableFunc(busiest, func(a, b *wherewasiv1.NamedPlace) int { return int(b.PhotoCount - a.PhotoCount) })
	kept := map[string]bool{}
	for i, p := range busiest {
		if i == max || (i > 0 && float64(p.PhotoCount) < share*float64(total)) {
			break
		}
		kept[p.Name] = true
	}
	var names []string
	for _, p := range places {
		if kept[p.Name] {
			names = append(names, p.Name)
		}
	}
	return names
}

func (c *namedPlaceCounter) mostPhotosFirst() []*wherewasiv1.NamedPlace {
	sorted := slices.Clone(c.inOrder)
	slices.SortStableFunc(sorted, func(a, b *wherewasiv1.NamedPlace) int { return int(b.PhotoCount - a.PhotoCount) })
	return sorted
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
	photos := x.byDay[day]
	names := placeNames(photos)
	spotsOf := photoSpots(photos, names)
	type counters struct {
		place          *wherewasiv1.DayPlace
		spots          map[string]*wherewasiv1.Spot
		neighbourhoods namedPlaceCounter
	}
	byName := map[string]*counters{}
	seenOffsets := map[int32]bool{}
	for i, p := range photos {
		row.PhotoCount++
		place := p.proto.Place
		c := byName[names[i]]
		if c == nil {
			kind := wherewasiv1.DayPlaceKind_DAY_PLACE_KIND_TOWN
			if names[i] == place.PointOfInterest {
				kind = wherewasiv1.DayPlaceKind_DAY_PLACE_KIND_AREA
			}
			c = &counters{place: &wherewasiv1.DayPlace{Name: names[i], Kind: kind, Region: regionOf(place)},
				spots: map[string]*wherewasiv1.Spot{}}
			byName[names[i]] = c
			row.Places = append(row.Places, c.place)
		}
		c.place.PhotoCount++
		c.neighbourhoods.add(place.Neighbourhood)
		for _, found := range spotsOf[i] {
			spot := c.spots[found.Name]
			if spot == nil {
				spot = &wherewasiv1.Spot{Name: found.Name, Kind: found.Kind, Neighbourhood: found.Neighbourhood}
				c.spots[found.Name] = spot
				c.place.Spots = append(c.place.Spots, spot)
			}
			spot.PhotoCount++
		}
		if offset := p.proto.Time.UtcOffsetSeconds; !seenOffsets[offset] {
			seenOffsets[offset] = true
			row.UtcOffsetsSeenSeconds = append(row.UtcOffsetsSeenSeconds, offset)
		}
	}
	var counted []*wherewasiv1.NamedPlace
	for _, place := range row.Places {
		slices.SortStableFunc(place.Spots, func(a, b *wherewasiv1.Spot) int { return int(b.PhotoCount - a.PhotoCount) })
		place.Neighbourhoods = byName[place.Name].neighbourhoods.mostPhotosFirst()
		counted = append(counted, &wherewasiv1.NamedPlace{Name: place.Name, PhotoCount: place.PhotoCount})
	}
	for _, name := range withMostPhotos(counted, 4, WentToShareOfPhotos) {
		byName[name].place.WentTo = true
	}
	if n, has := x.nights[day]; has {
		row.Night = n.proto
	}
	return row
}

// regionOf is the country, state and county of a place.
func regionOf(p *wherewasiv1.Place) *wherewasiv1.Place {
	return &wherewasiv1.Place{CountryCode: p.CountryCode, Country: p.Country, State: p.State, County: p.County}
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
