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
	return strconv.Itoa(int(n)) + " " + many
}

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
	chosen := placeLabel(n.KnownPlaceKind, n.Place, true)
	if n.Source == wherewasiv1.NightSource_NIGHT_SOURCE_HOME_OR_FAMILY || n.NextMorningPlace == nil ||
		townName(n.LastEveningPlace) == townName(n.NextMorningPlace) {
		return chosen
	}
	return townName(n.LastEveningPlace) + " → " + townWithRegion(n.NextMorningPlace)
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

// route reads "Tokyo (4) → Hakone → Kyoto (4)".
func route(trip *wherewasiv1.Trip) string {
	var parts []string
	for _, stop := range trip.Stops {
		part := placeLabel(stop.KnownPlaceKind, stop.Place, false)
		if stop.Nights > 1 {
			part += " (" + strconv.Itoa(int(stop.Nights)) + ")"
		}
		parts = append(parts, part)
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
		rows = append(rows, []string{dateRange(t.Left, t.Back), strconv.Itoa(int(t.Nights)), strings.Join(t.Countries, ", "), route(t), t.Link})
	}
	return rows
}

var tripColumns = []trawlrender.TableColumn{
	{Header: "when", KeepWholeTokensWhenTerminalWidthAllows: true},
	{Header: "nights", AlignRight: true},
	{Header: "countries", Wrap: true},
	{Header: "route", Wrap: true},
	{Header: "link", NeverTruncateCellValues: true, CellValueIsTrawlCommandAction: true},
}

// Overview is the bare command: what's here, the latest trips, and where to go next.
func Overview(out io.Writer, o *wherewasiv1.Overview) error {
	w := &writer{out: out}
	if o.LocatedPhotos == 0 {
		w.line("No located photos in this library yet. Photos taken with Location Services on will show up here.")
		return w.err
	}
	intro := fmt.Sprintf("Where you were, from %s located photos, %s to %s.", trawlrender.FormatInteger(int64(o.LocatedPhotos)),
		goDate(o.FirstDay).Format("Jan 2006"), goDate(o.LastDay).Format("Jan 2006"))
	if o.CurrentHome != nil {
		if o.CurrentHome.Source == wherewasiv1.KnownPlaceSource_KNOWN_PLACE_SOURCE_APPLE_HOME {
			intro += fmt.Sprintf(" Home is where Apple Photos says, since %s; earlier homes go in ~/.config/%s/known-places.textproto.",
				goDate(o.CurrentHome.ValidFrom).Format("Jan 2006"), Command)
		} else {
			intro += " Home: " + o.CurrentHome.Label + "."
		}
	}
	w.line("%s", intro)
	if len(o.LatestTrips) > 0 {
		w.line("")
		w.line("Latest trips (%s in all)", plural(o.TotalTrips, "trip", "trips"))
		w.table(tripColumns, tripRows(o.LatestTrips))
	}
	w.line("")
	w.line("Start here")
	hints := [][2]string{{Command + " trips", "every trip"}}
	if len(o.LatestTrips) > 0 {
		hints = append(hints, [2]string{Command + " trip " + o.LatestTrips[0].Link, "one trip, day by day"})
	}
	hints = append(hints, [2]string{Command + " on " + isoDate(o.LastDay), "one day, hour by hour"},
		[2]string{Command + " days", "the latest days, one line each"},
		[2]string{Command + " in Japan", "every day in a place Apple names"},
		[2]string{Command + " years", "nights at home and away, by year"})
	var rows [][]string
	for _, h := range hints {
		rows = append(rows, []string{h[0], h[1]})
	}
	w.table([]trawlrender.TableColumn{{Header: "command", NeverTruncateCellValues: true}, {Header: "shows", Wrap: true}}, rows)
	w.line("")
	w.line("For agents: output sent to another program is never wrapped, one row per line. Add --format textproto to any command for every field, down to each photo.")
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
	w.line("Showing %d of %s.", len(trips.Trips), plural(trips.TotalTrips, "trip", "trips"))
	if trips.UnconfirmedTripsLeftOut > 0 {
		w.line("%s left out: no photo the next morning shows you woke up away, so they may be long days out.",
			plural(trips.UnconfirmedTripsLeftOut, "short stay", "short stays"))
		w.hint(fmt.Sprintf("Include them: %s trips --all", Command))
	}
	if trips.TotalTrips > int32(len(trips.Trips)) {
		w.hint(fmt.Sprintf("More: %s trips --limit %d", Command, min(int(trips.TotalTrips), limit*2)))
	}
	w.hint(fmt.Sprintf("One trip, day by day: %s trip %s", Command, trips.Trips[0].Link))
	return w.err
}

