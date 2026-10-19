// Package render prints the typed results for people, with OpenTrawl's shared terminal renderer: width-aware tables,
// wrapping and next-step hints. Agents that want the typed messages use --format textproto instead.
package render

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	trawlrender "github.com/opentrawl/opentrawl/trawlkit/render"

	wherewasiv1 "github.com/opentrawl/where-was-i/gen/wherewasi/v1"
	"github.com/opentrawl/where-was-i/internal/places"
)

// Command is the name people type.
const Command = "where-was-i"

// RepeatedFlags are the flags the user ran with that every printed command repeats, such as {"known-places", PATH},
// so following a printed command keeps the same homes, library and range.
var RepeatedFlags [][2]string

// flagsOnlySomeCommandsTake are the flags that change only these commands' output, so others do not repeat them.
var flagsOnlySomeCommandsTake = map[string][]string{"from": {"trips", "days", "in"}, "to": {"trips", "days", "in"}, "all": {"trips"}}

// command is a where-was-i command line to print for the user to run next. A flag the arguments set themselves, such
// as the --to of an "Earlier" command, is not repeated.
func command(arguments string) string {
	line := Command + " " + arguments
	name, _, _ := strings.Cut(arguments, " ")
	for _, flag := range RepeatedFlags {
		if strings.Contains(" "+arguments+" ", " --"+flag[0]+" ") {
			continue
		}
		if takers, onlySome := flagsOnlySomeCommandsTake[flag[0]]; onlySome && !slices.Contains(takers, name) {
			continue
		}
		line += " --" + flag[0]
		if flag[1] != "" {
			line += " " + flag[1]
		}
	}
	return line
}

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
	return "a place Apple Photos has not named yet"
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

// nightText is where the night was spent: home, the town, or "travelling, Ota to Hounslow" when the
// evening's last photo and the next morning's first are far apart, as on an overnight flight.
func nightText(n *wherewasiv1.Night) string {
	if n == nil {
		return ""
	}
	if n.Travelling && n.LastEveningPlace != nil && n.NextMorningPlace != nil {
		return "travelling, " + townName(n.LastEveningPlace) + " to " + townName(n.NextMorningPlace)
	}
	return placeLabel(n.KnownPlaceKind, n.Place, false)
}

// nightDoubt says where both photos around a night were taken when they are in different towns, since either could
// be where the night was spent: "The evening's last photo was in Hakone at 23:10, and the next morning's
// first in Odawara at 10:40, 14 km apart."
func nightDoubt(n *wherewasiv1.Night) string {
	if n.Travelling || n.NextMorningPlace == nil || n.KmBetweenEveningAndMorning == nil ||
		townName(n.LastEveningPlace) == townName(n.NextMorningPlace) || n.Source == wherewasiv1.NightSource_NIGHT_SOURCE_HOME_OR_FAMILY {
		return ""
	}
	clock := func(t *wherewasiv1.LocalTime) string {
		if at, err := time.Parse(time.RFC3339, t.GetLocal()); err == nil {
			return at.Format("15:04")
		}
		return ""
	}
	return fmt.Sprintf("The evening's last photo was in %s at %s, and the next morning's first in %s at %s, %s km apart.",
		townName(n.LastEveningPlace), clock(n.LastEveningTime), townName(n.NextMorningPlace), clock(n.NextMorningTime),
		trawlrender.FormatInteger(int64(*n.KmBetweenEveningAndMorning+0.5)))
}

