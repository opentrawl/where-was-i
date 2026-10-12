// Package render prints the typed results for people, with OpenTrawl's shared terminal renderer: width-aware tables,
// wrapping and next-step hints. Agents that want the typed messages use --format textproto instead.
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	trawlrender "github.com/opentrawl/opentrawl/trawlkit/render"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
)

// Command is the name people type.
const Command = "where-was-i"

func goDate(d *wherewasiv1.CalendarDate) time.Time {
	return time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.UTC)
}

func isoDate(d *wherewasiv1.CalendarDate) string { return goDate(d).Format("2006-01-02") }

// dateRange reads "28 Apr – 3 May 2024", "15 – 18 Feb 2024" or "29 Dec 2023 – 2 Jan 2024".
func dateRange(from, to *wherewasiv1.CalendarDate) string {
	a, b := goDate(from), goDate(to)
	switch {
	case a.Year() != b.Year():
		return a.Format("2 Jan 2006") + " – " + b.Format("2 Jan 2006")
	case a.Month() != b.Month():
		return a.Format("2 Jan") + " – " + b.Format("2 Jan 2006")
	case a.Day() != b.Day():
		return a.Format("2") + " – " + b.Format("2 Jan 2006")
	}
	return b.Format("2 Jan 2006")
}

func dayLabel(d *wherewasiv1.CalendarDate) string { return goDate(d).Format("Mon 2 Jan 2006") }

func shortDayLabel(d *wherewasiv1.CalendarDate) string { return goDate(d).Format("Mon 2 Jan") }

func townName(p *wherewasiv1.Place) string {
	for _, v := range []string{p.GetTown(), p.GetCounty(), p.GetState(), p.GetCountry()} {
		if v != "" {
			return v
		}
	}
	return "somewhere unnamed"
}

// townWithRegion is the town and Apple's state, for example "Portree, Scotland" or "Kyoto, Kyoto".
func townWithRegion(p *wherewasiv1.Place) string {
	name := townName(p)
	if p.GetState() != "" && p.GetState() != name {
		return name + ", " + p.GetState()
	}
	if p.GetCountry() != "" && p.GetCountry() != name {
		return name + ", " + p.GetCountry()
	}
	return name
}

func clock(t *wherewasiv1.LocalTime) time.Time {
	return t.Utc.AsTime().In(time.FixedZone("", int(t.UtcOffsetSeconds)))
}

func offsetName(seconds int32) string {
	return "UTC" + time.Unix(0, 0).In(time.FixedZone("", int(seconds))).Format("-07:00")
}

func timeSpan(start, end *wherewasiv1.LocalTime, utc bool) string {
	format := func(t *wherewasiv1.LocalTime) string {
		if utc {
			return t.Utc.AsTime().UTC().Format("15:04")
		}
		return clock(t).Format("15:04")
	}
	if start.Utc.AsTime().Equal(end.Utc.AsTime()) || format(start) == format(end) {
		return format(start)
	}
	return format(start) + "–" + format(end)
}

func plural(n int32, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return trawlrender.FormatInteger(int64(n)) + " " + many
}

// keepTogether stops the renderer wrapping inside one entry, such as "Inverness, 2 nights" or "France 6", while
// still letting it wrap between entries. The renderer turns this marker back into a space.
func keepTogether(entry string) string { return strings.ReplaceAll(entry, " ", "\ue000") }

// tripDay is the date part of a trip link, which is also what `trip` accepts: "trip:2024-05-03" -> "2024-05-03".
func tripDay(link string) string { return strings.TrimPrefix(link, "trip:") }

var kindWord = map[wherewasiv1.KnownPlaceKind]string{
	wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_HOME:   "home",
	wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_FAMILY: "family",
	wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_WORK:   "work",
	wherewasiv1.KnownPlaceKind_KNOWN_PLACE_KIND_OTHER:  "known place",
}

// placeLabel is "home", "family" or "work" at a known place, else the town.
func placeLabel(kind wherewasiv1.KnownPlaceKind, place *wherewasiv1.Place, withRegion bool) string {
	if word, ok := kindWord[kind]; ok {
		return word
	}
	if withRegion {
		return townWithRegion(place)
	}
	return townName(place)
}