// Itinerary is one trip, day by day.
func Itinerary(out io.Writer, it *wherewasiv1.Itinerary) error {
	w := &writer{out: out}
	t := it.Trip
	summary := []string{dateRange(t.Left, t.Back), plural(t.Nights, "night", "nights"), plural(t.PhotoCount, "photo", "photos"),
		strings.Join(t.Countries, ", ")}
	w.line("%s", strings.Join(summary, " · "))
	if len(t.AppleTripTitles) > 0 {
		w.line("Apple Photos calls it: %s", strings.Join(t.AppleTripTitles, "; "))
	}
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
			return d.Night != nil && nightText(d.Night) == placeLabel(d.Night.KnownPlaceKind, d.Night.Place, true) && townName(d.Night.Place) == town
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
		if j-i > 1 {
			went += fmt.Sprintf(" (%d days)", j-i)
		}
		rows = append(rows, []string{when, went, strings.Join(named, ", "), nightText(day.Night)})
		i = j
	}
	w.table([]trawlrender.TableColumn{
		{Header: "day", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "went through", Wrap: true},
		{Header: "Apple named", Wrap: true},
		{Header: "night", Wrap: true},
	}, rows)
	w.line("")
	w.line("Apple named: parks, landmarks and places Apple Photos stored near your photos. Night: where the next morning's first photo was taken, or home when a photo that evening or the next day was.")
	w.line("")
	w.hint(fmt.Sprintf("One day in detail: %s on %s", Command, isoDate(it.Days[0].LocalDay)))
	w.hint(fmt.Sprintf("Every field, for agents: %s trip %s --format textproto", Command, t.Link))
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
	heading := []string{dayLabel(d.LocalDay), firstNonEmpty(first.Country, "")}
	if d.TripLink != "" {
		heading = append(heading, "on "+d.TripLink)
	}
	if d.MaxKmFromHome != nil && *d.MaxKmFromHome >= 1 {
		heading = append(heading, trawlrender.FormatInteger(int64(*d.MaxKmFromHome))+" km from home at the furthest")
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
		{Header: "Apple named", Wrap: true},
		{Header: "photos", AlignRight: true},
	}, rows)
	if n := d.Night; n != nil {
		w.line("")
		w.line("Night: %s (%s).", nightText(n), nightReason(n.Source))
	}
	w.line("")
	w.line("A day runs from 04:00 to 04:00 local time, so photos after midnight count with the evening before.")
	w.hint(fmt.Sprintf("Every photo, with its place fields and coordinates: %s on %s --format textproto", Command, isoDate(d.LocalDay)))
	next := localDay.AddDate(0, 0, 1).Format("2006-01-02")
	w.hint(fmt.Sprintf("Next day: %s on %s", Command, next))
	if d.TripLink != "" {
		w.hint(fmt.Sprintf("The whole trip: %s trip %s", Command, d.TripLink))
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

func nightReason(source wherewasiv1.NightSource) string {
	switch source {
	case wherewasiv1.NightSource_NIGHT_SOURCE_HOME_OR_FAMILY:
		return "a photo that evening or the next day was taken there"
	case wherewasiv1.NightSource_NIGHT_SOURCE_NEXT_MORNING:
		return "from the evening's last photo and the next morning's first"
	default:
		return "from the evening's last photo; there is none the next morning"
	}
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
		{Header: "went through", Wrap: true},
		{Header: "Apple named", Wrap: true},
		{Header: "night", Wrap: true},
	}, rows)
	w.line("")
	w.line("Showing %d of %s days with photos in this range.", len(days.Days), trawlrender.FormatInteger(int64(days.TotalDays)))
	if days.TotalDays > int32(len(days.Days)) {
		last := days.Days[len(days.Days)-1].LocalDay
		w.hint(fmt.Sprintf("Earlier: %s days --to %s --limit %d", Command, goDate(last).AddDate(0, 0, -1).Format("2006-01-02"), limit))
	}
	w.hint(fmt.Sprintf("One day in detail: %s on %s", Command, isoDate(days.Days[0].LocalDay)))
	return w.err
}

