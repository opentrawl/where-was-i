// Package places turns located photos into nights, trips, itineraries and days. It decides only what needs no
// judgement: local time from the time-zone map, and distance to known places (from a file, or Apple's own home).
// Place names are Apple's, field by field.
package places

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ringsaturn/tzf"
	"google.golang.org/protobuf/types/known/timestamppb"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
	"github.com/opentrawl/where-was-i/internal/sources"
)

// Settings, not judgement.
const (
	KnownPlaceRadiusKm           = 0.5           // a photo this close to a known place is at it
	AwayKm                       = 25.0          // a night is away when it is this far from every home
	DayStartsAt                  = 4 * time.Hour // a local day runs from 04:00 to 04:00, so a late evening stays with its evening
	MorningEndsAt                = 12            // the next day's first photo counts as "where you woke up" before this hour
	MaxNightsWithoutPhotosInTrip = 2             // nights without photos a trip may bridge
	PointOfInterestIsAnAreaKm    = 2
	WentToShareOfPhotos          = 0.25 // a place a day went to holds at least this share of its photos
	InTransitKmPerHour           = 200  // faster than this between photos, both arriving and leaving, is a plane or train
	TravellingNightKm            = 300  // evening and next-morning photos this far apart mean the night was spent travelling
)

// Date is a local calendar day, held as midnight UTC so it compares and sorts simply.
type Date struct{ time.Time }

func DateOf(t time.Time) Date {
	return Date{time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}
}

func (d Date) Proto() *wherewasiv1.CalendarDate {
	return &wherewasiv1.CalendarDate{Year: int32(d.Year()), Month: int32(d.Month()), Day: int32(d.Day())}
}

func (d Date) Next() Date           { return Date{d.AddDate(0, 0, 1)} }
func (d Date) DaysUntil(e Date) int { return int(e.Sub(d.Time).Hours() / 24) }

func DateFromProto(d *wherewasiv1.CalendarDate) Date {
	return Date{time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.UTC)}
}

func dateFromProto(d *wherewasiv1.CalendarDate, otherwise Date) Date {
	if d == nil {
		return otherwise
	}
	return DateFromProto(d)
}

var (
	earliestDate = Date{time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)}
	latestDate   = Date{time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)}
)

// Range is an inclusive span of local days.
type Range struct{ First, Last Date }

func (r Range) Contains(d Date) bool { return !d.Before(r.First.Time) && !d.After(r.Last.Time) }

// Everything is the range with no bounds.
var Everything = Range{earliestDate, latestDate}

func kmBetween(a, b *wherewasiv1.LatLng) float64 {
	p1, p2 := a.Latitude*math.Pi/180, b.Latitude*math.Pi/180
	dp, dl := p2-p1, (b.Longitude-a.Longitude)*math.Pi/180
	h := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 6371 * 2 * math.Asin(math.Sqrt(h))
}

type knownPlace struct {
	proto       *wherewasiv1.KnownPlace
	first, last Date
}

// photo is a LocatedPhoto with its local clock worked out once.
type photo struct {
	proto *wherewasiv1.LocatedPhoto
	local time.Time
}

func (p photo) day() Date { return DateOf(p.local.Add(-DayStartsAt)) }

// TownLevel is the part of a place that identifies a town: country, state, county and town.
func TownLevel(p *wherewasiv1.Place) *wherewasiv1.Place {
	if p == nil {
		return &wherewasiv1.Place{}
	}
	return &wherewasiv1.Place{CountryCode: p.CountryCode, Country: p.Country, State: p.State, County: p.County, Town: p.Town}
}

// townKey identifies a town by country and name. Apple sometimes gives one city two county or state names, so those
// are left out; the name falls back to county, then state, when Apple names no town.
func townKey(p *wherewasiv1.Place) string {
	name := p.GetTown()
	if name == "" {
		name = p.GetCounty()
	}
	if name == "" {
		name = p.GetState()
	}
	return p.GetCountryCode() + "\x00" + name
}

// Index holds every located photo by local day, with the nights and trips worked out from them.
type Index struct {
	known      []knownPlace
	photos     []photo
	days       []Date // days with photos, oldest first
	byDay      map[Date][]photo
	appleTrips map[Date][]string
	nights     map[Date]*night
	trips      []*trip // oldest first
	today      Date
}