// nightIn is nightText, except that a night inside one of the day's places, such as a national park you went to,
// reads as that place rather than its postal town.
func nightIn(n *wherewasiv1.Night, places []string) string {
	if n != nil && !n.Travelling && n.PointOfInterest != "" && slices.Contains(places, n.PointOfInterest) {
		return n.PointOfInterest
	}
	return nightText(n)
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

// tripDates is "3 – 17 May 2024", or "3 May 2024 – now" for a trip you are still on.
func tripDates(t *wherewasiv1.Trip) string {
	if t.Ongoing {
		return goDate(t.Left).Format("2 Jan 2006") + " – now"
	}
	return dateRange(t.Left, t.Back)
}

// route is the places of a trip with most photos, in the order you went, saying how many it leaves out.
func route(trip *wherewasiv1.Trip) string {
	var parts []string
	for _, name := range trip.Route {
		parts = append(parts, keepTogether(name))
	}
	text := strings.Join(parts, " → ")
	if left := len(trip.PlacesInOrder) - len(trip.Route); left > 0 {
		text += fmt.Sprintf(", and %d more", left)
	}
	return text
}

// wentTo reads "Kyoto (Fushimi Inari Taisha, Kiyomizu-dera) → Nara (Todai-ji)": the places one or
// more days went to, each with the spots you stopped at most in it over those days.
func wentTo(days []*wherewasiv1.ItineraryDay) string {
	var order []string
	spotPhotos := map[string]map[string]int32{}
	var spotOrder = map[string][]string{}
	for _, day := range days {
		for _, place := range day.Places {
			if !place.WentTo {
				continue
			}
			if spotPhotos[place.Name] == nil {
				order = append(order, place.Name)
				spotPhotos[place.Name] = map[string]int32{}
			}
			for _, spot := range place.Spots {
				if _, seen := spotPhotos[place.Name][spot.Name]; !seen {
					spotOrder[place.Name] = append(spotOrder[place.Name], spot.Name)
				}
				spotPhotos[place.Name][spot.Name] += spot.PhotoCount
			}
		}
	}
	var parts []string
	for _, name := range order {
		photos := spotPhotos[name]
		spots := spotOrder[name]
		slices.SortStableFunc(spots, func(a, b string) int { return int(photos[b] - photos[a]) })
		var shown []string
		for _, spot := range spots {
			if len(shown) == spotsPerPlace || photos[spot] < 3 {
				break
			}
			shown = append(shown, spot)
		}
		if len(shown) > 0 {
			name += " (" + strings.Join(shown, ", ") + ")"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, " → ")
}

// placesWentTo are the names of the places a day went to, in order.
func placesWentTo(day *wherewasiv1.ItineraryDay) []string {
	var names []string
	for _, place := range day.Places {
		if place.WentTo {
			names = append(names, place.Name)
		}
	}
	return names
}

const spotsPerPlace = 4

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
		rows = append(rows, []string{tripDates(t), strconv.Itoa(int(t.Nights)), strings.Join(t.Countries, ", "), route(t)})
	}
	return rows
}

var tripColumns = []trawlrender.TableColumn{
	{Header: "when", KeepWholeTokensWhenTerminalWidthAllows: true},
	{Header: "nights", AlignRight: true},
	{Header: "countries", Wrap: true},
	{Header: "where you went", Wrap: true},
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
	hints := [][2]string{{command("trips"), "your trips, newest first"}}
	if len(o.LatestTrips) > 0 {
		hints = append(hints, [2]string{command("trip " + tripDay(o.LatestTrips[0].Link)), "one trip, day by day"})
	}
	hints = append(hints, [2]string{command("on " + isoDate(o.LastDay)), "one day in detail"},
		[2]string{command("days"), "the latest days, one line each"},
		[2]string{command("in Japan"), "your visits to a country, region, town or landmark"},
		[2]string{command("years"), "nights at home and away, by year"})
	var rows [][]string
	for _, h := range hints {
		rows = append(rows, []string{h[0], h[1]})
	}
	w.table([]trawlrender.TableColumn{{Header: "command", NeverTruncateCellValues: true}, {Header: "shows", Wrap: true}}, rows)
	w.line("")
	w.line("Agents: run %s agent-instructions.", Command)
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
		w.hint("Show them: " + command("trips --all"))
	}
	if trips.TotalTrips > int32(len(trips.Trips)) {
		w.hint("More: " + command(fmt.Sprintf("trips --limit %d", min(int(trips.TotalTrips), limit*2))))
	}
	w.hint("One trip, day by day, from any date on it: " + command("trip "+tripDay(trips.Trips[0].Link)))
	return w.err
}

var dayColumns = []trawlrender.TableColumn{
	{Header: "day", KeepWholeTokensWhenTerminalWidthAllows: true},
	{Header: "where you went", Wrap: true},
	{Header: "night", Wrap: true},
}

// dayRows are one row per day. With merge, days in a row that went to the same place and spent the night the same
// way read as one block: "Thu 9 – Sat 11 May".
func dayRows(days []*wherewasiv1.ItineraryDay, merge bool) [][]string {
	var rows [][]string
	for i := 0; i < len(days); {
		day := days[i]
		went := placesWentTo(day)
		key := strings.Join(went, " → ") + "\x00" + nightText(day.Night)
		j := i + 1
		for merge && len(went) == 1 && j < len(days) && strings.Join(placesWentTo(days[j]), " → ")+"\x00"+nightText(days[j].Night) == key {
			j++
		}
		when := shortDayLabel(day.LocalDay)
		if !merge {
			when += goDate(day.LocalDay).Format(" 2006")
		}
		if j-i > 1 {
			when = goDate(day.LocalDay).Format("Mon 2") + " – " + shortDayLabel(days[j-1].LocalDay)
		}
		place := wentTo(days[i:j])
		if len(went) == 0 {
			place = "no photos"
		}
		night := nightIn(day.Night, went)
		rows = append(rows, []string{when, place, night})
		i = j
	}
	return rows
}

