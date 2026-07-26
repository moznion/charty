package charty_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"html"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/moznion/charty"
)

func render(t *testing.T, format string, c *charty.Chart, opts charty.Options) string {
	t.Helper()
	var buf bytes.Buffer
	if err := charty.Render(&buf, format, c, opts); err != nil {
		t.Fatalf("Render(%s): %v", format, err)
	}
	return buf.String()
}

func TestRenderPNG(t *testing.T) {
	out := render(t, "png", single(), charty.Options{})
	if !strings.HasPrefix(out, "\x89PNG\r\n\x1a\n") {
		t.Error("output is not a PNG")
	}
}

func TestRenderSVG(t *testing.T) {
	if out := render(t, "svg", single(), charty.Options{}); !strings.Contains(out, "<svg") {
		t.Error("output is not an SVG")
	}
}

func TestRenderJSONRoundTrips(t *testing.T) {
	out := render(t, "json", single(), charty.Options{})
	var got charty.Chart
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Title != "coverage" || len(got.Series[0].Points) != 2 {
		t.Errorf("round trip lost data: %+v", got)
	}
	if got.Series[0].Points[0].Meta["ref"] != "refs/heads/main" {
		t.Error("metadata did not survive the round trip")
	}
}

func TestRenderCSVUnionsMetadataIntoColumns(t *testing.T) {
	out := render(t, "csv", single(), charty.Options{})
	rows, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	want := []string{"series", "timestamp", "value", "label", "href", "build", "ref"}
	if strings.Join(rows[0], ",") != strings.Join(want, ",") {
		t.Errorf("header = %v, want %v", rows[0], want)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want a header plus 2 points", len(rows))
	}
	// The second point declares no metadata, so its columns are blank rather
	// than shifted.
	if rows[2][5] != "" || rows[2][6] != "" {
		t.Errorf("sparse metadata shifted the row: %v", rows[2])
	}
}

func TestRenderCSVKeepsFullPrecision(t *testing.T) {
	c := single()
	c.Series[0].Points[0].Value = 93.70220809347882
	if out := render(t, "csv", c, charty.Options{}); !strings.Contains(out, "93.70220809347882") {
		t.Error("csv rounded the value")
	}
}

func TestRenderRejectsUnknownFormats(t *testing.T) {
	if err := charty.Render(&bytes.Buffer{}, "pdf", single(), charty.Options{}); err == nil {
		t.Error("expected an error")
	}
}

func TestRenderValidatesBeforeDrawing(t *testing.T) {
	if err := charty.Render(&bytes.Buffer{}, "png", &charty.Chart{}, charty.Options{}); err == nil {
		t.Error("expected an error for a chart with no series")
	}
}

func TestOptionsTitleOverridesTheChart(t *testing.T) {
	out := render(t, "html", single(), charty.Options{Title: "overridden"})
	if !strings.Contains(out, "<title>overridden</title>") {
		t.Error("the title option was not applied")
	}
}

// Degenerate inputs must not divide by a zero range.
func TestRenderHandlesDegenerateSeries(t *testing.T) {
	tests := map[string]*charty.Chart{
		"one point": {Series: []charty.Series{{Points: []charty.Point{{Time: at(1), Value: 5}}}}},
		"flat":      {Series: []charty.Series{{Points: []charty.Point{{Time: at(1), Value: 5}, {Time: at(2), Value: 5}}}}},
		"zeroes":    {Series: []charty.Series{{Points: []charty.Point{{Time: at(1), Value: 0}, {Time: at(2), Value: 0}}}}},
	}
	for name, c := range tests {
		for _, format := range []string{"png", "svg", "html"} {
			t.Run(name+"/"+format, func(t *testing.T) {
				if out := render(t, format, c, charty.Options{}); len(out) == 0 {
					t.Error("no output")
				}
			})
		}
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	page := render(t, "html", single(), charty.Options{})
	external := regexp.MustCompile(`(?i)(src|href)\s*=\s*["']https?://[^"']*`)
	for _, m := range external.FindAllString(page, -1) {
		if strings.Contains(m, "example.com") {
			continue // the caller's own point links
		}
		t.Errorf("page references an external resource: %q", m)
	}
	for _, want := range []string{"<style>", "<script", "createElementNS"} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %s", want)
		}
	}
}

// The chart is drawn by script, so the values must also exist as markup for
// readers without JavaScript and for assistive technology.
func TestHTMLIncludesAServerRenderedTable(t *testing.T) {
	page := render(t, "html", single(), charty.Options{})
	for _, want := range []string{"<table>", "93.75", "91.5", "aaaaaaaa", "refs/heads/main"} {
		if !strings.Contains(page, want) {
			t.Errorf("table is missing %q", want)
		}
	}
}

func TestHTMLLinksPointsThatHaveAnHref(t *testing.T) {
	page := render(t, "html", single(), charty.Options{})
	if !strings.Contains(page, `href="https://example.com/commit/aaaaaaaa"`) {
		t.Error("the point link was not rendered")
	}
}

// Identity must never rest on colour alone: with several series the legend is
// always present and names each one.
func TestHTMLMultiSeriesHasALegend(t *testing.T) {
	page := render(t, "html", multi(), charty.Options{})
	if !strings.Contains(page, `class="legend"`) {
		t.Error("no legend")
	}
	for _, name := range []string{"core", "cli"} {
		if !strings.Contains(page, ">"+name+"<") {
			t.Errorf("legend is missing %q", name)
		}
	}
	if !strings.Contains(page, "<th>Series</th>") {
		t.Error("the table should name the series each row belongs to")
	}
}

