# where-was-i

where-was-i reads the places Apple Photos stored for your photos and tells you which trips you took, where you slept each night and what you saw each day. It runs on your Mac, reads the library without changing it and makes no network requests.

```
$ where-was-i trips --limit 3
when              nights  countries       where you slept
2 – 9 Aug 2024         7  United Kingdom  Pitlochry, 1 night → Inverness, 2 nights → Ullapool, 1 night →
                                          Portree, 2 nights → Fort William, 1 night
3 – 17 May 2024       14  Japan           travelling, 1 night → Tokyo, 3 nights → Hakone, 1 night →
                                          Kyoto, 4 nights → Osaka, 2 nights → Tokyo, 2 nights →
                                          travelling, 1 night
15 – 18 Feb 2024       3  Portugal        Lisbon, 3 nights

Showing 3 of 31 trips.
More: where-was-i trips --limit 6
One trip, day by day: where-was-i trip 2024-08-02 (any date on the trip)
```

Each trip lists where you slept, in order. Give any date on a trip to see it day by day:

```
$ where-was-i trip 2024-05-03
3 – 17 May 2024 · 14 nights away · 1,284 photos · Japan

day                 places                    points of interest                   slept in
Fri 3 May           London → Hounslow         Heathrow Airport                     travelling, Hounslow to Tokyo
Sat 4 – Mon 6 May   Tokyo                     Shinjuku Gyoen National Garden,      Tokyo
                                              Omoide Yokocho, Senso-ji, Ueno
                                              Park, teamLab Planets
Tue 7 May           Tokyo → Odawara → Hakone  Hakone Open-Air Museum, Lake Ashi    Hakone
Wed 8 May           Hakone → Kyoto            Kyoto Station                        Kyoto
Thu 9 – Sat 11 May  Kyoto                     Fushimi Inari Taisha,                Kyoto
                                              Kiyomizu-dera, Yasaka Shrine,
                                              Nishiki Market, Hanamikoji Street,
                                              Arashiyama Bamboo Grove, Nijo
                                              Castle
Sun 12 May          Kyoto → Nara → Osaka      Nara Park, Todai-ji, Dotonbori       Osaka
Mon 13 May          Osaka                     Osaka Castle, Kuromon Market         Osaka
Tue 14 May          Osaka → Tokyo             Tokyo Station                        Tokyo
Wed 15 May          Tokyo → Kamakura → Tokyo  Kotoku-in, Tsurugaoka Hachimangu     Tokyo
Thu 16 May          Tokyo → Ota               Haneda Airport                       travelling, Ota to Hounslow
Fri 17 May          Hounslow → London         Heathrow Airport                     home

One day in detail: where-was-i on 2024-05-03
Every field: where-was-i trip 2024-05-03 --format textproto
```

The points of interest are the parks, landmarks and places Apple Photos named near your photos. One day shows where you were and when, in local time and UTC:

```
$ where-was-i on 2024-05-09
Thu 9 May 2024 · Japan · day 7 of the trip from 3 May · up to 9,520 km from home · local time UTC+09:00

local        utc                where                 points of interest    photos
07:12–08:40  8 May 22:12–23:40  Kyoto · Fushimi Ward  Fushimi Inari Taisha      64
10:05–10:31  01:05–01:31        Kyoto · Higashiyama   Kiyomizu-dera             23
12:15–13:02  03:15–04:02        Kyoto · Gion          Yasaka Shrine             17
16:40–17:10  07:40–08:10        Kyoto · Nakagyo Ward  Nishiki Market             9
19:30–21:05  10:30–12:05        Kyoto · Gion          Hanamikoji Street         31

Slept in Kyoto.

Every photo, with coordinates: where-was-i on 2024-05-09 --format textproto
Next day: where-was-i on 2024-05-10
The whole trip: where-was-i trip 2024-05-03
```

You can look up your visits to any country, region, town or landmark that Apple Photos names:

```
$ where-was-i in Scotland
Scotland: 13 days with photos, 2 visits

when                      days  places
2 – 9 Aug 2024               8  Pitlochry → Inverness → Ullapool → Portree → Fort William
29 Dec 2023 – 2 Jan 2024     5  Edinburgh
```

And `years` counts your nights by where you spent them:

```
$ where-was-i years
Nights each year, by where you spent them

year  at home  with family  near home  away  home unknown  no photos  away, by country
2024      145            9         14    29             0         24  Japan 12, United Kingdom 8, Netherlands 4,
                                                                      Portugal 3, travelling 2
2023      238           19         30    41             0         37  Italy 12, United Kingdom 11, Spain 9,
                                                                      Germany 5, Ireland 4

Near home: within 25 km of home but not at it. No photos: none that evening or the next morning.
```

Run `where-was-i` with no arguments for your latest trips and the other commands.

## Install

You need a Mac with Apple silicon and macOS 14 or later.

```bash
mkdir -p ~/.local/bin
curl -fsSL https://github.com/opentrawl/where-was-i/releases/latest/download/where-was-i-macos-arm64.tar.gz | tar -xz -C ~/.local/bin
where-was-i
```

If the first run stops with a message about Full Disk Access, turn it on for the app you ran it in (System Settings, Privacy & Security, Full Disk Access), then restart that app. Over SSH, turn on "Allow full disk access for remote users" in System Settings, General, Sharing, Remote Login.

To build it yourself, install Xcode's command-line tools and run `go install github.com/opentrawl/where-was-i/cmd/where-was-i@latest`.

### Let an agent set it up

Paste this into Claude Code, Codex or another agent that can run commands on your Mac:

> Set up where-was-i on my Mac: https://github.com/opentrawl/where-was-i. Install the latest release as its README says, then run `where-was-i`. If macOS stops it reading my Photos library, use computer use to open System Settings, Privacy & Security, Full Disk Access, switch it on for the app you're running in and let me approve it. Restart that app and run `where-was-i` again. Then ask me where I lived before my current home and when I moved, and write those homes to `~/.config/where-was-i/known-places.textproto`, following the repo's `examples/known-places.textproto`.

Whatever where-was-i prints goes to that agent and its model provider.

## Older trips

Apple Photos knows only your current home, so trips from before you moved there are missing. To find them, list your earlier homes, and family you stay with, in `~/.config/where-was-i/known-places.textproto`:

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

[examples/known-places.textproto](examples/known-places.textproto) has the full format.

## For agents

When where-was-i writes to another program, it never wraps or cuts a row. Every list says how many rows it shows and which command shows more. Add `--format textproto` to any command for every field, down to each photo's place and coordinates, as defined in [where_was_i.proto](proto/wherewasi/v1/where_was_i.proto). Usage errors exit with 2 and other failures with 1.

## How it works

Places are Apple's own, as the Photos app shows them. Apple's levels differ by country: its "state" is a prefecture in Japan and "Scotland" in the UK.

Times are local to where each photo was taken, from a built-in time-zone map, because a phone's clock lags on travel days. A day runs from 04:00 to 04:00, so a late night counts with its evening.

A night was at home if you took a photo there that evening or the next day. Otherwise it was where you took your first photo the next morning, or your last photo that evening if there was no morning photo. When those two photos are more than 300 km apart, the night was spent travelling.

A trip is a run of nights more than 25 km from home and family. `trips` leaves out one-night stays where no photo shows you waking up away, unless you pass `--all`.

## Limits

where-was-i knows only the days you took photos with Location Services on. Apple files some photos under the nearest postal town, so a photo in a national park can show a village outside it, with the park as the point of interest. It runs only on a Mac, because it reads the Photos library directly.

## Licence

MIT. Time-zone boundaries come from [timezone-boundary-builder](https://github.com/evansiroky/timezone-boundary-builder) via [tzf](https://github.com/ringsaturn/tzf), © OpenStreetMap contributors, under the [ODbL](https://opendatacommons.org/licenses/odbl/).
