package sources

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/prototext"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

// DefaultKnownPlacesFile is where the optional known-places file lives. See examples/known-places.textproto.
var DefaultKnownPlacesFile = filepath.Join(os.Getenv("HOME"), ".config/where-was-i/known-places.textproto")

// ReadKnownPlaces parses the textproto, skipping YAML frontmatter if the file has any. A missing file is not an error:
// it returns no places.
func ReadKnownPlaces(path string) (*wherewasiv1.KnownPlaces, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &wherewasiv1.KnownPlaces{}, nil
	}
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte("---\n")) {
		if end := bytes.Index(data[4:], []byte("\n---\n")); end >= 0 {
			data = data[4+end+5:]
		}
	}
	places := &wherewasiv1.KnownPlaces{}
	if err := prototext.Unmarshal(data, places); err != nil {
		return nil, fmt.Errorf("known places %s: %w", path, err)
	}
	for _, place := range places.Places {
		place.Source = wherewasiv1.KnownPlaceSource_KNOWN_PLACE_SOURCE_FILE
	}
	return places, nil
}
