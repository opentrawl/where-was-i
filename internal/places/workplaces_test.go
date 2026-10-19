package places

import (
	"math"
	"testing"
	"time"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

func TestWorkplacesFindsRepeatedWeekdayDaytimesAwayFromHomes(t *testing.T) {
	index := homeFixture()
	start := DateOf(time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)) // Monday
	for offset := 0; offset <= 84; offset += 14 {
		day := Date{start.AddDate(0, 0, offset)}
		fixturePhoto(index, day, 10, 50, "Work Quarter")
		fixturePhoto(index, day, 14, 50.0002, "Work Quarter")
		fixturePhoto(index, day, 11, 51, "Home Quarter")
		fixturePhoto(index, day, 12, 52, "Earlier Home Quarter")
		fixturePhoto(index, day, 20, 53, "Evening Venue")
		fixturePhoto(index, Date{day.AddDate(0, 0, 5)}, 11, 54, "Weekend Venue")
	}
	// Many photos from a brief visit still do not establish a workplace.
	for offset := 0; offset < 20; offset++ {
		fixturePhoto(index, Date{start.AddDate(0, 0, offset)}, 10, 55, "Holiday")
	}
	index.known = []knownPlace{{proto: &wherewasiv1.KnownPlace{Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
		Location: &wherewasiv1.LatLng{Latitude: 51, Longitude: 1}}, first: earliestDate, last: latestDate}}
	homes := Homes{Proposals: []ProposedHome{{Home: &wherewasiv1.KnownPlace{Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
		Location: &wherewasiv1.LatLng{Latitude: 52, Longitude: 1}, ValidFrom: start.Proto()}}}}
	finishFixture(index)
	proposals := index.Workplaces(homes)
	if len(proposals) != 1 {
		t.Fatalf("got %d workplaces; want only the sustained weekday location", len(proposals))
	}
	proposal := proposals[0]
	if proposal.Days != 7 || proposal.PhotoCount != 14 || proposal.Place.Neighbourhood != "Work Quarter" {
		t.Fatalf("got %d days, %d photos at %q; want 7 days and 14 photos at Work Quarter", proposal.Days, proposal.PhotoCount, proposal.Place.Neighbourhood)
	}
	if math.Abs(proposal.Work.Location.Latitude-50.0001) > 1e-10 {
		t.Errorf("latitude %v; want the mean of both photos on each supporting day", proposal.Work.Location.Latitude)
	}
	if DateFromProto(proposal.Work.ValidFrom) != start || DateFromProto(proposal.Work.ValidTo) != (Date{start.AddDate(0, 0, 84)}) {
		t.Error("dates do not bound the supporting evidence")
	}
	if len(index.known) != 1 {
		t.Error("proposals mutated the known-place index")
	}
	index.known = append(index.known, knownPlace{proposal.Work, DateFromProto(proposal.Work.ValidFrom), DateFromProto(proposal.Work.ValidTo)})
	if got := index.Workplaces(homes); len(got) != 0 {
		t.Fatalf("got %d proposals after confirming the workplace", len(got))
	}
}

func TestWorkplacesSplitsLongGapsAndDoesNotCountPhotoBurstsAsWeeks(t *testing.T) {
	index := homeFixture()
	index.known = []knownPlace{{proto: &wherewasiv1.KnownPlace{Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
		Location: &wherewasiv1.LatLng{Latitude: 52, Longitude: 1}}, first: earliestDate, last: latestDate}}
	start := DateOf(time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC))
	for _, periodStart := range []Date{start, {start.AddDate(1, 0, 1)}} {
		for offset := 0; offset <= 70; offset += 14 {
			fixturePhoto(index, Date{periodStart.AddDate(0, 0, offset)}, 10, 50, "Work Quarter")
		}
	}
	for offset := 0; offset < 120; offset++ {
		fixturePhoto(index, start, 11, 51, "Photo Burst")
	}
	finishFixture(index)
	known := index.known
	index.known = nil
	if proposals := index.Workplaces(Homes{}); len(proposals) != 0 {
		t.Fatal("proposed workplaces before knowing where home was")
	}
	index.known = known
	proposals := index.Workplaces(Homes{})
	if len(proposals) != 2 {
		t.Fatalf("got %d workplaces; want separate periods at the same place", len(proposals))
	}
	for _, proposal := range proposals {
		if proposal.Days != 6 || proposal.PhotoCount != 6 || proposal.Place.Neighbourhood != "Work Quarter" {
			t.Fatalf("got %d days and %d photos at %q; want 6 days and photos at Work Quarter", proposal.Days, proposal.PhotoCount, proposal.Place.Neighbourhood)
		}
	}
}

func TestWorkplacesRequiresRecurrenceWithinWindowAndTrimsSparseTails(t *testing.T) {
	index := homeFixture()
	index.known = []knownPlace{{proto: &wherewasiv1.KnownPlace{Kind: wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME,
		Location: &wherewasiv1.LatLng{Latitude: 52, Longitude: 1}}, first: earliestDate, last: latestDate}}
	start := DateOf(time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC))
	for offset := 0; offset <= 280; offset += 56 {
		fixturePhoto(index, Date{start.AddDate(0, 0, offset)}, 10, 51, "Occasional Visit")
	}
	for _, offset := range []int{0, 140, 490, 630} {
		fixturePhoto(index, Date{start.AddDate(0, 0, offset)}, 10, 50.0005, "Work Quarter")
	}
	for offset := 280; offset <= 350; offset += 14 {
		fixturePhoto(index, Date{start.AddDate(0, 0, offset)}, 10, 50, "Work Quarter")
	}
	finishFixture(index)
	proposals := index.Workplaces(Homes{})
	if len(proposals) != 1 {
		t.Fatalf("got %d workplaces; want only the visits sustained within six months", len(proposals))
	}
	proposal := proposals[0]
	if proposal.Days != 6 || proposal.PhotoCount != 6 || proposal.Work.Location.Latitude != 50 {
		t.Fatal("the sparse tails affected the workplace evidence or mean position")
	}
	if DateFromProto(proposal.Work.ValidFrom) != (Date{start.AddDate(0, 0, 280)}) || DateFromProto(proposal.Work.ValidTo) != (Date{start.AddDate(0, 0, 350)}) {
		t.Fatal("the sparse tails stretched the workplace dates")
	}
}
