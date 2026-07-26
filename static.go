package charty

import (
	"fmt"
	"io"

	"golang.org/x/image/font/sfnt"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
	"gonum.org/v1/plot/vg/vgsvg"
)

// directLabelLimit is how many series can be labelled at their latest point
// before the labels start colliding and the legend has to carry identity alone.
const directLabelLimit = 4

// renderStatic draws the chart with gonum/plot.
//
// Note for callers: gonum/plot's embedded fonts have no CJK glyphs, so
// non-Latin text in a title, series name or unit is dropped silently. The HTML
// renderer has no such limit.
func renderStatic(w io.Writer, format string, c *Chart, opts Options) error {
	warnUndrawable(c, format, opts)

	p := plot.New()
	p.BackgroundColor = colSurface
	p.Title.Text = c.Title
	p.Title.TextStyle.Color = colPrimary
	p.Title.TextStyle.Font.Size = vg.Points(13)
	p.Title.Padding = vg.Points(8)

	xm := newXMap(c, opts.XAxis)
	p.X.Tick.Marker = ticker{xm: xm}
	p.Y.Label.Text = valueAxisLabel(c)
	for _, ax := range []*plot.Axis{&p.X, &p.Y} {
		ax.Color = colAxis
		ax.Tick.Color = colAxis
		ax.Tick.Label.Color = colMuted
		ax.Tick.Label.Font.Size = vg.Points(9)
		ax.Label.TextStyle.Color = colSecondary
		ax.Label.TextStyle.Font.Size = vg.Points(10)
	}

	// Horizontal hairlines only; vertical rules compete with the trend.
	grid := plotter.NewGrid()
	grid.Vertical.Color = nil
	grid.Horizontal.Color = colGrid
	grid.Horizontal.Width = vg.Points(0.5)
	p.Add(grid)

	x0, x1 := xm.bounds(c)
	// Reserve room on the right for the direct labels, which the automatic
	// range ignores because the text is drawn outside the mark.
	room := x1 > x0 && !opts.NoLabel && len(c.Series) <= directLabelLimit
	if x1 > x0 {
		p.X.Min, p.X.Max = x0, x1
		if room {
			p.X.Max = x1 + (x1-x0)*0.10
		}
	}
	p.Y.Min, p.Y.Max = valueRange(c, opts)

	// Rules go under the data: they are chrome, not a series. Dashed and in
	// the muted ink so they never read as a ninth measurement.
	for _, r := range c.Rules {
		rule, err := plotter.NewLine(plotter.XYs{{X: p.X.Min, Y: r.Value}, {X: p.X.Max, Y: r.Value}})
		if err != nil {
			return err
		}
		rule.Color = colMuted
		rule.Width = vg.Points(1)
		rule.Dashes = []vg.Length{vg.Points(4), vg.Points(3)}
		p.Add(rule)
		if r.Label != "" {
			lbl, err := plotter.NewLabels(plotter.XYLabels{
				XYs:    plotter.XYs{{X: p.X.Min, Y: r.Value}},
				Labels: []string{" " + r.Label},
			})
			if err != nil {
				return err
			}
			lbl.TextStyle[0].Color = colMuted
			lbl.TextStyle[0].Font.Size = vg.Points(9)
			lbl.TextStyle[0].YAlign = draw.YBottom
			lbl.Offset = vg.Point{Y: vg.Points(2)} // clear of the line it names
			p.Add(lbl)
		}
	}

	for i, s := range c.Series {
		col := seriesColor(i)
		xys := make(plotter.XYs, len(s.Points))
		for j, pt := range s.Points {
			xys[j] = plotter.XY{X: xm.at(pt.Time), Y: pt.Value}
		}

		// One line per segment: a gap in the measurements is a gap in the
		// line, not a straight run through the months nobody measured.
		legended := false
		for _, seg := range s.Segments(opts.Gap) {
			if len(seg) < 2 {
				continue
			}
			pts := make(plotter.XYs, len(seg))
			for j, pt := range seg {
				pts[j] = plotter.XY{X: xm.at(pt.Time), Y: pt.Value}
			}
			line, err := plotter.NewLine(pts)
			if err != nil {
				return err
			}
			line.Color = col
			line.Width = vg.Points(2)
			p.Add(line)
			if len(c.Series) > 1 && !legended {
				p.Legend.Add(s.Name, line)
				legended = true
			}
		}

		dots, err := plotter.NewScatter(xys)
		if err != nil {
			return err
		}
		dots.GlyphStyle = draw.GlyphStyle{Color: col, Radius: vg.Points(2.5), Shape: draw.CircleGlyph{}}
		p.Add(dots)
		if len(c.Series) > 1 && !legended {
			p.Legend.Add(s.Name, dots)
		}

		// Label the latest value only. A number on every point turns a trend
		// into a table, and the latest value is the one being asked about.
		if room {
			last := xys[len(xys)-1]
			lbl, err := plotter.NewLabels(plotter.XYLabels{
				XYs:    plotter.XYs{last},
				Labels: []string{" " + formatValue(last.Y)},
			})
			if err != nil {
				return err
			}
			lbl.TextStyle[0].Color = colPrimary
			lbl.TextStyle[0].Font.Size = vg.Points(10)
			lbl.TextStyle[0].YAlign = draw.YCenter
			p.Add(lbl)
		}
	}

	if len(c.Series) > 1 {
		p.Legend.Top = true
		p.Legend.Left = true
		p.Legend.XOffs = vg.Points(12)
		p.Legend.TextStyle.Color = colSecondary
		p.Legend.TextStyle.Font.Size = vg.Points(9)
		p.Legend.ThumbnailWidth = vg.Points(14)
	}

	width, height := vg.Length(opts.Width)*vg.Inch, vg.Length(opts.Height)*vg.Inch
	switch format {
	case "png":
		canvas := vgimg.NewWith(
			vgimg.UseWH(width, height),
			vgimg.UseDPI(opts.DPI),
			vgimg.UseBackgroundColor(colSurface),
		)
		p.Draw(draw.New(canvas))
		_, err := vgimg.PngCanvas{Canvas: canvas}.WriteTo(w)
		return err
	case "svg":
		canvas := vgsvg.New(width, height)
		p.Draw(draw.New(canvas))
		_, err := canvas.WriteTo(w)
		return err
	}
	return fmt.Errorf("unknown static format %q", format)
}

