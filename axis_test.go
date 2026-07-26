package charty_test

import (
	"strings"
	"testing"
	"time"

	"github.com/moznion/charty"
)

// bursty is the shape an octocov history actually has: several reports in one
// afternoon, then days of silence.
func bursty() *charty.Chart {
	base := time.Date(2026, 7, 11, 2, 0, 0, 0, time.UTC)
	var pts []charty.Point
	for i := 0; i < 4; i++ { // a busy afternoon
		pts = append(pts, charty.Point{Time: base.Add(time.Duration(i) * time.Hour), Value: 91 + float64(i)})
	}
	pts = append(pts,
		charty.Point{Time: base.AddDate(0, 0, 8), Value: 95},
		charty.Point{Time: base.AddDate(0, 0, 11), Value: 96},
	)
	return &charty.Chart{Title: "coverage", Series: []charty.Series{{Name: "coverage", Points: pts}}}
}

// The union is what keeps series measured at different moments lined up in
// index mode.
func TestTimelineIsTheSortedUnion(t *testing.T) {
	got := multi().Timeline()
	if len(got) != 3 {
		t.Fatalf("got %d times, want the union of 07-01, 07-02 and 07-03", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i].After(got[i-1]) {
			t.Errorf("timeline is not sorted: %v", got)
		}
	}
}

func TestIndexAxisRendersInEveryFormat(t *testing.T) {
	for _, format := range []string{"png", "svg", "html"} {
		if out := render(t, format, bursty(), charty.Options{XAxis: charty.XIndex}); len(out) == 0 {
			t.Errorf("%s: no output", format)
		}
	}
}

func TestIndexAxisReachesTheBrowser(t *testing.T) {
	page := render(t, "html", bursty(), charty.Options{XAxis: charty.XIndex})
	if !strings.Contains(page, `"xAxis":"index"`) {
		t.Error("the axis mode did not reach the browser")
	}
	if page := render(t, "html", bursty(), charty.Options{}); !strings.Contains(page, `"xAxis":"time"`) {
		t.Error("the default axis mode should be explicit in the payload")
	}
}

func TestUnknownAxisIsRejected(t *testing.T) {
	err := charty.Render(nil, "png", single(), charty.Options{XAxis: "sequence"})
	if err == nil || !strings.Contains(err.Error(), "unknown x axis") {
		t.Errorf("error = %v", err)
	}
}

// Index ticks are spaced by count, so neighbours can share a date. A date
// repeated down the axis reads as a mistake.
func TestIndexTicksDisambiguateSharedDates(t *testing.T) {
	out := render(t, "svg", bursty(), charty.Options{XAxis: charty.XIndex})
	if strings.Count(out, ">Jul 11<") > 1 {
		t.Error("the same date labels two ticks; the layout should have got finer")
	}
	if !strings.Contains(out, "Jul 11 0") {
		t.Errorf("expected a date-and-time tick label")
	}
}

// The tick layout follows the span, so a caller never has to name one.
func TestTimeTicksChooseTheirOwnLayout(t *testing.T) {
	base := time.Date(2026, 7, 11, 2, 0, 0, 0, time.UTC)
	hours := &charty.Chart{Series: []charty.Series{{Points: []charty.Point{
		{Time: base, Value: 1}, {Time: base.Add(4 * time.Hour), Value: 2},
	}}}}
	if out := render(t, "svg", hours, charty.Options{}); !strings.Contains(out, ":00") {
		t.Error("a four-hour span should be labelled with clock times")
	}

	years := &charty.Chart{Series: []charty.Series{{Points: []charty.Point{
		{Time: base, Value: 1}, {Time: base.AddDate(3, 0, 0), Value: 2},
	}}}}
	if out := render(t, "svg", years, charty.Options{}); !strings.Contains(out, "2027") {
		t.Error("a three-year span should be labelled with months and years")
	}
}
