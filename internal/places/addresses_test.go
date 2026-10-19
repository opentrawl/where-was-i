package places

import (
	"testing"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

func TestProposalAddressUsesPostalStreetWithNumber(t *testing.T) {
	photoAt := func(latitude float64, street string) *wherewasiv1.LocatedPhoto {
		return &wherewasiv1.LocatedPhoto{Location: &wherewasiv1.LatLng{Latitude: latitude, Longitude: 1},
			Place: &wherewasiv1.Place{Town: "Exampleton", Neighbourhood: "Old Quarter", Street: street, Road: "Sample Lane"}}
	}
	photos := []*wherewasiv1.LocatedPhoto{photoAt(50, "Sample Lane 12"), photoAt(50.0001, "Sample Lane 12"), photoAt(50.0002, "Sample Lane 14")}
	place := proposalPlace(photos, &wherewasiv1.LatLng{Latitude: 50.0002, Longitude: 1})
	if place.Street != "Sample Lane 12" {
		t.Fatalf("got %q; want the most common complete postal street", place.Street)
	}
	if label := proposalLabel(place, "Earlier home"); label != "Exampleton · Old Quarter · Sample Lane 12" {
		t.Errorf("label %q does not include town, neighbourhood and house number", label)
	}
	if photos[2].Place.Street != "Sample Lane 14" {
		t.Fatal("selecting an address changed the source photo")
	}
}

func TestProposalAddressBreaksTiesNearestTheMean(t *testing.T) {
	photos := []*wherewasiv1.LocatedPhoto{
		{Location: &wherewasiv1.LatLng{Latitude: 50.001, Longitude: 1}, Place: &wherewasiv1.Place{Street: "12 Sample Lane"}},
		{Location: &wherewasiv1.LatLng{Latitude: 50.0001, Longitude: 1}, Place: &wherewasiv1.Place{Street: "14 Sample Lane"}},
	}
	if got := proposalStreetAddress(photos, &wherewasiv1.LatLng{Latitude: 50, Longitude: 1}); got != "14 Sample Lane" {
		t.Fatalf("got %q; want the tied address nearest the averaged position", got)
	}
}

func TestProposalAddressDoesNotInventHouseNumbers(t *testing.T) {
	location := &wherewasiv1.LatLng{Latitude: 50, Longitude: 1}
	for _, place := range []*wherewasiv1.Place{{Street: "Sample Lane"}, {Road: "Sample Lane"}} {
		photos := []*wherewasiv1.LocatedPhoto{{Location: location, Place: place}}
		if got := proposalStreetAddress(photos, location); got != "Sample Lane" {
			t.Errorf("got %q; want Apple's street alone", got)
		}
	}
}
