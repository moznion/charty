package charty_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/moznion/charty"
)

func TestParseDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"":      0,
		"1d":    24 * time.Hour,
		"2.5d":  60 * time.Hour,
		"1w":    7 * 24 * time.Hour,
		"12h":   12 * time.Hour,
		"1h30m": 90 * time.Minute,
	}
	for in, want := range tests {
		got, err := charty.ParseDuration(in)
		if err != nil {
			t.Errorf("ParseDuration(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseDuration(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseDurationRejectsGarbage(t *testing.T) {
	for _, in := range []string{"daily", "1", "0s", "-1d", "1y"} {
		if _, err := charty.ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q): expected an error", in)
		}
	}
}

// The gap travels in the interchange JSON, so a chart saved and re-rendered
// still knows which stretches were never measured.
func TestDurationRoundTripsAsAString(t *testing.T) {
	c := single()
	c.Series[0].Gap = charty.Duration(7 * 24 * time.Hour)
	out := render(t, "json", c, charty.Options{})
	if !strings.Contains(out, `"gap": "168h0m0s"`) {
		t.Errorf("json = %s, want a string duration", out)
	}
	var got charty.Chart
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Series[0].Gap.Duration() != 7*24*time.Hour {
		t.Errorf("Gap = %v", got.Series[0].Gap.Duration())
	}
}

func TestDurationAcceptsFriendlySuffixesInJSON(t *testing.T) {
	c, err := charty.Decode(strings.NewReader(
		`{"series":[{"gap":"7d","points":[{"t":"2026-07-01T00:00:00Z","v":1}]}]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.Series[0].Gap.Duration() != 7*24*time.Hour {
		t.Errorf("Gap = %v", c.Series[0].Gap.Duration())
	}
}

func TestDurationRejectsANumber(t *testing.T) {
	_, err := charty.Decode(strings.NewReader(
		`{"series":[{"gap":604800,"points":[{"t":"2026-07-01T00:00:00Z","v":1}]}]}`))
	if err == nil {
		t.Error("expected an error; a bare number is ambiguous about its unit")
	}
}

func day(n int) charty.Point {
	return charty.Point{Time: time.Date(2026, 7, n, 0, 0, 0, 0, time.UTC), Value: float64(n)}
}

func TestSegmentsBreakOnAGap(t *testing.T) {
	s := charty.Series{Points: []charty.Point{day(1), day(2), day(10), day(11)}}
	segs := s.Segments(48 * time.Hour)
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(segs))
	}
	if len(segs[0]) != 2 || len(segs[1]) != 2 {
		t.Errorf("segments = %v", segs)
	}
}

// The series' own gap is what the interchange carries, so it wins.
func TestSeriesGapOverridesTheOption(t *testing.T) {
	s := charty.Series{Gap: charty.Duration(48 * time.Hour), Points: []charty.Point{day(1), day(2), day(10)}}
	if got := len(s.Segments(30 * 24 * time.Hour)); got != 2 {
		t.Errorf("got %d segments, want the series' own gap to apply", got)
	}
}

func TestSegmentsAreOneWithoutAGap(t *testing.T) {
	s := charty.Series{Points: []charty.Point{day(1), day(10)}}
	if got := len(s.Segments(0)); got != 1 {
		t.Errorf("got %d segments, want 1", got)
	}
}

func TestGapBreaksTheLineInEveryFormat(t *testing.T) {
	c := &charty.Chart{Series: []charty.Series{{
		Name:   "coverage",
		Points: []charty.Point{day(1), day(2), day(10), day(11)},
	}}}
	opts := charty.Options{Gap: 48 * time.Hour}
	for _, format := range []string{"png", "svg", "html"} {
		if out := render(t, format, c, opts); len(out) == 0 {
			t.Errorf("%s: no output", format)
		}
	}
	// The html path starts a fresh subpath at the break rather than one run.
	const gapMillis = `"gap":172800000` // 48h, in the unit the browser counts in
	if page := render(t, "html", c, opts); !strings.Contains(page, gapMillis) {
		t.Error("the gap did not reach the browser")
	}
}

func TestYBoundsPinTheAxis(t *testing.T) {
	lo, hi := 0.0, 100.0
	page := render(t, "html", single(), charty.Options{YMin: &lo, YMax: &hi})
	if !strings.Contains(page, `"yMin":0`) || !strings.Contains(page, `"yMax":100`) {
		t.Error("the pinned axis did not reach the browser")
	}
	// The static renderers must accept them too.
	for _, format := range []string{"png", "svg"} {
		if out := render(t, format, single(), charty.Options{YMin: &lo, YMax: &hi}); len(out) == 0 {
			t.Errorf("%s: no output", format)
		}
	}
}

// Text the chart font cannot draw disappears at render time. Saying so beats
// handing back a chart that merely looks oddly empty.
func TestUndrawableTextIsReported(t *testing.T) {
	c := single()
	c.Title = "カバレッジ推移"
	var warnings []string
	opts := charty.Options{Warn: func(m string) { warnings = append(warnings, m) }}
	render(t, "png", c, opts)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one about the title", warnings)
	}
	if !strings.Contains(warnings[0], "カバレッジ推移") || !strings.Contains(warnings[0], "title") {
		t.Errorf("warning = %q", warnings[0])
	}
}

func TestLatinTextIsNotReported(t *testing.T) {
	var warnings []string
	render(t, "png", single(), charty.Options{Warn: func(m string) { warnings = append(warnings, m) }})
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

// The html page uses the reader's own fonts, so it has nothing to warn about.
func TestHTMLDoesNotWarnAboutFonts(t *testing.T) {
	c := single()
	c.Title = "カバレッジ推移"
	var warnings []string
	render(t, "html", c, charty.Options{Warn: func(m string) { warnings = append(warnings, m) }})
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if page := render(t, "html", c, charty.Options{}); !strings.Contains(page, "カバレッジ推移") {
		t.Error("the html page dropped the title")
	}
}
