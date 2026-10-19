package places

import (
	"math"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

func homeFixture() *Index {
	return &Index{byDay: map[Date][]photo{}, today: DateOf(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))}
}

func fixturePhoto(index *Index, day Date, hour int, latitude float64, neighbourhood string) {
	local := day.Add(time.Duration(hour) * time.Hour)
	p := photo{local: local, proto: &wherewasiv1.LocatedPhoto{Time: &wherewasiv1.LocalTime{Utc: timestamppb.New(local)},
		Location: &wherewasiv1.LatLng{Latitude: latitude, Longitude: 1}, Place: &wherewasiv1.Place{CountryCode: "XX", Town: "Exampleton", Neighbourhood: neighbourhood}}}
	if len(index.byDay[day]) == 0 {
		index.days = append(index.days, day)
	}
	index.byDay[day] = append(index.byDay[day], p)
	index.photos = append(index.photos, p)
}

func finishFixture(index *Index) {
	sort.Slice(index.days, func(i, j int) bool { return index.days[i].Before(index.days[j].Time) })
	sort.Slice(index.photos, func(i, j int) bool { return index.photos[i].local.Before(index.photos[j].local) })
	for _, photos := range index.byDay {
		sort.Slice(photos, func(i, j int) bool { return photos[i].local.Before(photos[j].local) })
	}
	index.workOutNights()
}

