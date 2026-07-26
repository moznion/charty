// Package charty renders time series as line charts: PNG, SVG, or a
// self-contained interactive HTML page.
//
// A Chart is the interchange representation. It is deliberately ignorant of
// where the numbers came from: a point carries a time, a value, an optional
// short label, an optional link, and free-form metadata. A caller plotting
// code coverage puts a commit SHA in Label and the commit's URL in Href;
// charty never learns what a commit is.
package charty

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

// MaxSeries is the number of series a chart may carry. The limit is the
// categorical palette: past eight, hues stop being reliably distinguishable
// and a ninth would have to repeat one. Split the data across charts instead.
const MaxSeries = 8

// Chart is a titled group of series sharing one time axis and one value axis.
type Chart struct {
	Title  string   `json:"title,omitempty"`
	Series []Series `json:"series"`
	// Rules are horizontal reference lines — a target, a budget, a limit.
	Rules []Rule `json:"rules,omitempty"`
}

// Rule is a reference line at a fixed value.
//
// A rule is drawn inside the value axis, which means the axis stretches to
// reach it. That is the point: a target you cannot see on the chart tells you
// nothing about whether you are meeting it.
type Rule struct {
	Value float64 `json:"v"`
	Label string  `json:"label,omitempty"`
}

// Series is one line.
type Series struct {
	// Name identifies the series in the legend and tooltip. It is required
	// once a chart has more than one series.
	Name string `json:"name,omitempty"`
	// Unit annotates the value, e.g. "%" or "ms".
	Unit string `json:"unit,omitempty"`
	// Gap is how far apart two points may be before the line between them is
	// broken. A metric that stopped being measured for a month did not hold
	// steady across it, and a line drawn straight through says it did.
	Gap    Duration `json:"gap,omitempty"`
	Points []Point  `json:"points"`
}

// Segments splits a series wherever consecutive points are further apart than
// gap, which is the series' own Gap unless the caller overrides it. Every
// point still belongs to exactly one segment; only the connecting line breaks.
func (s Series) Segments(gap time.Duration) [][]Point {
	if s.Gap > 0 {
		gap = s.Gap.Duration()
	}
	if gap <= 0 || len(s.Points) < 2 {
		return [][]Point{s.Points}
	}
	var out [][]Point
	start := 0
	for i := 1; i < len(s.Points); i++ {
		if s.Points[i].Time.Sub(s.Points[i-1].Time) > gap {
			out = append(out, s.Points[start:i])
			start = i
		}
	}
	return append(out, s.Points[start:])
}

// Point is one measurement.
type Point struct {
	Time  time.Time `json:"t"`
	Value float64   `json:"v"`
	// Label is a short identifier for the point — a commit SHA, a build
	// number — shown in the tooltip and the table.
	Label string `json:"label,omitempty"`
	// Href makes the point clickable in HTML output.
	Href string `json:"href,omitempty"`
	// Meta is arbitrary extra detail, listed under the point in HTML output.
	// Keys are displayed in sorted order.
	Meta map[string]string `json:"meta,omitempty"`
}

// UnmarshalJSON accepts either a full chart object or a bare array of points,
// so that a one-series chart can be piped straight out of jq without having to
// wrap it.
func (c *Chart) UnmarshalJSON(data []byte) error {
	trimmed := firstToken(data)
	if trimmed == '[' {
		var points []Point
		if err := json.Unmarshal(data, &points); err != nil {
			return err
		}
		c.Series = []Series{{Points: points}}
		return nil
	}
	type chartAlias Chart // avoid recursing into this method
	var alias chartAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*c = Chart(alias)
	return nil
}

func firstToken(data []byte) byte {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return b
		}
	}
	return 0
}

// Decode reads a Chart from JSON and validates it.
func Decode(r io.Reader) (*Chart, error) {
	var c Chart
	if err := json.NewDecoder(r).Decode(&c); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("input is empty")
		}
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if err := c.Normalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Normalize sorts each series chronologically and reports what makes the chart
// unrenderable. Renderers call it, so callers rarely need to.
func (c *Chart) Normalize() error {
	if len(c.Series) == 0 {
		return errors.New("chart has no series")
	}
	if len(c.Series) > MaxSeries {
		return fmt.Errorf("chart has %d series; at most %d can be told apart by colour — split it across charts",
			len(c.Series), MaxSeries)
	}
	named := 0
	for i := range c.Series {
		s := &c.Series[i]
		if len(s.Points) == 0 {
			return fmt.Errorf("series %d (%q) has no points", i, s.Name)
		}
		for j, p := range s.Points {
			if math.IsNaN(p.Value) || math.IsInf(p.Value, 0) {
				return fmt.Errorf("series %d (%q) point %d has a non-finite value", i, s.Name, j)
			}
			if p.Time.IsZero() {
				return fmt.Errorf("series %d (%q) point %d has no timestamp", i, s.Name, j)
			}
		}
		sort.SliceStable(s.Points, func(a, b int) bool {
			return s.Points[a].Time.Before(s.Points[b].Time)
		})
		if s.Name != "" {
			named++
		}
	}
	if len(c.Series) > 1 && named < len(c.Series) {
		return errors.New("every series needs a name once a chart has more than one, so the legend can tell them apart")
	}
	// Series sharing one value axis must be in the same unit. Plotting, say,
	// percent against seconds on a single axis is the dual-axis mistake with
	// the second axis left off: whichever unit labels the axis, the other
	// series is read against a scale that is not its own.
	for i, r := range c.Rules {
		if math.IsNaN(r.Value) || math.IsInf(r.Value, 0) {
			return fmt.Errorf("rule %d (%q) has a non-finite value", i, r.Label)
		}
	}
	if units := declaredUnits(c); len(units) > 1 {
		return fmt.Errorf("series declare %d different units (%s); they cannot share one value axis — "+
			"draw them as separate charts, or index them to a common base",
			len(units), strings.Join(units, ", "))
	}
	return nil
}

// declaredUnits lists the distinct non-empty units, in order of appearance.
func declaredUnits(c *Chart) []string {
	var units []string
	seen := map[string]struct{}{}
	for _, s := range c.Series {
		if s.Unit == "" {
			continue
		}
		if _, ok := seen[s.Unit]; ok {
			continue
		}
		seen[s.Unit] = struct{}{}
		units = append(units, s.Unit)
	}
	return units
}

// Bounds returns the extent of every series and rule combined.
func (c *Chart) Bounds() (tMin, tMax time.Time, vMin, vMax float64) {
	vMin, vMax = math.Inf(1), math.Inf(-1)
	for _, r := range c.Rules {
		vMin, vMax = math.Min(vMin, r.Value), math.Max(vMax, r.Value)
	}
	for _, s := range c.Series {
		for _, p := range s.Points {
			if tMin.IsZero() || p.Time.Before(tMin) {
				tMin = p.Time
			}
			if tMax.IsZero() || p.Time.After(tMax) {
				tMax = p.Time
			}
			vMin, vMax = math.Min(vMin, p.Value), math.Max(vMax, p.Value)
		}
	}
	return tMin, tMax, vMin, vMax
}

// Len reports the total number of points across every series.
func (c *Chart) Len() int {
	n := 0
	for _, s := range c.Series {
		n += len(s.Points)
	}
	return n
}

// unit returns the unit shared by every series that declares one, which
// Normalize has already established is unambiguous.
func (c *Chart) unit() string {
	if units := declaredUnits(c); len(units) == 1 {
		return units[0]
	}
	return ""
}
