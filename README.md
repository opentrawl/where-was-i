# where-was-i

Your iPhone has been quietly recording where you were for years. Every photo carries a location, and Apple Photos has already turned each one into a place name: the town, the neighbourhood, often the park or landmark. `where-was-i` reads that and gives it back as trips, itineraries and days. You get answers to "where was I on 9 May?", "which trips did I take last year?" and "when was I last in Scotland?"

It runs on your Mac, reads your Photos library without changing it, and sends nothing anywhere.

```
$ where-was-i trips
when              nights  countries              route                                                              link
2 – 9 Aug 2024         7  United Kingdom         Pitlochry → Inverness (2) → Ullapool → Portree (2) → Fort William  trip:2024-08-02
3 – 17 May 2024       14  Japan, United Kingdom  Tokyo (4) → Hakone → Kyoto (4) → Osaka (2) → Tokyo (2) → Hounslow  trip:2024-05-03
15 – 18 Feb 2024       3  Portugal               Lisbon (3)                                                         trip:2024-02-15

Showing 3 of 31 trips.
More: where-was-i trips --limit 6
One trip, day by day: where-was-i trip trip:2024-05-03
```

The route is where you slept, with the number of nights where it's more than one.

## One trip, day by day

```
$ where-was-i trip trip:2024-05-03
3 – 17 May 2024 · 14 nights · 1,284 photos · Japan, United Kingdom
Apple Photos calls it: Japan

day                 went through              apple named                                         night
Fri 3 May           London → Hounslow         Heathrow Airport                                    Hounslow → Tokyo
Sat 4 May           Tokyo                     Shinjuku Gyoen National Garden, Omoide Yokocho      Tokyo
Sun 5 – Mon 6 May   Tokyo (2 days)            Senso-ji, Ueno Park, teamLab Planets                Tokyo
Tue 7 May           Tokyo → Odawara → Hakone  Hakone Open-Air Museum, Lake Ashi                   Hakone
Wed 8 May           Hakone → Kyoto            Kyoto Station                                       Kyoto
Thu 9 – Sat 11 May  Kyoto (3 days)            Fushimi Inari Taisha, Kiyomizu-dera, Yasaka Shrine  Kyoto
Sun 12 May          Kyoto → Nara → Osaka      Nara Park, Todai-ji, Dotonbori                      Osaka
Mon 13 May          Osaka                     Osaka Castle, Kuromon Market                        Osaka
Tue 14 May          Osaka → Tokyo             Tokyo Station                                       Tokyo
Wed 15 May          Tokyo → Kamakura → Tokyo  Kotoku-in, Tsurugaoka Hachimangu                    Tokyo
Thu 16 May          Tokyo → Ota               Haneda Airport                                      Ota → Hounslow
Fri 17 May          Hounslow → London         Heathrow Airport

Apple named: parks, landmarks and places Apple Photos stored near your photos. Night: where the next morning's first
photo was taken, or home when a photo that evening or the next day was.

One day in detail: where-was-i on 2024-05-03
Every field, for agents: where-was-i trip trip:2024-05-03 --format textproto
```

"Hounslow → Tokyo" is an overnight flight: your last photo that evening was at Heathrow, and your first the next morning was in Tokyo. The names under "apple named" are the parks, landmarks and places Apple Photos stored for your photos.

## One day, hour by hour

```
$ where-was-i on 2024-05-09
Thu 9 May 2024 · Japan · on trip:2024-05-03 · 9,520 km from home at the furthest · local time UTC+09:00

local        utc                where                 apple named           photos
07:12–08:40  8 May 22:12–23:40  Kyoto · Fushimi Ward  Fushimi Inari Taisha      64
10:05–10:31  01:05–01:31        Kyoto · Higashiyama   Kiyomizu-dera             23
12:15–13:02  03:15–04:02        Kyoto · Gion          Yasaka Shrine             17
16:40–17:10  07:40–08:10        Kyoto · Nakagyo Ward  Nishiki Market             9
19:30–21:05  10:30–12:05        Kyoto · Gion          Hanamikoji Street         31

Night: Kyoto (from the evening's last photo and the next morning's first).

A day runs from 04:00 to 04:00 local time, so photos after midnight count with the evening before.
Every photo, with its place fields and coordinates: where-was-i on 2024-05-09 --format textproto
Next day: where-was-i on 2024-05-10
The whole trip: where-was-i trip trip:2024-05-03
```

Every time is shown in local time and UTC, using the time zone where each photo was taken. Your phone's clock doesn't always catch up on travel days.

## More ways in

```
$ where-was-i                    what's here, your latest trips and where to start
$ where-was-i in Scotland        your stays in a country, region, town or landmark
$ where-was-i days               the latest days, one line each
$ where-was-i years              nights at home, with family and away, by year
```

