// Package sources reads records as they are stored: Apple Photos' private database and an optional known-places
// file. Nothing here interprets anything.
package sources

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"howett.net/plist"
)

// AppleEpoch is where Apple's Core Data timestamps count from.
var AppleEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

const appleNoCoordinate = -180.0

// DefaultLibrary is the system Photos library's database. It is opened immutable and never written.
var DefaultLibrary = filepath.Join(os.Getenv("HOME"), "Pictures/Photos Library.photoslibrary/database/Photos.sqlite")

// ErrNoFullDiskAccess means macOS refused to let this process read the Photos library. Only the user can grant it, so
// the message says exactly where.
var ErrNoFullDiskAccess = errors.New("macOS won't let this app read the Photos library. Turn on Full Disk Access for " +
	"the app you run this in (System Settings > Privacy & Security > Full Disk Access; " +
	`open "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles" goes straight there), then restart that app`)

// StoredPlace is Apple's own reverse geocoding of a photo, from ZREVERSELOCATIONDATA: the postal address fields, and the
// smallest named point of interest, water and island from Apple's typed list of surrounding places.
type StoredPlace struct {
	Country, CountryCode, State, County, Town, Neighbourhood, Street string
	PointOfInterest, Water, Island                                   string
	Address                                                          string
	Regions                                                          []string // Apple's administrative and informal areas, smallest first
	AppleMarksAsHome                                                 bool
}

// Apple's place types in sortedPlaceInfos (PLRevGeoMapItem).
const (
	applePlaceTypeAdministrativeArea = 2
	applePlaceTypeInformalRegion     = 3
	applePlaceTypeOcean              = 7
	applePlaceTypePointOfInterest    = 8
	applePlaceTypeWater              = 9
	applePlaceTypeIsland             = 11
)

// LibraryPhoto is one visible, untrashed photo or video the library's owner took.
type LibraryPhoto struct {
	UUID     string
	TakenUTC time.Time
	// UTCOffsetSeconds is the photo's own offset, else the one Apple inferred; nil when Apple has neither.
	UTCOffsetSeconds *int
	OffsetIsInferred bool
	Latitude         *float64
	Longitude        *float64
	Place            *StoredPlace
}

// AppleTrip is a trip Apple Photos detected (highlight kind 3), with Apple's own dates and title.
type AppleTrip struct {
	FirstDay, LastDay time.Time
	IsShortTrip       bool
	PlaceTitle        string
}

// Library is an open Photos database together with the names its schema uses on this macOS version.
type Library struct {
	db *sql.DB
	// assetTable is ZASSET on current macOS and ZGENERICASSET on older versions.
	assetTable string
	// sharedLibrary is true when the library has an iCloud Shared Library; contributorColumn then links contributors
	// to assets. Its name carries a number that changes between macOS versions, such as Z3LIBRARYSCOPEASSETCONTRIBUTORS.
	sharedLibrary     bool
	contributorColumn string
	inferredOffset    bool
}

// OpenLibrary opens a Photos database read-only and looks up the names its schema uses.
func OpenLibrary(path string) (*Library, error) {
	if file, err := os.Open(path); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil, ErrNoFullDiskAccess
		}
		return nil, fmt.Errorf("open Photos library %s: %w", path, err)
	} else {
		file.Close()
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro&immutable=1&_query_only=1")
	if err != nil {
		return nil, fmt.Errorf("open Photos database %s: %w", path, err)
	}
	// Keep SQLite's temporary storage in memory: sorting the photo list would otherwise need a temporary file, which
	// sandboxes that agents run in often refuse.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("pragma temp_store = memory"); err != nil {
		db.Close()
		return nil, fmt.Errorf("open Photos database %s: %w", path, err)
	}
	library := &Library{db: db}
	for _, table := range []string{"ZASSET", "ZGENERICASSET"} {
		if len(library.columns(table)) > 0 {
			library.assetTable = table
			break
		}
	}
	if library.assetTable == "" {
		db.Close()
		return nil, fmt.Errorf("%s does not look like a Photos library: no asset table", path)
	}
	library.sharedLibrary = library.columns(library.assetTable)["ZLIBRARYSCOPE"]
	for column := range library.columns("ZASSETCONTRIBUTOR") {
		if strings.HasSuffix(column, "LIBRARYSCOPEASSETCONTRIBUTORS") {
			library.contributorColumn = column
		}
	}
	library.inferredOffset = library.columns("ZADDITIONALASSETATTRIBUTES")["ZINFERREDTIMEZONEOFFSET"]
	return library, nil
}