// Build reads nothing itself: the sources are passed in.
// today is the local day now, which has no night yet.
func Build(libraryPhotos []sources.LibraryPhoto, appleTrips []sources.AppleTrip, known *wherewasiv1.KnownPlaces, zones *tzf.DefaultFinder,
	today Date) *Index {
	index := &Index{byDay: map[Date][]photo{}, appleTrips: map[Date][]string{}, today: today}
	for _, place := range known.GetPlaces() {
		if place.Location != nil {
			index.known = append(index.known, knownPlace{place, dateFromProto(place.ValidFrom, earliestDate), dateFromProto(place.ValidTo, latestDate)})
		}
	}
	for _, p := range libraryPhotos {
		if p.Latitude == nil {
			continue
		}
		location := &wherewasiv1.LatLng{Latitude: *p.Latitude, Longitude: *p.Longitude}
		located := &wherewasiv1.LocatedPhoto{Time: localTime(p.TakenUTC, location, zones), Location: location,
			Place: placeFromApple(p.Place), PhotoUuid: p.UUID}
		if p.UTCOffsetSeconds != nil {
			source := wherewasiv1.DeviceOffsetSource_DEVICE_OFFSET_SOURCE_PHOTO
			if p.OffsetIsInferred {
				source = wherewasiv1.DeviceOffsetSource_DEVICE_OFFSET_SOURCE_APPLE_INFERRED
			}
			located.DeviceOffset = &wherewasiv1.DeviceOffset{UtcOffsetSeconds: int32(*p.UTCOffsetSeconds), Source: source}
		}
		t := located.Time
		index.photos = append(index.photos, photo{located, t.Utc.AsTime().In(time.FixedZone(t.ZoneName, int(t.UtcOffsetSeconds)))})
	}
	sort.SliceStable(index.photos, func(i, j int) bool { return index.photos[i].local.Before(index.photos[j].local) })
	for _, p := range index.photos {
		day := p.day()
		if _, seen := index.byDay[day]; !seen {
			index.days = append(index.days, day)
		}
		index.byDay[day] = append(index.byDay[day], p)
	}
	sort.Slice(index.days, func(i, j int) bool { return index.days[i].Before(index.days[j].Time) })
	for _, t := range appleTrips {
		title := t.PlaceTitle
		if title == "" {
			title = "untitled"
		}
		for d := DateOf(t.FirstDay); !d.After(DateOf(t.LastDay).Time); d = d.Next() {
			index.appleTrips[d] = append(index.appleTrips[d], title)
		}
	}
	index.workOutNights()
	index.workOutTrips()
	return index
}

// localTime is the legal time where the photo was, from the time-zone map.
func localTime(instant time.Time, location *wherewasiv1.LatLng, zones *tzf.DefaultFinder) *wherewasiv1.LocalTime {
	local := &wherewasiv1.LocalTime{Utc: timestamppb.New(instant), ZoneSource: wherewasiv1.ZoneSource_ZONE_SOURCE_UNKNOWN}
	if name := zones.GetTimezoneName(location.Longitude, location.Latitude); name != "" {
		if zone, err := time.LoadLocation(name); err == nil {
			_, offset := instant.In(zone).Zone()
			local.UtcOffsetSeconds, local.ZoneName, local.ZoneSource = int32(offset), name, wherewasiv1.ZoneSource_ZONE_SOURCE_TIME_ZONE_MAP
		}
	}
	local.Local = instant.In(time.FixedZone("", int(local.UtcOffsetSeconds))).Format(time.RFC3339)
	return local
}

func placeFromApple(stored *sources.StoredPlace) *wherewasiv1.Place {
	if stored == nil {
		return &wherewasiv1.Place{}
	}
	return &wherewasiv1.Place{CountryCode: stored.CountryCode, Country: stored.Country, State: stored.State,
		County: stored.County, Town: stored.Town, Neighbourhood: stored.Neighbourhood, Street: stored.Street,
		PointOfInterest: stored.PointOfInterest, Road: stored.Road, Water: stored.Water, Island: stored.Island, FullAddress: stored.Address,
		Regions: stored.Regions}
}