```
$ where-was-i in Scotland
Scotland: 13 days with photos, 2 stays

when                      days  towns                                                  matched           trip
2 – 9 Aug 2024               8  Pitlochry, Inverness, Ullapool, Portree, Fort William  Scotland (state)  trip:2024-08-02
29 Dec 2023 – 2 Jan 2024     5  Edinburgh                                              Scotland (state)  trip:2023-12-29

Showing 2 of 2 stays.
```

```
$ where-was-i years
year  home  family  near home  away  home unknown  no photos  away, by country
2024   171      12         21    38             0         32  Japan 13, Netherlands 8, United Kingdom 8, France 6, Portugal 3
2023   238      19         30    41             0         37  Italy 12, United Kingdom 11, Spain 9, Germany 5, Ireland 4

Nights by where they were spent. A night with no photo that evening or the next day isn't guessed.
```

## Set it up with your agent

Paste this into Claude, Codex or any agent that can run commands on your Mac. where-was-i itself sends nothing anywhere, but whatever it prints goes to the agent you run it through.

> Set up where-was-i on my Mac: https://github.com/opentrawl/where-was-i. Install the latest release by following the "Install" section of its README, then run `where-was-i`. If macOS stops it reading my Photos library, open System Settings, Privacy & Security, Full Disk Access with computer use, switch it on for the app you're running in, and let me approve it. Then restart that app and run `where-was-i` again. Finally, ask me where I lived before my current home and when I moved, and write those homes to `~/.config/where-was-i/known-places.textproto`, using the repo's `examples/known-places.textproto` as the template, so my older trips come out right.

## Using it from an agent

- When the output goes to another program rather than a terminal, nothing is wrapped or cut short: one row per line.
- Every list says how many rows it shows, and gives the command for more.
- Add `--format textproto` to any command for the typed messages in [where_was_i.proto](proto/wherewasi/v1/where_was_i.proto). That's every field, down to each photo's place, coordinates and both clocks.
- Usage errors exit with code 2, other failures with 1, and messages go to stderr.

## Install

You need a Mac with Apple silicon, macOS 14 or later, and your photos in the Photos app.

```bash
mkdir -p ~/.local/bin
curl -fsSL https://github.com/opentrawl/where-was-i/releases/latest/download/where-was-i-macos-arm64.tar.gz | tar -xz -C ~/.local/bin
where-was-i
```

The first run may ask for Full Disk Access. macOS protects the Photos library, and the error says where to turn it on: System Settings, Privacy & Security, Full Disk Access, for the app you run it in (Terminal, Ghostty, your editor). Then restart that app. If your agent works on the Mac over SSH, turn on "Allow full disk access for remote users" under System Settings, General, Sharing, Remote Login instead.

To build from source instead: `go install github.com/opentrawl/where-was-i/cmd/where-was-i@latest`. This needs Xcode's command-line tools.

## Your homes over time

Apple Photos knows where you live now, and that's enough for recent trips. It doesn't know where you lived before. Without that, nights before your current home show as "home unknown" rather than as trips. To fix that, list your earlier homes, and any family you stay with, in `~/.config/where-was-i/known-places.textproto`:

```
places {
  label: "Flat in Leith"
  kind: KNOWN_PLACE_KIND_HOME
  location { latitude: 55.9750 longitude: -3.1700 }
  valid_to { year: 2021 month: 8 day: 31 }
}
places {
  label: "Mum and Dad"
  kind: KNOWN_PLACE_KIND_FAMILY
  location { latitude: 53.4808 longitude: -2.2426 }
}
```

[examples/known-places.textproto](examples/known-places.textproto) explains every field.

## How it works

- **Places** are Apple's own: country, region, county, town, neighbourhood, landmark. Nothing is looked up online, so the names are what Apple Photos shows you. Apple's levels differ by country: "state" is a province in Spain, a prefecture in Japan and "Scotland" in the UK.
- **Local time** comes from a built-in map of the world's time zones, at each photo's position. A day runs from 04:00 to 04:00, so a late night counts with its evening.
- **Nights:** if a photo that evening or the next day was at home or with family, the night was there. Otherwise it was where your first photo the next morning was taken, and failing that where your last photo of the evening was.
- **A trip** is nights in a row more than 25 km from home and family. Short stays with no morning photo away are often long days out, so `trips` leaves them out unless you add `--all`.

It never names your trips or guesses what you did. Apple's own trip titles appear as they are.

## Limits

- It only knows days you took photos with Location Services on.
- Apple sometimes uses the postal town rather than where you were. A photo in a national park can be filed under a village outside it, but the park usually appears under "apple named".
- Mac only, because it reads Apple Photos.

## Licence

MIT. Time-zone boundaries come from [timezone-boundary-builder](https://github.com/evansiroky/timezone-boundary-builder) via [tzf](https://github.com/ringsaturn/tzf), and are © OpenStreetMap contributors under the [ODbL](https://opendatacommons.org/licenses/odbl/).
