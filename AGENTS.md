# Working on where-was-i

where-was-i reads the places Apple Photos stored for your photos and gives them back as trips, itineraries and days.
It is a small, local, mechanical CLI: no model, no network, no setup command. Judgement (naming trips, deciding what
mattered) belongs to whoever reads its output.

- The typed contract is `proto/wherewasi/v1/where_was_i.proto`. Change it first, regenerate with
  `devenv shell -- buf generate proto`, and keep human output and textproto carrying the same facts.
- Build with `devenv shell -- build`. It uses Apple's compiler so the binary runs on any Mac with macOS 14 or later.
- Human output goes through OpenTrawl's `trawlkit/render`. When stdout is not a terminal, nothing wraps.
- `internal/sources/apple_photos.go` is the only code that knows Apple's private Photos schema. Look up table and
  column names there rather than assuming them.
- Check a change by running the commands on a real Photos library and reading the output, at full width, at 80
  columns and piped.
- Never commit anything from a real library: no output, names, places, dates or coordinates. Examples are made up.