// Itinerary is one trip, day by day.
func Itinerary(out io.Writer, it *wherewasiv1.Itinerary) error {
	w := &writer{out: out}
	t := it.Trip
	nights := plural(t.Nights, "night away", "nights away")
	if t.Ongoing {
		nights += " so far"
	}
	summary := []string{tripDates(t), nights, plural(t.PhotoCount, "photo", "photos"),
		strings.Join(t.Countries, ", ")}
	w.line("%s", strings.Join(summary, " · "))
	if t.NightsWithoutPhotos > 0 {
		w.line("%s without photos, so where you slept then isn't known.", plural(t.NightsWithoutPhotos, "night", "nights"))
	}
	w.line("")
	w.table(dayColumns, dayRows(it.Days, true))
	w.line("")
	w.hint("One day in detail: " + command("on "+isoDate(it.Days[0].LocalDay)))
	w.hint("Every field: " + command("trip "+tripDay(t.Link)+" --format textproto"))
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
		if !v.Travelling { // countries flown over are not countries visited
			countries = appendNew(countries, v.Place.Country)
		}
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
		if _, known := kindWord[v.KnownPlaceKind]; !known && v.PlaceName != "" {
			where[0] = v.PlaceName
		}
		if v.Travelling {
			where = []string{"travelling, " + v.PlaceName + " to " + v.EndPlaceName}
		}
		var neighbourhoods []string
		for _, name := range v.Neighbourhoods {
			if name != where[0] {
				neighbourhoods = append(neighbourhoods, name)
			}
		}
		if len(neighbourhoods) > 0 && !v.Travelling {
			where = append(where, strings.Join(neighbourhoods, ", "))
		}
		if !v.Travelling {
			if v.Place.State != previous.State || v.Place.Country != previous.Country {
				where = append(where, v.Place.State)
			}
			if v.Place.Country != previous.Country {
				where = append(where, v.Place.Country)
			}
			previous = v.Place
		}
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
		var spots []string
		for _, spot := range v.Spots {
			spots = append(spots, spot.Name)
		}
		rows = append(rows, []string{local, utc, strings.Join(nonEmpty(where), " · "), strings.Join(spots, ", "),
			strconv.Itoa(int(v.PhotoCount))})
	}
	w.table([]trawlrender.TableColumn{
		{Header: "local", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "utc", KeepWholeTokensWhenTerminalWidthAllows: true},
		{Header: "where", Wrap: true},
		{Header: "spots", Wrap: true},
		{Header: "photos", AlignRight: true},
	}, rows)
	if n := d.Night; n != nil {
		w.line("")
		var places []string
		for _, v := range d.Visits {
			places = append(places, v.PlaceName)
		}
		if n.Travelling {
			w.line("Spent the night %s.", nightText(n))
		} else {
			w.line("Slept in %s.", nightIn(n, places))
		}
		if doubt := nightDoubt(n); doubt != "" {
			w.line("%s", doubt)
		}
	}
	w.line("")
	w.hint("Every photo, with coordinates: " + command("on "+isoDate(d.LocalDay)+" --format textproto"))
	next := localDay.AddDate(0, 0, 1).Format("2006-01-02")
	w.hint("Next day: " + command("on "+next))
	if d.TripLink != "" {
		w.hint("The whole trip: " + command("trip "+tripDay(d.TripLink)))
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
	rows := dayRows(days.Days, false)
	if len(rows) == 0 {
		w.line("No located photos in this range.")
		return w.err
	}
	w.table(dayColumns, rows)
	w.line("")
	w.line("Showing %d of %s days with photos.", len(days.Days), trawlrender.FormatInteger(int64(days.TotalDays)))
	if days.TotalDays > int32(len(days.Days)) {
		last := days.Days[len(days.Days)-1].LocalDay
		w.hint("Earlier: " + command(fmt.Sprintf("days --to %s --limit %d", goDate(last).AddDate(0, 0, -1).Format("2006-01-02"), limit)))
	}
	w.hint("One day in detail: " + command("on "+isoDate(days.Days[0].LocalDay)))
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
		w.hint("More: " + command(fmt.Sprintf("in %q --limit %d", p.PlaceAskedFor, min(int(p.TotalStays), limit*2))))
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
		w.line("Home unknown: no home is known for these dates.")
		w.hint("Propose earlier homes: " + command("known-places"))
	}
	return w.err
}