func TestHomesFindsMovesIgnoresShortTripsAndRoundTripsKnownPlaces(t *testing.T) {
	index := homeFixture()
	start := DateOf(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	move := Date{start.AddDate(0, 8, 0)}
	appleStart := Date{start.AddDate(1, 4, 0)}
	for day := start; day.Before(appleStart.Time); day = (Date{day.AddDate(0, 0, 4)}) {
		latitude, neighbourhood := 50.0, "Old Quarter"
		if !day.Before(move.Time) {
			latitude, neighbourhood = 50.02, "New Quarter"
		}
		if offset := start.DaysUntil(day); offset >= 100 && offset < 120 {
			latitude, neighbourhood = 52, "Holiday"
		}
		fixturePhoto(index, day, 20, latitude, neighbourhood)
		fixturePhoto(index, day, 21, latitude+0.0002, neighbourhood)
	}
	index.known = []knownPlace{{proto: &wherewasiv1.KnownPlace{Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
		Location: &wherewasiv1.LatLng{Latitude: 53, Longitude: 1}}, first: appleStart, last: latestDate}}
	finishFixture(index)
	homes := index.Homes()
	if len(homes.Proposals) != 2 {
		t.Fatalf("got %d homes; want the two sustained homes", len(homes.Proposals))
	}
	for i, want := range []string{"Old Quarter", "New Quarter"} {
		proposal := homes.Proposals[i]
		if proposal.Place.Neighbourhood != want {
			t.Errorf("home %d: got %q, want %q", i, proposal.Place.Neighbourhood, want)
		}
		wantLatitude := []float64{50.0002, 50.0202}[i]
		if math.Abs(proposal.Home.Location.Latitude-wantLatitude) > 1e-10 {
			t.Errorf("mean latitude %v; want %v", proposal.Home.Location.Latitude, wantLatitude)
		}
		if proposal.PhotoCount != proposal.Nights {
			t.Errorf("%d photos for %d nights; want only the last photo on each evening", proposal.PhotoCount, proposal.Nights)
		}
	}
	first, second := homes.Proposals[0].Home, homes.Proposals[1].Home
	if DateFromProto(first.ValidFrom) != start || DateFromProto(first.ValidTo).Next() != DateFromProto(second.ValidFrom) || DateFromProto(second.ValidTo).Next() != appleStart {
		t.Fatal("home dates do not cover the unknown period up to Apple's home")
	}
	data, err := prototext.Marshal(homes.KnownPlaces())
	if err != nil {
		t.Fatal(err)
	}
	confirmed := &wherewasiv1.KnownPlaces{}
	if err := prototext.Unmarshal(data, confirmed); err != nil {
		t.Fatal(err)
	}
	for _, home := range confirmed.Places {
		index.known = append(index.known, knownPlace{home, DateFromProto(home.ValidFrom), DateFromProto(home.ValidTo)})
	}
	index.workOutNights()
	for _, year := range index.Years(index.today).Years {
		if year.NightsHomeUnknown != 0 {
			t.Errorf("%d home-unknown nights remain", year.NightsHomeUnknown)
		}
	}
	if got := index.Homes(); got.UnknownNights != 0 || len(got.Proposals) != 0 {
		t.Fatal("proposed homes on already covered nights")
	}
}

func TestHomesNeedsSustainedEvidence(t *testing.T) {
	index := homeFixture()
	start := DateOf(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	for offset := 0; offset < 30; offset++ {
		fixturePhoto(index, Date{start.AddDate(0, 0, offset)}, 20, 50, "Holiday")
	}
	finishFixture(index)
	homes := index.Homes()
	if homes.UnknownNights != 30 || len(homes.Proposals) != 0 {
		t.Fatal("a short stay became a home")
	}
}

func TestHomesUsesNextMorningAndDeduplicatesPhotoEvidence(t *testing.T) {
	index := homeFixture()
	start := DateOf(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	for offset := 0; offset <= 120; offset++ {
		day := Date{start.AddDate(0, 0, offset)}
		fixturePhoto(index, day, 8, 50, "Morning Home")
		fixturePhoto(index, day, 20, 50.1, "Evening Out")
	}
	finishFixture(index)
	homes := index.Homes()
	if len(homes.Proposals) != 1 {
		t.Fatalf("got %d homes; want one", len(homes.Proposals))
	}
	proposal := homes.Proposals[0]
	if proposal.Place.Neighbourhood != "Morning Home" || proposal.Nights != 120 || proposal.PhotoCount != 120 || proposal.Home.Location.Latitude != 50 {
		t.Fatalf("morning evidence not used once per photo: %+v", proposal)
	}
}

func TestHomesEndsBeforeKnownHomeStartingToday(t *testing.T) {
	index := homeFixture()
	start := Date{index.today.AddDate(0, 0, -120)}
	for day := start; day.Before(index.today.Time); day = day.Next() {
		fixturePhoto(index, day, 20, 50, "Earlier Home")
	}
	index.known = []knownPlace{{proto: &wherewasiv1.KnownPlace{Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
		Location: &wherewasiv1.LatLng{Latitude: 52, Longitude: 1}}, first: index.today, last: latestDate}}
	finishFixture(index)
	homes := index.Homes()
	if len(homes.Proposals) != 1 || homes.Proposals[0].Home.ValidTo == nil || DateFromProto(homes.Proposals[0].Home.ValidTo).Next() != index.today {
		t.Fatal("earlier home overlaps the known home starting today")
	}
}

func TestHomesDoesNotEmitBriefWindowWinnerPeriods(t *testing.T) {
	index := homeFixture()
	start := DateOf(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	for offset := 0; offset < 660; offset++ {
		latitude, neighbourhood := 54.0, "Sustained Home"
		if offset < 360 {
			latitude, neighbourhood = 50+float64((offset/30)%3)*0.2, "Repeated Short Stay"
		}
		fixturePhoto(index, Date{start.AddDate(0, 0, offset)}, 20, latitude, neighbourhood)
	}
	finishFixture(index)
	homes := index.Homes()
	if len(homes.Proposals) == 0 || homes.Proposals[len(homes.Proposals)-1].Place.Neighbourhood != "Sustained Home" {
		t.Fatal("the sustained home is missing")
	}
	for _, proposal := range homes.Proposals {
		if proposal.Nights < HomeMinimumNights {
			t.Errorf("brief window winner became a home with %d nights", proposal.Nights)
		}
		var first, last Date
		for _, day := range index.days {
			if day.Before(DateFromProto(proposal.Home.ValidFrom).Time) || (proposal.Home.ValidTo != nil && day.After(DateFromProto(proposal.Home.ValidTo).Time)) {
				continue
			}
			if kmBetween(index.nights[day].photo.proto.Location, proposal.Home.Location) > KnownPlaceRadiusKm {
				continue
			}
			if first.IsZero() {
				first = day
			}
			last = day
		}
		if first.DaysUntil(last) < HomeMinimumSpanDays {
			t.Errorf("brief window winner became a home spanning %d days", first.DaysUntil(last))
		}
	}
	if DateFromProto(homes.Proposals[0].Home.ValidFrom) != start {
		t.Fatal("retained home does not cover the earlier unknown nights")
	}
}