// PlaceDays lists the stays in the place asked for, newest first.
func PlaceDays(out io.Writer, p *wherewasiv1.PlaceDays, limit int) error {
	w := &writer{out: out}
	if p.TotalDays == 0 {
		w.line("No photo has \"%s\" in its place. Try a country, region, town or landmark as Apple Photos writes it.", p.PlaceAskedFor)
		return w.err
	}
	w.line("%s: %s days with photos, %s", p.PlaceAskedFor, trawlrender.FormatInteger(int64(p.TotalDays)), plural(p.TotalStays, "stay", "stays"))
	w.line("")
	var rows [][]string
	for _, stay := range p.Stays {
		var towns []string
		for _, town := range stay.TownsInOrder {
			towns = append(towns, townName(town))
		}
		rows = append(rows, []string{dateRange(stay.FirstDay, stay.LastDay), strconv.Itoa(int(stay.DaysWithPhotos)), strings.Join(towns, ", "),
			stay.MatchedValue + " (" + stay.MatchedField + ")", stay.TripLink})
	}
	w.table([]trawlrender.TableColumn{
		{Header: "when", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "days", AlignRight: true},
		{Header: "towns", Wrap: true},
		{Header: "matched", Wrap: true},
		{Header: "trip", NeverTruncateCellValues: true, CellValueIsTrawlCommandAction: true},
	}, rows)
	w.line("")
	w.line("Showing %d of %s.", len(p.Stays), plural(p.TotalStays, "stay", "stays"))
	if p.TotalStays > int32(len(p.Stays)) {
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
			away = append(away, fmt.Sprintf("%s %d", c.Country, c.Nights))
		}
		rows = append(rows, []string{strconv.Itoa(int(y.Year)), strconv.Itoa(int(y.NightsAtHome)), strconv.Itoa(int(y.NightsWithFamily)),
			strconv.Itoa(int(y.NightsNearHome)), strconv.Itoa(int(y.NightsAway)), strconv.Itoa(int(y.NightsHomeUnknown)),
			strconv.Itoa(int(y.NightsWithoutPhotos)), strings.Join(away, ", ")})
		unknown += y.NightsHomeUnknown
	}
	w.table([]trawlrender.TableColumn{
		{Header: "year"},
		{Header: "home", AlignRight: true},
		{Header: "family", AlignRight: true},
		{Header: "near home", AlignRight: true},
		{Header: "away", AlignRight: true},
		{Header: "home unknown", AlignRight: true},
		{Header: "no photos", AlignRight: true},
		{Header: "away, by country", Wrap: true},
	}, rows)
	w.line("")
	w.line("Nights by where they were spent. A night with no photo that evening or the next day isn't guessed.")
	if unknown > 0 {
		w.line("Home unknown: no home is known for those dates. Add earlier homes to ~/.config/%s/known-places.textproto.", Command)
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