// Homes shows home and workplace proposals, leaving exact positions to the textproto.
func Homes(out io.Writer, homes places.Homes) error {
	w := &writer{out: out}
	if homes.UnknownNights == 0 && len(homes.Workplaces) == 0 {
		w.line("No earlier homes or workplaces to propose.")
		return w.err
	}
	if homes.UnknownNights == 0 {
		w.line("No nights with an unknown home.")
	} else if len(homes.Proposals) == 0 {
		w.line("No sustained home found in %s with an unknown home.", plural(homes.UnknownNights, "night", "nights"))
	} else {
		w.line("Proposed earlier homes · %s with an unknown home", plural(homes.UnknownNights, "night", "nights"))
		w.line("")
		var rows [][]string
		for _, proposal := range homes.Proposals {
			to := "now"
			if proposal.Home.ValidTo != nil {
				to = isoDate(proposal.Home.ValidTo)
			}
			rows = append(rows, []string{isoDate(proposal.Home.ValidFrom), to, townName(proposal.Place), proposal.Place.GetNeighbourhood(), proposal.Place.GetStreet(), strconv.Itoa(int(proposal.Nights)), strconv.Itoa(int(proposal.PhotoCount))})
		}
		w.table([]trawlrender.TableColumn{{Header: "from"}, {Header: "to"}, {Header: "town", Wrap: true}, {Header: "neighbourhood", Wrap: true}, {Header: "street and number", Wrap: true}, {Header: "nights", AlignRight: true}, {Header: "photos", AlignRight: true}}, rows)
	}
	if homes.UnknownNights > 0 {
		w.line("")
		w.line("How it decides: a home is the place within %.1f km where most nights were spent in a %d-day window, with at least %d nights across %d days. Nights spent travelling are left out.", places.KnownPlaceRadiusKm, places.HomeWindowDays, places.HomeMinimumNights, places.HomeMinimumSpanDays)
		w.line("Each position is the average of the evening and next-morning photos taken there. A home starts on its first night and ends the day before the next one starts.")
	}
	w.line("")
	if len(homes.Workplaces) == 0 {
		w.line("No recurring weekday daytime places found away from home.")
	} else {
		w.line("Possible workplaces")
		w.line("")
		var rows [][]string
		for _, proposal := range homes.Workplaces {
			rows = append(rows, []string{isoDate(proposal.Work.ValidFrom), isoDate(proposal.Work.ValidTo), townName(proposal.Place), proposal.Place.GetNeighbourhood(), proposal.Place.GetStreet(), strconv.Itoa(int(proposal.Days)), strconv.Itoa(int(proposal.PhotoCount))})
		}
		w.table([]trawlrender.TableColumn{{Header: "from"}, {Header: "to"}, {Header: "town", Wrap: true}, {Header: "neighbourhood", Wrap: true}, {Header: "street and number", Wrap: true}, {Header: "days", AlignRight: true}, {Header: "photos", AlignRight: true}}, rows)
	}
	w.line("")
	w.line("How it decides: a possible workplace has photos within %.0f metres on at least %d weekdays in %d different weeks, across %d days within %d days. It uses Monday to Friday, %02d:00 to %02d:00, away from home, and starts a new period after %d days without a matching photo.", places.WorkplaceRadiusKm*1000, places.WorkplaceMinimumDays, places.WorkplaceMinimumWeeks, places.WorkplaceMinimumSpanDays, places.WorkplaceWindowDays, places.WorkplaceDayStartsAt, places.WorkplaceDayEndsAt, places.WorkplaceMaximumGapDays)
	w.line("Workplace positions average those photos. Dates are the first and last photos there; regular visits can also be cafés or other places, so confirm whether each was work.")
	if len(homes.Proposals)+len(homes.Workplaces) > 0 {
		w.line("")
		knownPlacesFile := "~/.config/" + Command + "/known-places.textproto"
		for _, flag := range RepeatedFlags {
			if flag[0] == "known-places" {
				knownPlacesFile = flag[1]
			}
		}
		w.line("Confirm these homes, workplaces and dates with the user, then save the confirmed entries in %s. This command saves nothing.", knownPlacesFile)
		w.hint("Get the entries to save: " + command("known-places --format textproto"))
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
