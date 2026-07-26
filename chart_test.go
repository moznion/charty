package charty_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/moznion/charty"
)

func at(day int) time.Time {
	return time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC)
}

func single() *charty.Chart {
	return &charty.Chart{
		Title: "coverage",
		Series: []charty.Series{{
			Name: "coverage",
			Unit: "%",
			Points: []charty.Point{
				{Time: at(1), Value: 91.5, Label: "aaaaaaaa", Href: "https://example.com/commit/aaaaaaaa",
					Meta: map[string]string{"ref": "refs/heads/main", "build": "17"}},
				{Time: at(2), Value: 93.75, Label: "cccccccc"},
			},
		}},
	}
}

func multi() *charty.Chart {
	return &charty.Chart{
		Title: "coverage by crate",
		Series: []charty.Series{
			{Name: "core", Unit: "%", Points: []charty.Point{{Time: at(1), Value: 91}, {Time: at(2), Value: 92}}},
			{Name: "cli", Unit: "%", Points: []charty.Point{{Time: at(1), Value: 80}, {Time: at(3), Value: 85}}},
		},
	}
}

func decode(t *testing.T, in string) *charty.Chart {
	t.Helper()
	c, err := charty.Decode(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return c
}

// A bare array of points is the shape that falls out of jq, so it decodes into
// a one-series chart without the caller having to wrap it.
func TestDecodeAcceptsABareArrayOfPoints(t *testing.T) {
	c := decode(t, `[{"t":"2026-07-02T00:00:00Z","v":2},{"t":"2026-07-01T00:00:00Z","v":1}]`)
	if len(c.Series) != 1 {
		t.Fatalf("got %d series, want 1", len(c.Series))
	}
	if got := c.Series[0].Points; len(got) != 2 || got[0].Value != 1 {
		t.Errorf("points = %+v, want them sorted oldest first", got)
	}
}

func TestDecodeAcceptsAChartObject(t *testing.T) {
	c := decode(t, `{"title":"t","series":[{"name":"s","points":[{"t":"2026-07-01T00:00:00Z","v":1}]}]}`)
	if c.Title != "t" || c.Series[0].Name != "s" {
		t.Errorf("chart = %+v", c)
	}
}

func TestDecodeCarriesLabelHrefAndMeta(t *testing.T) {
	c := decode(t, `[{"t":"2026-07-01T00:00:00Z","v":1,"label":"abc","href":"https://e.x/1","meta":{"k":"v"}}]`)
	p := c.Series[0].Points[0]
	if p.Label != "abc" || p.Href != "https://e.x/1" || p.Meta["k"] != "v" {
		t.Errorf("point = %+v", p)
	}
}

func TestDecodeRejectsEmptyInput(t *testing.T) {
	if _, err := charty.Decode(strings.NewReader("")); err == nil {
		t.Error("expected an error")
	}
}

func TestNormalizeSortsChronologically(t *testing.T) {
	c := &charty.Chart{Series: []charty.Series{{Points: []charty.Point{
		{Time: at(3), Value: 3}, {Time: at(1), Value: 1}, {Time: at(2), Value: 2},
	}}}}
	if err := c.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	for i, want := range []float64{1, 2, 3} {
		if c.Series[0].Points[i].Value != want {
			t.Errorf("point %d = %v, want %v", i, c.Series[0].Points[i].Value, want)
		}
	}
}

func TestNormalizeRejectsUnrenderableCharts(t *testing.T) {
	tests := map[string]*charty.Chart{
		"no series": {},
		"no points": {Series: []charty.Series{{Name: "s"}}},
		"no time":   {Series: []charty.Series{{Points: []charty.Point{{Value: 1}}}}},
		"unnamed in a multi-series chart": {Series: []charty.Series{
			{Name: "a", Points: []charty.Point{{Time: at(1), Value: 1}}},
			{Points: []charty.Point{{Time: at(1), Value: 2}}},
		}},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			if err := c.Normalize(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// Percent and seconds on one axis is the dual-axis mistake with the second
// axis left off, so it is refused rather than drawn.
func TestNormalizeRejectsMixedUnits(t *testing.T) {
	c := multi()
	c.Series[1].Unit = "s"
	err := c.Normalize()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "different units") {
		t.Errorf("error = %q", err)
	}
}

func TestNormalizeAllowsUndeclaredUnits(t *testing.T) {
	c := multi()
	c.Series[0].Unit, c.Series[1].Unit = "", ""
	if err := c.Normalize(); err != nil {
		t.Errorf("Normalize: %v", err)
	}
}

func TestNormalizeRejectsMoreSeriesThanThePaletteHolds(t *testing.T) {
	c := &charty.Chart{}
	for i := 0; i <= charty.MaxSeries; i++ {
		c.Series = append(c.Series, charty.Series{
			Name:   string(rune('a' + i)),
			Points: []charty.Point{{Time: at(1), Value: float64(i)}},
		})
	}
	err := c.Normalize()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "split it across charts") {
		t.Errorf("error = %q, want it to suggest the way out", err)
	}
}

func TestNormalizeRejectsNonFiniteValues(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := &charty.Chart{Series: []charty.Series{{Points: []charty.Point{{Time: at(1), Value: v}}}}}
		if err := c.Normalize(); err == nil {
			t.Errorf("value %v: expected an error", v)
		}
	}
}

func TestBoundsSpansEverySeries(t *testing.T) {
	c := multi()
	tMin, tMax, vMin, vMax := c.Bounds()
	if !tMin.Equal(at(1)) || !tMax.Equal(at(3)) {
		t.Errorf("time bounds = %v..%v", tMin, tMax)
	}
	if vMin != 80 || vMax != 92 {
		t.Errorf("value bounds = %v..%v", vMin, vMax)
	}
}

func TestLenCountsEveryPoint(t *testing.T) {
	if got := multi().Len(); got != 4 {
		t.Errorf("Len = %d, want 4", got)
	}
}
