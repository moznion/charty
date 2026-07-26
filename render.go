package charty

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

// Formats lists what Render accepts.
var Formats = []string{"png", "svg", "html", "csv", "json"}

// Options tunes a render. The zero value is usable.
type Options struct {
	// Title overrides Chart.Title.
	Title string
	// Width and Height are in inches for the static formats, and set the
	// page's chart box for HTML.
	Width, Height float64
	// DPI applies to PNG.
	DPI int
	// XAxis chooses between placing points by timestamp (the default) and
	// placing them at even spacing in order. See XTime and XIndex.
	XAxis XAxis
	// NoLabel suppresses the direct label on each series' latest point.
	NoLabel bool
	// Generator names the producing tool in HTML output.
	Generator string
	// Gap breaks the line between points further apart than this, for series
	// that do not carry a Gap of their own.
	Gap time.Duration
	// YMin and YMax pin the value axis. Either may be nil, in which case that
	// end is chosen from the data.
	YMin, YMax *float64
	// Warn receives anything the caller ought to know that is not fatal — a
	// title the chart font cannot draw, say. Nil discards them.
	Warn func(string)
}

func (o Options) warn(format string, args ...any) {
	if o.Warn != nil {
		o.Warn(fmt.Sprintf(format, args...))
	}
}

func (o Options) withDefaults() Options {
	if o.Width <= 0 {
		o.Width = 7
	}
	if o.Height <= 0 {
		o.Height = 3.2
	}
	if o.DPI <= 0 {
		o.DPI = 144
	}
	if o.XAxis == "" {
		o.XAxis = XTime
	}
	if o.Generator == "" {
		o.Generator = "charty"
	}
	return o
}

// Render writes the chart in one of Formats.
func Render(w io.Writer, format string, c *Chart, opts Options) error {
	if c == nil {
		return fmt.Errorf("chart is nil")
	}
	if err := c.Normalize(); err != nil {
		return err
	}
	opts = opts.withDefaults()
	if opts.Title != "" {
		c.Title = opts.Title
	}
	switch opts.XAxis {
	case XTime, XIndex:
	default:
		return fmt.Errorf("unknown x axis %q (want %q or %q)", opts.XAxis, XTime, XIndex)
	}
	switch strings.ToLower(format) {
	case "png", "svg":
		return renderStatic(w, format, c, opts)
	case "html":
		return renderHTML(w, c, opts)
	case "csv":
		return renderCSV(w, c)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(c)
	default:
		return fmt.Errorf("unknown format %q (want one of %s)", format, strings.Join(Formats, ", "))
	}
}

// valueRange pads the value axis so no series rides the top or bottom edge.
//
// The axis is deliberately not anchored at zero. These are trend lines, whose
// interesting range is often a few percent wide; a zero baseline would flatten
// every change worth seeing. That is sound for a line, which encodes change by
// position rather than by the length of a bar. It is only the default, though:
// a caller comparing charts, or wanting the full 0–100 of a percentage, pins
// the axis with Options.YMin and YMax.
func valueRange(c *Chart, opts Options) (float64, float64) {
	_, _, lo, hi := c.Bounds()
	pad := (hi - lo) * 0.25
	if pad == 0 {
		pad = math.Max(math.Abs(hi)*0.05, 1)
	}
	lo, hi = lo-pad, hi+pad
	if opts.YMin != nil {
		lo = *opts.YMin
	}
	if opts.YMax != nil {
		hi = *opts.YMax
	}
	if lo >= hi {
		return lo, lo + 1
	}
	return lo, hi
}

// sortedMetaKeys gives metadata a stable presentation order. Go maps have no
// order of their own, and a tooltip whose rows shuffle between renders is
// worse than one whose rows are merely alphabetical.
func sortedMetaKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// formatValue is for display chrome — the hero number, direct labels, legend
// readouts — where two decimals is enough and a trailing ".00" is noise.
func formatValue(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