// nightText is where the night was spent: one town, or "Hounslow → Ota, Tokyo" when the evening's last photo and
// the next morning's first were in different towns, as on an overnight flight.
func nightText(n *wherewasiv1.Night) string {
	if n == nil {
		return ""
	}
	if n.Travelling && n.LastEveningPlace != nil && n.NextMorningPlace != nil {
		return "travelling, " + townName(n.LastEveningPlace) + " to " + townName(n.NextMorningPlace)
	}
	chosen := placeLabel(n.KnownPlaceKind, n.Place, false)
	// Show both ends only when the night was placed by the next morning's photo; otherwise the text would disagree
	// with the place the night counts as.
	if n.Source != wherewasiv1.NightSource_NIGHT_SOURCE_NEXT_MORNING || n.NextMorningPlace == nil ||
		townName(n.LastEveningPlace) == townName(n.NextMorningPlace) {
		return chosen
	}
	return townName(n.LastEveningPlace) + " → " + townName(n.NextMorningPlace)
}

// townNames are the named towns in order, leaving out photos Apple placed nowhere (often taken in flight), with
// repeats that this leaves side by side merged.
func townNames(places []*wherewasiv1.Place) []string {
	var names []string
	for _, p := range places {
		if p.GetTown() == "" && p.GetCounty() == "" && p.GetState() == "" && p.GetCountry() == "" {
			continue
		}
		if name := townName(p); len(names) == 0 || names[len(names)-1] != name {
			names = append(names, name)
		}
	}
	return names
}

func namedTowns(d *wherewasiv1.ItineraryDay) string {
	return strings.Join(townNames(d.TownsInOrder), " → ")
}

