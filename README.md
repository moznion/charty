# charty

Render a time series to a line chart: PNG, SVG, or a self-contained interactive HTML page.

charty is deliberately ignorant of where the numbers came from. A point carries a time, a
value, an optional short label, an optional link, and free-form metadata. A caller plotting
code coverage puts a commit SHA in `label` and the commit's URL in `href`; charty never
learns what a commit is.

```console
$ charty -o coverage.png coverage.json
$ jq '[.[] | {t: .measured_at, v: .percent}]' raw.json | charty -f html -o coverage.html
```

## Install

```console
$ go install github.com/moznion/charty/cmd/charty@latest   # CLI
$ go get github.com/moznion/charty                         # library
```

## Input

Either a chart object:

```json
{
  "title": "moznion/cccc",
  "series": [
    {
      "name": "coverage",
      "unit": "%",
      "points": [
        {
          "t": "2026-07-22T05:49:04.104436086Z",
          "v": 93.74624689028052,
          "label": "ac01f467",
          "href": "https://github.com/moznion/cccc/commit/ac01f467...",
          "meta": { "ref": "refs/heads/main" }
        }
      ]
    }
  ]
}
```

or a bare array of points, which becomes a single unnamed series — the shape that falls out
of `jq`:

```json
[{ "t": "2026-07-22T05:49:04Z", "v": 93.75 }]
```

A chart may also carry `rules` — horizontal reference lines for a target, a budget, a limit:

```json
{ "series": [...], "rules": [{ "v": 80, "label": "acceptable" }] }
```

A rule is drawn dashed and in the muted ink, under the data, so it never reads as another
series. It stretches the value axis to reach it: a target you cannot see on the chart tells
you nothing about whether you are meeting it.

Only `t` and `v` are required. `label`, `href` and `meta` are carried through to the HTML
page, where a point can be clicked through to its link and its metadata is listed in the
tooltip and the table.

A series may also declare a `gap`:

```json
{ "name": "coverage", "gap": "7d", "points": [...] }
```

Wherever two points are further apart than that, the line breaks. A metric that stopped being
measured for a month did not hold steady across it, and a line drawn straight through says it
did. The points themselves are still drawn — only the connection is dropped. `gap` lives in
the chart rather than in the render options precisely so that it survives being saved and
re-rendered.

Points are sorted chronologically on input, so a series that arrives newest-first is fine.

## Output

`-f` takes `png`, `svg`, `html`, `csv` or `json`; without it the format follows the `-o`
extension, and failing that it is `png`. `csv` and `json` carry full precision — the chart is
lossy, the data is not — and `csv` turns the union of metadata keys into columns.

`-y-min` and `-y-max` pin the value axis, for comparing charts side by side or for showing a
percentage against its full 0–100.

### Placing points by time or in order

By default a point sits at its timestamp, so distance along the axis is elapsed time. That is
what a reader expects, but it collapses a history measured in bursts — several reports in one
afternoon, then a quiet week — into a vertical wall.

`-x index` places points at even spacing instead, in order:

```console
$ charty -x index -o coverage.png coverage.json
```

Every measurement then gets the same width. The cost is that the axis no longer shows elapsed
time: a long silence looks like one short step, which is why it is not the default. Positions
come from the union of every series' timestamps, so series measured at different moments still
line up.

The tick layout is chosen from the span in both modes — clock times over an afternoon, months
over a couple of years — so there is nothing to configure. HTML formats dates in the reader's
locale; PNG and SVG use a fixed English layout.

### The HTML page

One file, no external requests, no bundled charting library: the chart is drawn as SVG by a
small inline script. A 20-point series comes out around 24 KB.

- Hover, or step through with the arrow keys (`Shift` for ×10, `Home`/`End` for the ends),
  for a crosshair and a readout of every series measured at that instant.
- Click a point — or press `Enter` on it — to follow its `href`.
- Drag across the plot to zoom the time axis; `Reset zoom`, double-click or `Escape` to undo.
- A data table below the chart holds every value at full precision. It is rendered as markup,
  so the numbers survive with JavaScript disabled and are reachable without hovering.
- The page follows the reader's light/dark preference, with palette steps chosen for each
  surface rather than flipped.

### Limitation: PNG and SVG cannot draw CJK

PNG and SVG text is drawn with the fonts embedded in
[gonum/plot](https://github.com/gonum/plot) — Liberation, which covers Latin, Greek and
Cyrillic but no CJK. Japanese, Chinese and Korean characters in a title, series name, unit or
rule label **will be missing from the image**. There is no way to supply another font today.

charty reports it rather than leaving you with a chart that merely looks oddly empty:

```console
$ charty --title 'カバレッジ推移' -o coverage.png series.json
charty: png: the chart font has no glyphs for "カバレッジ推移" in the title; those
characters will be missing — render html for non-Latin text
```

As a library, the report arrives through `Options.Warn`; a nil `Warn` discards it and the
characters still vanish. **Use `-f html` for non-Latin text** — the page uses the reader's own
system fonts and has no such limit.

## What charty refuses to draw

A chart that would mislead is an error, not a warning:

- **More than 8 series.** Past eight, hues stop being reliably distinguishable and a ninth
  would have to repeat one. Split the data across charts.
- **Series declaring different units.** Percent and seconds on one value axis is the
  dual-axis mistake with the second axis left off: whichever unit labels the axis, the other
  series is read against a scale that is not its own. Draw them separately, or index them to
  a common base.
- **An unnamed series in a multi-series chart**, since the legend would have nothing to say
  and identity would rest on colour alone.

Two deliberate choices that are *not* errors: hues are assigned in a fixed order and never
cycled, so a series keeps its colour however many others are present; and the value axis is
not anchored at zero, because a trend line's interesting range is often a few percent wide and
a zero baseline would flatten every change worth seeing. That is sound for a line, which
encodes change by position rather than by the length of a bar.

## Development

```console
$ make            # fmt-check, lint, test, build
$ make build      # bin/charty
$ make test
$ make lint       # golangci-lint, pinned in the Makefile so CI agrees
$ make fmt        # gofmt and goimports
```

Dependencies are vendored, so building the CLI needs no network. `make vendor` refreshes
`vendor/` after a dependency change; `make vendor-check` fails if it needs refreshing.
Vendoring does not affect anyone importing charty as a library — a dependency's
`vendor/` is ignored — nor `go install github.com/moznion/charty/cmd/charty@latest`, which
builds from the module proxy.

## Library

```go
c := &charty.Chart{
	Title: "coverage",
	Series: []charty.Series{{
		Name: "coverage", Unit: "%",
		Points: []charty.Point{
			{Time: t, Value: 93.75, Label: "ac01f467", Href: url,
				Meta: map[string]string{"ref": "refs/heads/main"}},
		},
	}},
}
err := charty.Render(w, "html", c, charty.Options{Generator: "myapp"})
```

`charty.Decode` reads the same JSON the CLI accepts. `Chart.Normalize` sorts and validates,
and `Render` calls it for you.
