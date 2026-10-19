---
name: where-was-i
description: Use where-was-i to find where the user was, which trips they took, where they slept and what they did each day, from the places Apple Photos stored for their photos. Use when a question depends on the user's past whereabouts, travel or dates away from home.
---

# Find where the user was with where-was-i

where-was-i reads the user's Apple Photos library on this Mac and reports Apple's own place names for their photos, arranged by day, night and trip. It holds no model and makes no judgements: everything it prints is Apple's names, photo counts, times and distances. Deciding what a trip was for, what mattered or how to tell the story is your job.

Run `where-was-i` with no arguments first. It shows the size of the library, the latest trips and the commands to go further. Run `where-was-i --help` for every command and flag.

## Follow the printed commands

Output sent to another program is never wrapped or cut, one row per line. Every list says how many rows it shows and prints the command that shows more. Every view ends with commands for the next level of detail, repeating the flags you ran with, such as `--known-places`. Run those commands as printed instead of guessing new ones.

The levels are:

- `trips`: each trip with its dates, nights away, countries and the ten places it went to with most photos, in order. "and 26 more" means the full list is in `trip DATE`.
- `trip DATE`: one trip day by day: where the user went, the spots they stopped at in brackets, and where they slept. Any date on the trip works.
- `on DATE`: one day, stretch by stretch, in local time and UTC, using the same places and spots.
- `days`, `in PLACE`, `years`: the latest days, visits to a named place, and nights by year.
- `known-places`: proposed earlier homes and possible workplaces, with dates, Apple's town, neighbourhood, street and house number, and counts of supporting nights or days and photos. It saves nothing.

## Read the typed output

Add `--format textproto` to any command for every field as typed text. The fields are defined, with a comment on each, in https://github.com/opentrawl/where-was-i/blob/main/proto/wherewasi/v1/where_was_i.proto, and their names are meant to be read without it.

- A trip has `route`, the ten places its days went to with most photos, and `places_in_order` with photo counts.
- Each day of `trip DATE` has `places`, in the order first seen. A place is a town (`DAY_PLACE_KIND_TOWN`), or an area you moved through, such as a national park (`DAY_PLACE_KIND_AREA`). `went_to` marks the places holding a quarter or more of the day's photos. Inside each place are its `spots` (points of interest and roads where the user stopped, with photo counts) and its `neighbourhoods`.
- `on DATE --format textproto` has the day's `visits`, each with its `place_name` and `spots`, and lists every photo: exact latitude and longitude, the time in UTC and local time, the phone's own clock offset, and every place field Apple stored, including the full address. This is the raw evidence. A whole trip takes one call per day.

## Know what the rules are

These are fixed rules, not judgements. Check them against the photos when a conclusion depends on them.

- A day runs from 04:00 to 04:00 local time, so a late night counts with its evening.
- A night is at home or with family when a photo that evening or the next day was taken there. Otherwise it is where the first photo of the next morning, before noon, was taken, or failing that the last photo of the evening. When those two photos are more than 300 km apart, the night was spent travelling. This is wrong when the user drove somewhere before their first morning photo, or after their last evening photo. When the two photos are in different towns, `on DATE` says where and when each was taken, and `night` keeps both photos' places and times. A night whose photo was inside a park the day went to reads as the park, not its postal town.
- A trip is a run of nights more than 25 km from home, including nights with family who live further away, with up to two nights without photos inside it. One-night stays with no morning photo away are left out unless you pass `--all`. A trip the user is still on reads "– now", and today has no night yet.
- In `on DATE`, photos taken while moving faster than 200 km/h, measured over a minute or more each side, form one `travelling` stretch, as on a flight. Countries flown over are not counted as visited.
- A point of interest counts as where the user went when the day's photos in it are more than 2 km apart. Otherwise it is a spot within its town. A road counts as a spot only when its photos that day are within 2 km of each other, so roads the user drove along are left out.

## Know the limits of the evidence

- Only photos taken with Location Services on are counted. Days without them are missing, not empty.
- Apple names postal towns, so a campsite in a national park can show the town whose post code covers it. The point of interest usually has the real name.
- Apple Photos knows only the user's current home. Nights before then show as "home unknown", and those trips are missing until earlier homes are listed. Run `where-was-i known-places` and ask the user to confirm the proposed homes, workplaces and dates together. The rule picks the location within 0.5 km with most observed nights in a 180-day window. Both the window and the proposed period require at least 10 nights spanning 90 days. Positions average matching last-evening and first-next-morning-before-noon photos, counting each photo once; dates are photo evidence, not verified move dates. After confirmation, run the printed `known-places --format textproto` command and add only confirmed entries to `~/.config/where-was-i/known-places.textproto`, preserving existing entries. The output is already the file's `KnownPlaces` format: do not estimate coordinates or house numbers yourself. If no sustained home is found, ask where the user lived and when rather than inventing one. Re-run `years` with the saved file to check that the covered dates no longer show home unknown.
- Possible workplaces are recurring places away from home: Monday to Friday, 09:00–18:00, within 100 metres, with at least six days in six different weeks spanning 60 days within a 180-day window. Gaps over 180 days split periods. Dates are the first and last supporting photos, not employment dates. A café or another regular stop can qualify too. Ask which were workplaces, and save only confirmed WORK entries. Missing proposals mean insufficient qualifying photo evidence, not that the user never worked there.
- Street and house number come from the exact photos used for the averaged position. The most common address is used; ties use the photo nearest that position. If Apple stored no number, show only the street.
- A photo shows where the camera was, not who the user was with or why they were there.

where-was-i reads the library without changing it. If macOS refuses access, the error says which app needs Full Disk Access. Ask the user to grant it, or help them through System Settings if you can operate the screen.

## Keep these instructions updateable

These instructions ship inside where-was-i. Do not copy them into an agent's skill directory. For durable discovery, point `AGENTS.md` at `where-was-i agent-instructions`, so an update to where-was-i changes them in one place.
