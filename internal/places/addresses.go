package places

import (
	"strings"

	"google.golang.org/protobuf/proto"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

// proposalPlace takes its names and address only from the photos used for the proposed position.
func proposalPlace(photos []*wherewasiv1.LocatedPhoto, location *wherewasiv1.LatLng) *wherewasiv1.Place {
	byName := map[string]int{}
	var place *wherewasiv1.Place
	most := 0
	for _, p := range photos {
		key := townKey(p.Place) + "\x00" + p.GetPlace().GetNeighbourhood()
		byName[key]++
		if byName[key] > most {
			place, most = p.Place, byName[key]
		}
	}
	if place == nil {
		place = &wherewasiv1.Place{}
	} else {
		place = proto.Clone(place).(*wherewasiv1.Place)
	}
	place.Street = proposalStreetAddress(photos, location)
	return place
}

func proposalStreetAddress(photos []*wherewasiv1.LocatedPhoto, location *wherewasiv1.LatLng) string {
	// Apple's postal street includes the house number. Its road name does not.
	addresses := make([]string, len(photos))
	hasPostalStreet := false
	for i, p := range photos {
		addresses[i] = strings.TrimSpace(p.GetPlace().GetStreet())
		hasPostalStreet = hasPostalStreet || addresses[i] != ""
	}
	if !hasPostalStreet {
		for i, p := range photos {
			addresses[i] = strings.TrimSpace(p.GetPlace().GetRoad())
		}
	}
	counts := map[string]int{}
	for _, address := range addresses {
		if address != "" {
			counts[address]++
		}
	}
	selected, most, nearest := "", 0, 0.0
	for i, address := range addresses {
		if address == "" {
			continue
		}
		distance := kmBetween(photos[i].Location, location)
		if count := counts[address]; count > most || count == most && distance < nearest {
			selected, most, nearest = address, count, distance
		}
	}
	return selected
}

func proposalLabel(place *wherewasiv1.Place, fallback string) string {
	label := strings.Join(nonEmpty(place.GetTown(), place.GetNeighbourhood(), place.GetStreet()), " · ")
	if label == "" {
		label = firstNonEmptyString(place.GetCounty(), place.GetState(), place.GetCountry(), fallback)
	}
	return label
}
