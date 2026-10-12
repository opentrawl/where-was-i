// Command where-was-i says where you were, and which trips you took, from the places Apple Photos stored for your
// photos. Run it with no arguments to start.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ringsaturn/tzf"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"github.com/opentrawl/where-was-i/internal/places"
	"github.com/opentrawl/where-was-i/internal/render"
	"github.com/opentrawl/where-was-i/internal/sources"
)

const help = `where-was-i: where you were, and which trips you took, from the places Apple Photos stored for your photos.

Usage:
  where-was-i                         what's here, your latest trips and where to start
  where-was-i trips                   your trips, newest first (--all adds short stays)
  where-was-i trip LINK|DATE          one trip, day by day (LINK looks like trip:2024-05-03)
  where-was-i on DATE                 one day, stretch by stretch, in local time and UTC
  where-was-i days                    the latest days, one line each
  where-was-i in PLACE                every day in a country, region, town or landmark Apple names
  where-was-i years                   nights at home, with family and away, by year

Flags:
  --from DATE, --to DATE              limit trips, days and places to a range (DATE is YYYY-MM-DD)
  --limit N                           how many rows to show (default 10)
  --all                               trips: include stays with no morning photo away, often a long day out
  --format human|textproto            textproto prints the typed messages: every field, down to each photo
  --version                           print the version
  --library PATH                      a Photos.sqlite other than your system library
  --known-places PATH                 homes, family and work over time (default ~/.config/where-was-i/known-places.textproto)

Agents: output sent to another program is never wrapped, one row per line. Lists say how many rows they show
and how to see more. --format textproto gives every field.

Examples:
  where-was-i trips --from 2024-01-01
  where-was-i on 2024-05-07
  where-was-i in Scotland
`

// Version is set at release time with -ldflags "-X main.Version=...".
var Version = "dev"

type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

func main() {
	// When the output goes to another program rather than a terminal, don't wrap or fit it to a width: agents and
	// scripts get one row per line with every value whole. The shared renderer reads the width from COLUMNS.
	if _, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err != nil && os.Getenv("COLUMNS") == "" {
		os.Setenv("COLUMNS", "100000")
	}
	err := run(os.Args[1:], os.Stdout)
	var usage usageError
	switch {
	case err == nil:
	case errors.As(err, &usage):
		fmt.Fprintf(os.Stderr, "where-was-i: %s\nRun where-was-i --help for usage.\n", usage.message)
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "where-was-i:", err)
		os.Exit(1)
	}
}

type options struct {
	command, argument  string
	from, to           *places.Date
	limit              int
	all                bool
	format             string
	library, knownFile string
}

func parseDay(text string) (places.Date, error) {
	day, err := time.Parse("2006-01-02", strings.TrimPrefix(text, "trip:"))
	if err != nil {
		return places.Date{}, usageError{fmt.Sprintf("%q is not a date like 2024-05-07", text)}
	}
	return places.Date{Time: day}, nil
}

func parse(args []string) (options, error) {
	o := options{limit: 10, format: "human", library: sources.DefaultLibrary, knownFile: sources.DefaultKnownPlacesFile}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name == "h" || name == "help" {
			o.command = "help"
			return o, nil
		}
		if name == "version" {
			o.command = "version"
			return o, nil
		}
		if name == "all" {
			o.all = true
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return o, usageError{"--" + name + " needs a value"}
			}
			i++
			value = args[i]
		}
		switch name {
		case "from", "to":
			day, err := parseDay(value)
			if err != nil {
				return o, err
			}
			if name == "from" {
				o.from = &day
			} else {
				o.to = &day
			}
		case "limit":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return o, usageError{"--limit needs a positive number"}
			}
			o.limit = n
		case "format":
			if value != "human" && value != "textproto" {
				return o, usageError{"--format is human or textproto"}
			}
			o.format = value
		case "library":
			o.library = value
		case "known-places":
			o.knownFile = value
		default:
			return o, usageError{"unknown flag --" + name}
		}
	}
	if len(positional) > 0 {
		o.command = positional[0]
	}
	if len(positional) > 1 {
		o.argument = strings.Join(positional[1:], " ")
	}
	needsArgument := map[string]string{"trip": "a trip link or a date", "on": "a date", "in": "a place name"}
	if what, ok := needsArgument[o.command]; ok && o.argument == "" {
		return o, usageError{o.command + " needs " + what}
	}
	return o, nil
}

func run(args []string, out io.Writer) error {
	o, err := parse(args)
	if err != nil {
		return err
	}
	if o.command == "help" {
		_, err := io.WriteString(out, help)
		return err
	}
	if o.command == "version" {
		_, err := fmt.Fprintln(out, "where-was-i", Version)
		return err
	}
	switch o.command {
	case "", "trips", "trip", "on", "days", "in", "years":
	default:
		return usageError{fmt.Sprintf("unknown command %q", o.command)}
	}

	library, err := sources.OpenLibrary(o.library)
	if err != nil {
		return err
	}
	defer library.Close()
	photos, err := library.OwnPhotos()
	if err != nil {
		return err
	}
	appleTrips, err := library.AppleTrips()
	if err != nil {
		return err
	}
	known, err := sources.ReadKnownPlaces(o.knownFile)
	if err != nil {
		return err
	}
	zones, err := tzf.NewDefaultFinder()
	if err != nil {
		return fmt.Errorf("time-zone map: %w", err)
	}
	index := places.Build(photos, appleTrips, places.WithAppleHome(known, photos), zones.(*tzf.DefaultFinder))

	within := places.Everything
	if o.from != nil {
		within.First = *o.from
	}
	if o.to != nil {
		within.Last = *o.to
	}
	today := places.DateOf(time.Now())

	switch o.command {
	case "":
		overview := index.Overview(3, today)
		return write(out, o.format, overview, func() error { return render.Overview(out, overview) })
	case "trips":
		trips := index.Trips(within, o.limit, o.all)
		return write(out, o.format, trips, func() error { return render.Trips(out, trips, o.limit) })
	case "trip":
		day, err := parseDay(o.argument)
		if err != nil {
			return err
		}
		itinerary, err := index.FindTrip(day)
		if err != nil {
			return err
		}
		return write(out, o.format, itinerary, func() error { return render.Itinerary(out, itinerary) })
	case "on":
		day, err := parseDay(o.argument)
		if err != nil {
			return err
		}
		timeline := index.Day(day)
		return write(out, o.format, timeline, func() error { return render.Day(out, timeline) })
	case "days":
		days := index.Days(within, o.limit)
		return write(out, o.format, days, func() error { return render.Days(out, days, o.limit) })
	case "in":
		placeDays := index.InPlace(o.argument, within, o.limit)
		return write(out, o.format, placeDays, func() error { return render.PlaceDays(out, placeDays, o.limit) })
	default: // years
		years := index.Years(today)
		return write(out, o.format, years, func() error { return render.Years(out, years) })
	}
}

func write(out io.Writer, format string, result proto.Message, human func() error) error {
	if format == "human" {
		return human()
	}
	text, err := prototext.MarshalOptions{Multiline: true}.Marshal(result)
	if err != nil {
		return err
	}
	_, err = out.Write(text)
	return err
}