func (l *Library) Close() error { return l.db.Close() }

func (l *Library) columns(table string) map[string]bool {
	found := map[string]bool{}
	rows, err := l.db.Query("select name from pragma_table_info(?)", table)
	if err != nil {
		return found
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			found[name] = true
		}
	}
	return found
}

func appleTime(seconds float64) time.Time {
	return AppleEpoch.Add(time.Duration(seconds * float64(time.Second)))
}

// OwnPhotos returns the photos in the owner's own library and their contributions to the iCloud Shared Library, oldest
// first. Shared albums and photos someone else added to the shared library are left out.
func (l *Library) OwnPhotos() ([]LibraryPhoto, error) {
	ownContribution := map[int64]bool{}
	if l.sharedLibrary && l.contributorColumn != "" {
		rows, err := l.db.Query(`select c.` + l.contributorColumn + ` from ZASSETCONTRIBUTOR c
			join ZSHAREPARTICIPANT p on p.Z_PK = c.ZPARTICIPANT where p.ZISCURRENTUSER = 1`)
		if err != nil {
			return nil, fmt.Errorf("read shared-library contributors: %w", err)
		}
		for rows.Next() {
			var asset sql.NullInt64
			if err := rows.Scan(&asset); err != nil {
				rows.Close()
				return nil, err
			}
			ownContribution[asset.Int64] = true
		}
		rows.Close()
	}

	inferred, inShared := "null", "0"
	if l.inferredOffset {
		inferred = "aa.ZINFERREDTIMEZONEOFFSET"
	}
	if l.sharedLibrary {
		inShared = "a.ZLIBRARYSCOPE is not null"
	}
	rows, err := l.db.Query(`select a.Z_PK, a.ZUUID, cast(a.ZDATECREATED as real), aa.ZTIMEZONEOFFSET, ` + inferred + `,
			a.ZLATITUDE, a.ZLONGITUDE, ` + inShared + `, aa.ZREVERSELOCATIONDATA
		from ` + l.assetTable + ` a join ZADDITIONALASSETATTRIBUTES aa on aa.ZASSET = a.Z_PK
		where a.ZTRASHEDSTATE = 0 and a.ZVISIBILITYSTATE = 0 and a.ZCOLLECTIONSHARE is null and a.ZDATECREATED is not null
		order by a.ZDATECREATED`)
	if err != nil {
		return nil, fmt.Errorf("read photos: %w", err)
	}
	defer rows.Close()

	var photos []LibraryPhoto
	for rows.Next() {
		var (
			pk                     int64
			uuid                   string
			created                float64
			offset, inferredOffset sql.NullInt64
			latitude, longitude    sql.NullFloat64
			inSharedLibrary        bool
			placeBlob              []byte
		)
		if err := rows.Scan(&pk, &uuid, &created, &offset, &inferredOffset, &latitude, &longitude,
			&inSharedLibrary, &placeBlob); err != nil {
			return nil, fmt.Errorf("read photo row: %w", err)
		}
		if inSharedLibrary && !ownContribution[pk] {
			continue
		}
		photo := LibraryPhoto{UUID: uuid, TakenUTC: appleTime(created)}
		switch {
		case offset.Valid:
			seconds := int(offset.Int64)
			photo.UTCOffsetSeconds = &seconds
		case inferredOffset.Valid:
			seconds := int(inferredOffset.Int64)
			photo.UTCOffsetSeconds, photo.OffsetIsInferred = &seconds, true
		}
		if latitude.Valid && longitude.Valid && latitude.Float64 != appleNoCoordinate {
			photo.Latitude, photo.Longitude = &latitude.Float64, &longitude.Float64
		}
		if len(placeBlob) > 0 {
			photo.Place = decodeStoredPlace(placeBlob)
		}
		photos = append(photos, photo)
	}
	return photos, rows.Err()
}