// route reads "travelling, 1 night → Tokyo, 4 nights → Hakone, 1 night", so the nights add up to the trip's.
func route(trip *wherewasiv1.Trip) string {
	var parts []string
	for _, stop := range trip.Stops {
		where := placeLabel(stop.KnownPlaceKind, stop.Place, false)
		switch {
		case stop.Travelling:
			where = "travelling"
		case stop.NoPhotos:
			where = "no photos"
		}
		parts = append(parts, keepTogether(where+", "+plural(stop.Nights, "night", "nights")))
	}
	return strings.Join(parts, " → ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

type writer struct {
	out io.Writer
	err error
}

func (w *writer) line(format string, args ...any) {
	if w.err != nil {
		return
	}
	text := fmt.Sprintf(format, args...)
	if text == "" {
		_, w.err = fmt.Fprintln(w.out)
		return
	}
	for _, line := range trawlrender.Wrap(text, trawlrender.OutputWidth(w.out)) {
		if _, w.err = fmt.Fprintln(w.out, line); w.err != nil {
			return
		}
	}
}

func (w *writer) table(columns []trawlrender.TableColumn, rows [][]string) {
	if w.err == nil && len(rows) > 0 {
		w.err = trawlrender.WriteTable(w.out, columns, rows)
	}
}

func (w *writer) hint(text string) {
	if w.err == nil {
		w.err = trawlrender.WriteTrawlCommandHint(w.out, text)
	}
}

func tripRows(trips []*wherewasiv1.Trip) [][]string {
	var rows [][]string
	for _, t := range trips {
		rows = append(rows, []string{dateRange(t.Left, t.Back), strconv.Itoa(int(t.Nights)), strings.Join(t.Countries, ", "), route(t)})
	}
	return rows
}

var tripColumns = []trawlrender.TableColumn{
	{Header: "when", KeepWholeTokensWhenTerminalWidthAllows: true},
	{Header: "nights", AlignRight: true},
	{Header: "countries", Wrap: true},
	{Header: "where you slept", Wrap: true},
}

// Overview is the bare command: what's here, the latest trips, and where to go next.
func Overview(out io.Writer, o *wherewasiv1.Overview) error {
	w := &writer{out: out}
	if o.LocatedPhotos == 0 {
		w.line("No located photos in this library yet. Photos taken with Location Services on will show up here.")
		return w.err
	}
	w.line("Where you were, from %s photos with a location, %s to %s.", trawlrender.FormatInteger(int64(o.LocatedPhotos)),
		goDate(o.FirstDay).Format("Jan 2006"), goDate(o.LastDay).Format("Jan 2006"))
	if o.CurrentHome != nil && o.CurrentHome.Source == wherewasiv1.KnownPlaceSource_KNOWN_PLACE_SOURCE_APPLE_HOME {
		w.line("Apple Photos has your home only from %s, so trips before then are missing. To find them, add your earlier homes to ~/.config/%s/known-places.textproto.",
			goDate(o.CurrentHome.ValidFrom).Format("Jan 2006"), Command)
	}
	if len(o.LatestTrips) > 0 {
		w.line("")
		w.line("Latest trips, %d of %s", len(o.LatestTrips), trawlrender.FormatInteger(int64(o.TotalTrips)))
		w.table(tripColumns, tripRows(o.LatestTrips))
	}
	w.line("")
	w.line("Start here")
	hints := [][2]string{{Command + " trips", "your trips, newest first"}}
	if len(o.LatestTrips) > 0 {
		hints = append(hints, [2]string{Command + " trip " + tripDay(o.LatestTrips[0].Link), "one trip, day by day"})
	}
	hints = append(hints, [2]string{Command + " on " + isoDate(o.LastDay), "one day in detail"},
		[2]string{Command + " days", "the latest days, one line each"},
		[2]string{Command + " in Japan", "your visits to a country, region, town or landmark"},
		[2]string{Command + " years", "nights at home and away, by year"})
	var rows [][]string
	for _, h := range hints {
		rows = append(rows, []string{h[0], h[1]})
	}
	w.table([]trawlrender.TableColumn{{Header: "command", NeverTruncateCellValues: true}, {Header: "shows", Wrap: true}}, rows)
	w.line("")
	w.line("Agents: add --format textproto to any command for every field.")
	return w.err
}

// Trips lists trips, newest first.
func Trips(out io.Writer, trips *wherewasiv1.Trips, limit int) error {
	w := &writer{out: out}
	if len(trips.Trips) == 0 {
		w.line("No trips found: no nights more than 25 km from home in this range.")
		return w.err
	}
	w.table(tripColumns, tripRows(trips.Trips))
	w.line("")
	if trips.TotalTrips > int32(len(trips.Trips)) {
		w.line("Showing %d of %s.", len(trips.Trips), plural(trips.TotalTrips, "trip", "trips"))
	}
	if trips.UnconfirmedTripsLeftOut > 0 {
		w.line("%s left out, because no photo shows you waking up away.", plural(trips.UnconfirmedTripsLeftOut, "one-night stay", "one-night stays"))
		w.hint(fmt.Sprintf("Show them: %s trips --all", Command))
	}
	if trips.TotalTrips > int32(len(trips.Trips)) {
		w.hint(fmt.Sprintf("More: %s trips --limit %d", Command, min(int(trips.TotalTrips), limit*2)))
	}
	w.hint(fmt.Sprintf("One trip, day by day: %s trip %s (any date on the trip)", Command, tripDay(trips.Trips[0].Link)))
	return w.err
}

// Itinerary is one trip, day by day.
func Itinerary(out io.Writer, it *wherewasiv1.Itinerary) error {
	w := &writer{out: out}
	t := it.Trip
	summary := []string{dateRange(t.Left, t.Back), plural(t.Nights, "night away", "nights away"), plural(t.PhotoCount, "photo", "photos"),
		strings.Join(t.Countries, ", ")}
	w.line("%s", strings.Join(summary, " · "))
	if t.NightsWithoutPhotos > 0 {
		w.line("%s without photos, so where you slept then isn't known.", plural(t.NightsWithoutPhotos, "night", "nights"))
	}
	w.line("")
	var rows [][]string
	for i := 0; i < len(it.Days); {
		day := it.Days[i]
		towns := townNames(day.TownsInOrder)
		// Consecutive days spent in one town, sleeping there, read as one block: "Thu 9 – Sat 11 May".
		j, named := i+1, append([]string{}, day.PointsOfInterest...)
		sleptInTown := func(d *wherewasiv1.ItineraryDay, town string) bool {
			return d.Night != nil && nightText(d.Night) == placeLabel(d.Night.KnownPlaceKind, d.Night.Place, false) && townName(d.Night.Place) == town
		}
		if len(towns) == 1 && sleptInTown(day, towns[0]) {
			for j < len(it.Days) {
				next := it.Days[j]
				if namedTowns(next) != towns[0] || !sleptInTown(next, towns[0]) {
					break
				}
				for _, poi := range next.PointsOfInterest {
					named = appendNew(named, poi)
				}
				j++
			}
		}
		when := shortDayLabel(day.LocalDay)
		if j-i > 1 {
			when = goDate(day.LocalDay).Format("Mon 2") + " – " + shortDayLabel(it.Days[j-1].LocalDay)
		}
		went := strings.Join(towns, " → ")
		if len(towns) == 0 {
			went = "no photos"
		}
		rows = append(rows, []string{when, went, strings.Join(named, ", "), nightText(day.Night)})
		i = j
	}
	w.table([]trawlrender.TableColumn{
		{Header: "day", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "places", Wrap: true},
		{Header: "points of interest", Wrap: true},
		{Header: "slept in", Wrap: true},
	}, rows)
	w.line("")
	w.hint(fmt.Sprintf("One day in detail: %s on %s", Command, isoDate(it.Days[0].LocalDay)))
	w.hint(fmt.Sprintf("Every field: %s trip %s --format textproto", Command, tripDay(t.Link)))
	return w.err
}

// Day is one day, stretch by stretch, in local time and UTC.
func Day(out io.Writer, d *wherewasiv1.DayTimeline) error {
	w := &writer{out: out}
	if len(d.Visits) == 0 {
		w.line("No located photos on %s.", dayLabel(d.LocalDay))
		return w.err
	}
	var offsets []string
	for _, v := range d.Visits {
		offsets = appendNew(offsets, offsetName(v.Start.UtcOffsetSeconds))
	}
	first := d.Visits[0].Place
	var countries []string
	for _, v := range d.Visits {
		countries = appendNew(countries, v.Place.Country)
	}
	heading := []string{dayLabel(d.LocalDay), strings.Join(countries, ", ")}
	if d.TripLink != "" {
		if left, err := time.Parse("2006-01-02", tripDay(d.TripLink)); err == nil {
			heading = append(heading, fmt.Sprintf("day %d of the trip from %s", int(goDate(d.LocalDay).Sub(left).Hours()/24)+1, left.Format("2 Jan")))
		}
	}
	if d.MaxKmFromHome != nil && *d.MaxKmFromHome >= 1 {
		heading = append(heading, "up to "+trawlrender.FormatInteger(int64(*d.MaxKmFromHome))+" km from home")
	}
	if len(offsets) == 1 {
		heading = append(heading, "local time "+offsets[0])
	}
	w.line("%s", strings.Join(nonEmpty(heading), " · "))
	w.line("")
	localDay := goDate(d.LocalDay)
	var rows [][]string
	previous := &wherewasiv1.Place{Country: first.Country, State: first.State}
	for _, v := range d.Visits {
		where := []string{placeLabel(v.KnownPlaceKind, v.Place, false)}
		if len(v.Neighbourhoods) > 0 {
			where = append(where, strings.Join(v.Neighbourhoods, ", "))
		}
		if v.Place.State != previous.State || v.Place.Country != previous.Country {
			where = append(where, v.Place.State)
		}
		if v.Place.Country != previous.Country {
			where = append(where, v.Place.Country)
		}
		previous = v.Place
		local := timeSpan(v.Start, v.End, false)
		if len(offsets) > 1 {
			local += " " + offsetName(v.Start.UtcOffsetSeconds)
		}
		utc := timeSpan(v.Start, v.End, true)
		startUTC, endUTC := v.Start.Utc.AsTime().UTC(), v.End.Utc.AsTime().UTC()
		switch {
		case startUTC.YearDay() != endUTC.YearDay():
			utc = startUTC.Format("15:04") + "–" + endUTC.Format("2 Jan 15:04")
			if startUTC.YearDay() != localDay.YearDay() {
				utc = startUTC.Format("2 Jan ") + utc
			}
		case startUTC.Year() != localDay.Year() || startUTC.YearDay() != localDay.YearDay():
			utc = startUTC.Format("2 Jan") + " " + utc
		}
		rows = append(rows, []string{local, utc, strings.Join(nonEmpty(where), " · "), strings.Join(v.PointsOfInterest, ", "),
			strconv.Itoa(int(v.PhotoCount))})
	}
	w.table([]trawlrender.TableColumn{
		{Header: "local", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "utc", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "where", Wrap: true},
		{Header: "points of interest", Wrap: true},
		{Header: "photos", AlignRight: true},
	}, rows)
	if n := d.Night; n != nil {
		w.line("")
		if n.Travelling {
			w.line("Spent the night %s.", nightText(n))
		} else {
			w.line("Slept in %s.", nightText(n))
		}
	}
	w.line("")
	w.hint(fmt.Sprintf("Every photo, with coordinates: %s on %s --format textproto", Command, isoDate(d.LocalDay)))
	next := localDay.AddDate(0, 0, 1).Format("2006-01-02")
	w.hint(fmt.Sprintf("Next day: %s on %s", Command, next))
	if d.TripLink != "" {
		w.hint(fmt.Sprintf("The whole trip: %s trip %s", Command, tripDay(d.TripLink)))
	}
	return w.err
}

func nonEmpty(values []string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// Days lists days, newest first, one line each.
func Days(out io.Writer, days *wherewasiv1.Days, limit int) error {
	w := &writer{out: out}
	var rows [][]string
	for _, day := range days.Days {
		towns := townNames(day.TownsInOrder)
		rows = append(rows, []string{shortDayLabel(day.LocalDay) + goDate(day.LocalDay).Format(" 2006"), strings.Join(towns, " → "),
			strings.Join(day.PointsOfInterest, ", "), nightText(day.Night)})
	}
	if len(rows) == 0 {
		w.line("No located photos in this range.")
		return w.err
	}
	w.table([]trawlrender.TableColumn{
		{Header: "day", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "places", Wrap: true},
		{Header: "points of interest", Wrap: true},
		{Header: "slept in", Wrap: true},
	}, rows)
	w.line("")
	w.line("Showing %d of %s days with photos.", len(days.Days), trawlrender.FormatInteger(int64(days.TotalDays)))
	if days.TotalDays > int32(len(days.Days)) {
		last := days.Days[len(days.Days)-1].LocalDay
		w.hint(fmt.Sprintf("Earlier: %s days --to %s --limit %d", Command, goDate(last).AddDate(0, 0, -1).Format("2006-01-02"), limit))
	}
	w.hint(fmt.Sprintf("One day in detail: %s on %s", Command, isoDate(days.Days[0].LocalDay)))
	return w.err
}

// PlaceDays lists the visits to the place asked for, newest first.
func PlaceDays(out io.Writer, p *wherewasiv1.PlaceDays, limit int) error {
	w := &writer{out: out}
	if p.TotalDays == 0 {
		w.line("No photo has \"%s\" in its place. Try a country, region, town or landmark as Apple Photos writes it.", p.PlaceAskedFor)
		return w.err
	}
	name := p.PlaceAskedFor
	if len(p.Stays) > 0 && !strings.EqualFold(p.Stays[0].MatchedValue, name) {
		name = p.Stays[0].MatchedValue
	}
	w.line("%s: %s days with photos, %s", name, trawlrender.FormatInteger(int64(p.TotalDays)), plural(p.TotalStays, "visit", "visits"))
	w.line("")
	var rows [][]string
	for _, stay := range p.Stays {
		var towns []string
		for _, town := range stay.TownsInOrder {
			towns = append(towns, townName(town))
		}
		rows = append(rows, []string{dateRange(stay.FirstDay, stay.LastDay), strconv.Itoa(int(stay.DaysWithPhotos)), strings.Join(towns, " → ")})
	}
	w.table([]trawlrender.TableColumn{
		{Header: "when", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "days", AlignRight: true},
		{Header: "places", Wrap: true},
	}, rows)
	if p.TotalStays > int32(len(p.Stays)) {
		w.line("")
		w.line("Showing %d of %s.", len(p.Stays), plural(p.TotalStays, "visit", "visits"))
		w.hint(fmt.Sprintf("More: %s in %q --limit %d", Command, p.PlaceAskedFor, min(int(p.TotalStays), limit*2)))
	}
	return w.err
}

// Years lists nights by year, newest first.
func Years(out io.Writer, years *wherewasiv1.Years) error {
	w := &writer{out: out}
	var rows [][]string
	var unknown int32
	for i := len(years.Years) - 1; i >= 0; i-- {
		y := years.Years[i]
		var away []string
		for _, c := range y.AwayByCountry {
			away = append(away, keepTogether(fmt.Sprintf("%s %d", c.Country, c.Nights)))
		}
		rows = append(rows, []string{strconv.Itoa(int(y.Year)), strconv.Itoa(int(y.NightsAtHome)), strconv.Itoa(int(y.NightsWithFamily)),
			strconv.Itoa(int(y.NightsNearHome)), strconv.Itoa(int(y.NightsAway)), strconv.Itoa(int(y.NightsHomeUnknown)),
			strconv.Itoa(int(y.NightsWithoutPhotos)), strings.Join(away, ", ")})
		unknown += y.NightsHomeUnknown
	}
	w.line("Nights each year, by where you spent them")
	w.line("")
	w.table([]trawlrender.TableColumn{
		{Header: "year"},
		{Header: "at home", AlignRight: true},
		{Header: "with family", AlignRight: true},
		{Header: "near home", AlignRight: true},
		{Header: "away", AlignRight: true},
		{Header: "home unknown", AlignRight: true},
		{Header: "no photos", AlignRight: true},
		{Header: "away, by country", Wrap: true},
	}, rows)
	w.line("")
	w.line("Near home: within 25 km of home but not at it. No photos: none that evening or the next morning.")
	if unknown > 0 {
		w.line("")
		w.line("Home unknown: before the first home where-was-i knows about. Add earlier homes to ~/.config/%s/known-places.textproto.", Command)
	}
	return w.err
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