// valueAxisLabel names the value axis. With one series its name is the label;
// with several the legend carries the names and only a shared unit belongs on
// the axis.
func valueAxisLabel(c *Chart) string {
	if len(c.Series) == 1 {
		s := c.Series[0]
		switch {
		case s.Name != "" && s.Unit != "":
			return fmt.Sprintf("%s (%s)", s.Name, s.Unit)
		case s.Name != "":
			return s.Name
		default:
			return s.Unit
		}
	}
	return c.unit()
}

// warnUndrawable reports text the chart font has no glyphs for.
//
// gonum/plot embeds Liberation, which covers Latin, Greek and Cyrillic but no
// CJK. Missing glyphs are dropped silently at draw time, which turns a
// Japanese title into a chart that merely looks oddly empty, so it is worth
// saying out loud. The HTML renderer uses the reader's system fonts and has no
// such limit.
func warnUndrawable(c *Chart, format string, opts Options) {
	if opts.Warn == nil {
		return
	}
	check := func(where, s string) {
		if missing := undrawable(s); missing != "" {
			opts.warn("%s: the chart font has no glyphs for %q in the %s; those characters will be missing — render html for non-Latin text",
				format, missing, where)
		}
	}
	check("title", c.Title)
	for _, s := range c.Series {
		check(fmt.Sprintf("name of series %q", s.Name), s.Name)
		check("unit", s.Unit)
	}
	for _, r := range c.Rules {
		check("rule label", r.Label)
	}
}

// undrawable returns the distinct runes of s the default plot font cannot draw.
func undrawable(s string) string {
	if s == "" {
		return ""
	}
	hdlr := plot.DefaultTextHandler
	if hdlr == nil {
		return ""
	}
	cache := hdlr.Cache()
	if cache == nil {
		return ""
	}
	face := cache.Lookup(plot.DefaultFont, vg.Points(12))
	if face.Face == nil {
		return ""
	}
	var (
		buf     sfnt.Buffer
		seen    = map[rune]bool{}
		missing []rune
	)
	for _, r := range s {
		if seen[r] {
			continue
		}
		seen[r] = true
		idx, err := face.Face.GlyphIndex(&buf, r)
		if err != nil || idx == 0 {
			missing = append(missing, r)
		}
	}
	return string(missing)
}