// AppleHome is the home Apple Photos knows about: the mean position of the photos it marks as taken at home, in use
// from the first of them. Apple only knows the current home. Returns nil when no photo is marked.
func AppleHome(libraryPhotos []sources.LibraryPhoto) *wherewasiv1.KnownPlace {
	var latitude, longitude float64
	var count int
	var first time.Time
	for _, p := range libraryPhotos {
		if p.Latitude == nil || p.Place == nil || !p.Place.AppleMarksAsHome {
			continue
		}
		latitude, longitude, count = latitude+*p.Latitude, longitude+*p.Longitude, count+1
		if first.IsZero() || p.TakenUTC.Before(first) {
			first = p.TakenUTC
		}
	}
	if count == 0 {
		return nil
	}
	return &wherewasiv1.KnownPlace{Label: "Home", Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
		Location:  &wherewasiv1.LatLng{Latitude: latitude / float64(count), Longitude: longitude / float64(count)},
		ValidFrom: DateOf(first).Proto(), Source: wherewasiv1.KnownPlaceSource_KNOWN_PLACE_SOURCE_APPLE_HOME,
		Evidence: fmt.Sprintf("%d photos Apple Photos marks as taken at home", count)}
}

// WithAppleHome adds Apple's home to the known places unless they name a current home of their own: one with a
// location and no end date. Apple knows only the current home, so a file of earlier homes adds to it.
func WithAppleHome(known *wherewasiv1.KnownPlaces, libraryPhotos []sources.LibraryPhoto) *wherewasiv1.KnownPlaces {
	for _, place := range known.GetPlaces() {
		if place.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME && place.Location != nil && place.ValidTo == nil {
			return known
		}
	}
	if home := AppleHome(libraryPhotos); home != nil {
		return &wherewasiv1.KnownPlaces{Places: append([]*wherewasiv1.KnownPlace{home}, known.GetPlaces()...)}
	}
	return known
}

type nearest struct {
	place *knownPlace
	km    float64
}

func (x *Index) nearestKnown(p photo, kinds ...wherewasiv1.KnownPlaceKind) (nearest, bool) {
	day, best, found := p.day(), nearest{}, false
	for i := range x.known {
		k := &x.known[i]
		if day.Before(k.first.Time) || day.After(k.last.Time) || (len(kinds) > 0 && !kindIn(k.proto.Kind, kinds)) {
			continue
		}
		if km := kmBetween(p.proto.Location, k.proto.Location); !found || km < best.km {
			best, found = nearest{k, km}, true
		}
	}
	return best, found
}

func kindIn(kind wherewasiv1.KnownPlaceKind, kinds []wherewasiv1.KnownPlaceKind) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

var kindsThatAnchorTrips = []wherewasiv1.KnownPlaceKind{wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
	wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_OTHER}

// knownPlaceAt is the known place the photo was taken at, if any.
func (x *Index) knownPlaceAt(p photo) *knownPlace {
	if n, ok := x.nearestKnown(p); ok && n.km <= KnownPlaceRadiusKm {
		return n.place
	}
	return nil
}

func (x *Index) knownLabelAt(p photo) string {
	if k := x.knownPlaceAt(p); k != nil {
		return k.proto.Label
	}
	return ""
}

func (x *Index) knownKindAt(p photo) wherewasiv1.KnownPlaceKind {
	if k := x.knownPlaceAt(p); k != nil {
		return k.proto.Kind
	}
	return wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_UNSPECIFIED
}

// homeKnown is true when some home is known for the photo's day, so whether it is away can be told.
func (x *Index) homeKnown(p photo) bool {
	_, ok := x.nearestKnown(p, kindsThatAnchorTrips...)
	return ok
}

func (x *Index) atHomeOrFamily(p photo) bool {
	k := x.knownPlaceAt(p)
	return k != nil && (k.proto.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME || k.proto.Kind == wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_FAMILY)
}

func (x *Index) kmFromHome(p photo) (float64, bool) {
	n, ok := x.nearestKnown(p, wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME)
	return n.km, ok
}

// isAway is true when the photo is beyond the away distance from every home. Family places label nights but do not
// anchor trips: staying with family who live far away is travel. Nor do workplaces: an office near an airport would
// otherwise make a departure day look like a day near home. Without a known home for the day it is not away: it is
// unknown.
func (x *Index) isAway(p photo) bool {
	n, ok := x.nearestKnown(p, kindsThatAnchorTrips...)
	return ok && n.km > AwayKm
}

func appendNew(list []string, value string) []string {
	if value == "" {
		return list
	}
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}