// decodeStoredPlace reads Apple's NSKeyedArchiver plist (root object PLRevGeoLocationInfo). It returns nil for a blob
// it cannot read, which Apple writes for a few photos.
func decodeStoredPlace(blob []byte) *StoredPlace {
	var archive map[string]any
	if _, err := plist.Unmarshal(blob, &archive); err != nil {
		return nil
	}
	objects, _ := archive["$objects"].([]any)
	deref := func(value any) any {
		if uid, ok := value.(plist.UID); ok && int(uid) < len(objects) {
			return objects[uid]
		}
		return value
	}
	text := func(value any) string {
		if s, ok := deref(value).(string); ok && s != "$null" {
			return s
		}
		return ""
	}
	var info map[string]any
	for _, object := range objects {
		if candidate, ok := object.(map[string]any); ok {
			if _, has := candidate["isHome"]; has {
				info = candidate
				break
			}
		}
	}
	if info == nil {
		return nil
	}
	place := &StoredPlace{Address: text(info["addressString"])}
	if isHome, ok := deref(info["isHome"]).(bool); ok {
		place.AppleMarksAsHome = isHome
	}
	if postal, ok := deref(info["postalAddress"]).(map[string]any); ok {
		place.Country, place.CountryCode = text(postal["_country"]), text(postal["_ISOCountryCode"])
		place.State, place.County = text(postal["_state"]), text(postal["_subAdministrativeArea"])
		place.Town, place.Neighbourhood, place.Street = text(postal["_city"]), text(postal["_subLocality"]), text(postal["_street"])
	}
	if place.CountryCode == "" {
		place.CountryCode = text(info["countryCode"])
	}
	if mapItem, ok := deref(info["mapItem"]).(map[string]any); ok {
		if sorted, ok := deref(mapItem["sortedPlaceInfos"]).(map[string]any); ok {
			items, _ := sorted["NS.objects"].([]any)
			for _, raw := range items { // smallest first, so the first of each type is the most specific
				item, ok := deref(raw).(map[string]any)
				if !ok {
					continue
				}
				name := text(item["name"])
				switch placeType(deref(item["placeType"])) {
				case applePlaceTypePointOfInterest:
					place.PointOfInterest = firstNonEmpty(place.PointOfInterest, name)
				case applePlaceTypeWater, applePlaceTypeOcean:
					place.Water = firstNonEmpty(place.Water, name)
				case applePlaceTypeIsland:
					place.Island = firstNonEmpty(place.Island, name)
				case applePlaceTypeAdministrativeArea, applePlaceTypeInformalRegion:
					if name != "" && !contains(place.Regions, name) {
						place.Regions = append(place.Regions, name)
					}
				}
			}
		}
	}
	return place
}

func placeType(value any) int {
	switch v := value.(type) {
	case uint64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func firstNonEmpty(current, candidate string) string {
	if current != "" {
		return current
	}
	return candidate
}

// AppleTrips returns the trips Apple Photos detected, oldest first, or none when the library has no highlights.
func (l *Library) AppleTrips() ([]AppleTrip, error) {
	if len(l.columns("ZPHOTOSHIGHLIGHT")) == 0 {
		return nil, nil
	}
	rows, err := l.db.Query(`select ZTYPE, coalesce(ZSUBTITLE, ''), cast(ZSTARTDATE as real), cast(ZENDDATE as real)
		from ZPHOTOSHIGHLIGHT where ZKIND = 3 and ZSTARTDATE is not null order by ZSTARTDATE`)
	if err != nil {
		return nil, fmt.Errorf("read Apple trips: %w", err)
	}
	defer rows.Close()
	var trips []AppleTrip
	for rows.Next() {
		var kind int
		var title string
		var start, end float64
		if err := rows.Scan(&kind, &title, &start, &end); err != nil {
			return nil, err
		}
		trips = append(trips, AppleTrip{FirstDay: appleTime(start), LastDay: appleTime(end), IsShortTrip: kind != 1, PlaceTitle: title})
	}
	return trips, rows.Err()
}