// Hues are assigned in a fixed order so a series keeps its colour regardless
// of how many others are present.
func TestHTMLAssignsPaletteSlotsInOrder(t *testing.T) {
	page := render(t, "html", multi(), charty.Options{})
	for _, want := range []string{".s0 { color: #2a78d6; }", ".s1 { color: #eb6834; }"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing palette rule %q", want)
		}
	}
	// The dark step is chosen, not derived by flipping the light one.
	if !strings.Contains(page, "prefers-color-scheme: dark") || !strings.Contains(page, "#3987e5") {
		t.Error("no dark-surface palette")
	}
}

func TestHTMLSingleSeriesHasNoLegend(t *testing.T) {
	page := render(t, "html", single(), charty.Options{})
	if strings.Contains(page, `class="legend"`) {
		t.Error("a single series needs no legend; the title names it")
	}
	if !strings.Contains(page, `class="hero"`) {
		t.Error("expected the hero number")
	}
}

func TestHTMLShowsTheDeltaDirection(t *testing.T) {
	// html/template writes "+" as &#43;, so compare against the decoded page.
	if page := html.UnescapeString(render(t, "html", single(), charty.Options{})); !strings.Contains(page, "+2.25") {
		t.Error("expected the rise from 91.5 to 93.75 to be reported")
	}
	c := single()
	c.Series[0].Points[1].Value = 90
	if page := render(t, "html", c, charty.Options{}); !strings.Contains(page, "−1.5") {
		t.Error("expected a signed decrease")
	}
	c.Series[0].Points[1].Value = c.Series[0].Points[0].Value
	if page := render(t, "html", c, charty.Options{}); !strings.Contains(page, "no change") {
		t.Error("expected a flat series to say so")
	}
}

// Titles, series names and metadata are caller data and must not escape their
// context.
func TestHTMLEscapesUntrustedText(t *testing.T) {
	c := single()
	c.Title = `</title><script>alert(1)</script>`
	c.Series[0].Name = `"><img onerror=alert(1) src=x>`
	c.Series[0].Points[0].Meta = map[string]string{"k": `</td><script>alert(2)</script>`}
	page := render(t, "html", c, charty.Options{})
	for _, bad := range []string{"<script>alert(1)</script>", "<img onerror", "<script>alert(2)</script>"} {
		if strings.Contains(page, bad) {
			t.Errorf("%q escaped its context", bad)
		}
	}
}

func TestHTMLPayloadStaysValidJSON(t *testing.T) {
	c := single()
	c.Series[0].Points[0].Label = `</script><script>alert(1)//`
	page := render(t, "html", c, charty.Options{})

	const open = `<script id="charty-data" type="application/json">`
	i := strings.Index(page, open)
	if i < 0 {
		t.Fatal("data payload not found")
	}
	rest := page[i+len(open):]
	end := strings.Index(rest, "</script>")
	if end < 0 {
		t.Fatal("data payload is not closed")
	}
	payload := rest[:end]

	var got struct {
		Series []struct {
			Name   string `json:"name"`
			Unit   string `json:"unit"`
			Points []struct {
				T     int64       `json:"t"`
				V     float64     `json:"v"`
				Label string      `json:"label"`
				Meta  [][2]string `json:"meta"`
			} `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if len(got.Series) != 1 || len(got.Series[0].Points) != 2 {
		t.Fatalf("payload = %+v", got)
	}
	if got.Series[0].Points[0].Label != `</script><script>alert(1)//` {
		t.Error("the label did not survive escaping intact")
	}
	// Metadata is ordered server-side so the tooltip never shuffles.
	if meta := got.Series[0].Points[0].Meta; len(meta) != 2 || meta[0][0] != "build" || meta[1][0] != "ref" {
		t.Errorf("meta = %v, want it sorted by key", meta)
	}
	if got.Series[0].Points[0].T >= got.Series[0].Points[1].T {
		t.Error("payload points are not in chronological order")
	}
}

// A target the axis does not reach says nothing about whether it is being met,
// so a rule stretches the value range the way data does.
func TestRuleStretchesTheValueRange(t *testing.T) {
	c := single()
	c.Rules = []charty.Rule{{Value: 60, Label: "acceptable"}}
	if _, _, vMin, vMax := c.Bounds(); vMin != 60 || vMax != 93.75 {
		t.Errorf("bounds = %v..%v, want the rule included", vMin, vMax)
	}
}

func TestRuleIsDrawnInEveryFormat(t *testing.T) {
	c := single()
	c.Rules = []charty.Rule{{Value: 90, Label: "acceptable"}}
	for _, format := range []string{"png", "svg", "html"} {
		t.Run(format, func(t *testing.T) {
			out := render(t, format, c, charty.Options{})
			if len(out) == 0 {
				t.Fatal("no output")
			}
			if format == "png" {
				return // nothing to assert on beyond it rendering
			}
			// Dashed, so it never reads as a series.
			if !strings.Contains(out, "dash") && !strings.Contains(out, "stroke-dasharray") {
				t.Error("the rule is not dashed")
			}
			if !strings.Contains(out, "acceptable") {
				t.Error("the rule label is missing")
			}
		})
	}
}

func TestRuleSurvivesTheJSONRoundTrip(t *testing.T) {
	c := single()
	c.Rules = []charty.Rule{{Value: 90, Label: "acceptable"}}
	out := render(t, "json", c, charty.Options{})
	var got charty.Chart
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Rules) != 1 || got.Rules[0].Value != 90 || got.Rules[0].Label != "acceptable" {
		t.Errorf("rules = %+v", got.Rules)
	}
}

func TestRuleMustBeFinite(t *testing.T) {
	c := single()
	c.Rules = []charty.Rule{{Value: math.Inf(1)}}
	if err := charty.Render(&bytes.Buffer{}, "png", c, charty.Options{}); err == nil {
		t.Error("expected an error")
	}
}
