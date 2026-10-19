# where-was-i

where-was-i reads the places Apple Photos stored for your photos and tells you which trips you took, where you went each day, what you stopped at and where you slept. It runs on your Mac, reads the library without changing it and makes no network requests.

```
$ where-was-i trips --limit 3
when              nights  countries       where you went
2 – 9 Aug 2024         7  United Kingdom  Pitlochry → Cairngorms National Park → Inverness → Ullapool → Portree
                                          → Fort William
3 – 17 May 2024       14  Japan           Hounslow → Tokyo → Fuji-Hakone-Izu National Park → Hakone → Kyoto →
                                          Nara Park → Osaka → Kamakura
15 – 18 Feb 2024       3  Portugal        Lisbon → Sintra

Showing 3 of 31 trips.
More: where-was-i trips --limit 6
One trip, day by day, from any date on it: where-was-i trip 2024-08-02
```

Each trip lists the places you went, in order. Give any date on a trip to see it day by day:

```
$ where-was-i trip 2024-05-03
3 – 17 May 2024 · 14 nights away · 1,284 photos · Japan

day                 where you went                                                 night
Fri 3 May           Hounslow (Heathrow Airport)                                    travelling, Hounslow to Tokyo
Sat 4 – Mon 6 May   Tokyo (Shinjuku Gyoen National Garden, teamLab Planets,        Tokyo
                    Senso-ji, Ueno Park)
Tue 7 May           Tokyo → Fuji-Hakone-Izu National Park (Owakudani, Lake Ashi,   Hakone
                    Hakone Open-Air Museum)
Wed 8 May           Hakone → Kyoto (Kyoto Station)                                 Kyoto
Thu 9 – Sat 11 May  Kyoto (Fushimi Inari Taisha, Arashiyama Bamboo Grove,          Kyoto
                    Kiyomizu-dera, Tenryu-ji)
Sun 12 May          Nara Park (Todai-ji, Kasuga Taisha) → Osaka (Dotonbori)        Osaka
Mon 13 May          Osaka (Osaka Castle, Kuromon Market)                           Osaka
Tue 14 May          Osaka → Tokyo (Tokyo Station)                                  Tokyo
Wed 15 May          Kamakura (Kotoku-in, Tsurugaoka Hachimangu)                    Tokyo
Thu 16 May          Tokyo (Haneda Airport)                                         travelling, Tokyo to Hounslow
Fri 17 May          Hounslow (Heathrow Airport)

One day in detail: where-was-i on 2024-05-03
Every field: where-was-i trip 2024-05-03 --format textproto
```

In brackets are the spots you stopped at in each place: the landmarks, viewpoints and trails Apple Photos named near your photos. One day shows where you were and when, in local time and UTC:

```
$ where-was-i on 2024-05-09
Thu 9 May 2024 · Japan · day 7 of the trip from 3 May · up to 9,520 km from home · local time UTC+09:00

local        utc                where                 spots                 photos
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
2 – 9 Aug 2024               8  Pitlochry → Aviemore → Inverness → Ullapool → Portree → Fort William
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

> Set up where-was-i on my Mac: https://github.com/opentrawl/where-was-i. Install the latest release as its README says, run `where-was-i agent-instructions` and follow them, then run `where-was-i`. If macOS stops it reading my Photos library, use computer use to open System Settings, Privacy & Security, Full Disk Access, switch it on for the app you're running in and let me approve it. Restart that app and run `where-was-i` again. Then run `where-was-i homes`, ask me to confirm the homes and move dates, and add the confirmed entries from `where-was-i homes --format textproto` to `~/.config/where-was-i/known-places.textproto`.

Whatever where-was-i prints goes to that agent and its model provider.

## Older trips

Apple Photos knows only your current home, so trips from before you moved there are missing. Run `where-was-i homes` to propose earlier homes from nights whose home is unknown. Each row gives the dates, Apple's town and neighbourhood, and the numbers of nights and photos behind the proposal. It saves nothing.

The rule groups nights within 0.5 km and picks the place with most observed nights in a 180-day window. Both the window and the proposed home period need at least 10 nights spanning 90 days. Short stays cannot become homes. The position is the mean of the photos that place those nights: the last photo of the evening and the first of the next morning before noon, when taken at that place. Each photo is counted once. The first home starts at the first unknown night; each later home starts at its first supporting night around the change. A home's end is the day before the next home starts, including homes already known. Photos cannot establish the exact day you moved: confirm the homes and dates first.

Then run `where-was-i homes --format textproto` and add the confirmed entries to `~/.config/where-was-i/known-places.textproto`. Preserve entries already in the file. The output is the file's `KnownPlaces` format, including exact positions and evidence. Family homes and other known places can be added too:

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

Run `where-was-i agent-instructions` for how an agent should use it; point your `AGENTS.md` at that command rather than copying [the instructions](cmd/where-was-i/SKILL.md). When where-was-i writes to another program, it never wraps or cuts a row. Every list says how many rows it shows and which command shows more. Add `--format textproto` to any command for every field, as defined in [where_was_i.proto](proto/wherewasi/v1/where_was_i.proto). `on DATE --format textproto` lists every photo of the day with its GPS position, time and Apple's place fields. Usage errors exit with 2 and other failures with 1.

## How it works

Places are Apple's own, as the Photos app shows them. Apple's levels differ by country: its "state" is a prefecture in Japan and "Scotland" in the UK.

A day's places are towns, or areas you moved through. A park or other landmark counts as a place when your photos in it that day are more than 2 km apart; otherwise it is a spot in its town, as a museum or a hotel is. A road counts as a spot only when its photos that day are within 2 km of each other, so roads you drove along are left out. Where you went is the places holding a quarter or more of the day's photos, and a trip lists the ten of those with most photos.

Times are local to where each photo was taken, from a built-in time-zone map, because a phone's clock lags on travel days. A day runs from 04:00 to 04:00, so a late night counts with its evening.

A night was at home if you took a photo there that evening or the next day. Otherwise it was where you took your first photo the next morning, or your last photo that evening if there was no morning photo. When those two photos are more than 300 km apart, the night was spent travelling. When they are in different towns, `on DATE` shows where and when each was taken.

A trip is a run of nights more than 25 km from home, including nights with family who live further away than that. `trips` leaves out one-night stays where no photo shows you waking up away, unless you pass `--all`.

## Limits

where-was-i knows only the days you took photos with Location Services on. Apple files some photos under the nearest postal town, so a photo in a national park can show a village outside it. Photos can't show where you slept when you drove somewhere after your last photo of the evening or before your first of the morning. It runs only on a Mac, because it reads the Photos library directly.

## Licence

MIT. Time-zone boundaries come from [timezone-boundary-builder](https://github.com/evansiroky/timezone-boundary-builder) via [tzf](https://github.com/ringsaturn/tzf), © OpenStreetMap contributors, under the [ODbL](https://opendatacommons.org/licenses/odbl/).
