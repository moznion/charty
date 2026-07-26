package charty

import (
	"math"
	"sort"
	"time"

	"gonum.org/v1/plot"
)

// XAxis selects how points are placed along the horizontal axis.
type XAxis string

const (
	// XTime places each point at its timestamp. Distance on the axis is
	// elapsed time, which is what a reader expects — but a history measured in
	// bursts (a day of CI activity, then a quiet week) collapses its busiest
	// stretches into a vertical wall.
	XTime XAxis = "time"
	// XIndex places points at even spacing, in order. Every measurement gets
	// the same width, so a burst stays readable; the cost is that the axis no
	// longer shows elapsed time, and a long silence looks like one short step.
	XIndex XAxis = "index"
)

// Timeline returns the sorted union of every series' timestamps.
//
// It is the union rather than any one series' own times so that, in index
// mode, series measured at different moments still line up: a point's position
// is its place in the chart's history, not in its own.
func (c *Chart) Timeline() []time.Time {
	seen := make(map[int64]struct{})
	var out []time.Time
	for _, s := range c.Series {
		for _, p := range s.Points {
			key := p.Time.UnixNano()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, p.Time)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// xmap converts timestamps into axis coordinates for the chosen mode.
type xmap struct {
	mode     XAxis
	timeline []time.Time
	pos      map[int64]int
}

func newXMap(c *Chart, mode XAxis) *xmap {
	m := &xmap{mode: mode}
	if mode != XIndex {
		return m
	}
	m.timeline = c.Timeline()
	m.pos = make(map[int64]int, len(m.timeline))
	for i, t := range m.timeline {
		m.pos[t.UnixNano()] = i
	}
	return m
}

func (m *xmap) at(t time.Time) float64 {
	if m.mode == XIndex {
		return float64(m.pos[t.UnixNano()])
	}
	return float64(t.Unix())
}

// bounds returns the axis extent covering every point.
func (m *xmap) bounds(c *Chart) (float64, float64) {
	if m.mode == XIndex {
		if len(m.timeline) == 0 {
			return 0, 0
		}
		return 0, float64(len(m.timeline) - 1)
	}
	tMin, tMax, _, _ := c.Bounds()
	return float64(tMin.Unix()), float64(tMax.Unix())
}

// ticker labels the horizontal axis.
//
// It replaces plot.TimeTicks, which cannot label index positions and which
// needs a layout chosen up front. The step comes from the span instead, so an
// hour-long history gets clock times and a two-year one gets months without
// the caller having to say so — and without a knob to get wrong.
type ticker struct {
	xm *xmap
}

// timeSteps is the ladder of tick intervals, coarsening until at most a
// handful of labels are left.
var timeSteps = []time.Duration{
	time.Minute, 5 * time.Minute, 15 * time.Minute,
	time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour,
	24 * time.Hour, 2 * 24 * time.Hour, 7 * 24 * time.Hour, 14 * 24 * time.Hour,
	30 * 24 * time.Hour, 91 * 24 * time.Hour, 182 * 24 * time.Hour, 365 * 24 * time.Hour,
}

const maxTicks = 5

func (t ticker) Ticks(from, to float64) []plot.Tick {
	if t.xm.mode == XIndex {
		return t.indexTicks(from, to)
	}
	return t.timeTicks(from, to)
}

// indexTicks labels evenly spaced positions with the date found there. The
// positions are round numbers of points, not of time.
func (t ticker) indexTicks(from, to float64) []plot.Tick {
	n := len(t.xm.timeline)
	if n == 0 {
		return nil
	}
	lo, hi := int(math.Ceil(from)), int(math.Floor(to))
	lo, hi = clamp(lo, 0, n-1), clamp(hi, 0, n-1)
	if hi <= lo {
		return []plot.Tick{{Value: float64(lo), Label: label(t.xm.timeline[lo], 24*time.Hour)}}
	}

	step := (hi - lo) / maxTicks
	if step < 1 {
		step = 1
	}
	var at []time.Time
	var idx []int
	for i := lo; i <= hi; i += step {
		idx = append(idx, i)
		at = append(at, t.xm.timeline[i])
	}

	// Ticks are spaced by count, so neighbours can share a date — several
	// reports from one busy afternoon. A date repeated down the axis reads as
	// a mistake, so the labels drop to the finer layout that tells them apart.
	span := at[len(at)-1].Sub(at[0])
	format := coarseness(span / maxTicks)
	if repeats(at, format) {
		format = "Jan 02 15:04"
	}
	ticks := make([]plot.Tick, len(idx))
	for i := range idx {
		ticks[i] = plot.Tick{Value: float64(idx[i]), Label: at[i].Format(format)}
	}
	return ticks
}

// repeats reports whether consecutive times render identically.
func repeats(at []time.Time, format string) bool {
	for i := 1; i < len(at); i++ {
		if at[i].Format(format) == at[i-1].Format(format) {
			return true
		}
	}
	return false
}

// timeTicks steps in round units of time, aligned on local midnight once the
// step is a day or more so a label never names the day before the tick it
// sits under.
func (t ticker) timeTicks(lo, hi float64) []plot.Tick {
	from, to := time.Unix(int64(lo), 0), time.Unix(int64(hi), 0)
	span := to.Sub(from)
	if span <= 0 {
		return []plot.Tick{{Value: lo, Label: label(from, 24*time.Hour)}}
	}

	step := timeSteps[len(timeSteps)-1]
	for _, s := range timeSteps {
		if span/s <= maxTicks {
			step = s
			break
		}
	}

	var ticks []plot.Tick
	if step >= 24*time.Hour {
		days := int(step / (24 * time.Hour))
		local := from.Local()
		y, m, d := local.Date()
		at := time.Date(y, m, d, 0, 0, 0, 0, local.Location())
		if at.Before(from) {
			at = at.AddDate(0, 0, 1)
		}
		for ; !at.After(to); at = at.AddDate(0, 0, days) {
			ticks = append(ticks, plot.Tick{Value: float64(at.Unix()), Label: label(at, step)})
		}
		return ticks
	}
	secs := int64(step.Seconds())
	for u := (from.Unix() + secs - 1) / secs * secs; u <= to.Unix(); u += secs {
		at := time.Unix(u, 0)
		ticks = append(ticks, plot.Tick{Value: float64(u), Label: label(at, step)})
	}
	return ticks
}

// label formats a tick at the coarseness of its step.
func label(at time.Time, step time.Duration) string {
	return at.Format(coarseness(step))
}

// coarseness picks a layout to match the interval between ticks: clock time
// within a day, dates within a season, months beyond it.
func coarseness(step time.Duration) string {
	switch {
	case step >= 60*24*time.Hour:
		return "Jan 2006"
	case step >= 24*time.Hour:
		return "Jan 02"
	default:
		return "15:04"
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
