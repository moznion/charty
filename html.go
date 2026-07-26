package charty

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed html.tmpl
var htmlFS embed.FS

var htmlTemplate = template.Must(template.ParseFS(htmlFS, "html.tmpl"))

// jsPoint is the compact per-point shape handed to the browser. Metadata is
// pre-ordered here so the script never has to sort it.
type jsPoint struct {
	T     int64       `json:"t"`
	V     float64     `json:"v"`
	Label string      `json:"label,omitempty"`
	Href  string      `json:"href,omitempty"`
	Meta  [][2]string `json:"meta,omitempty"`
}

type jsSeries struct {
	Name string `json:"name"`
	Unit string `json:"unit,omitempty"`
	// Gap is in milliseconds, the unit the browser measures time in.
	Gap    int64     `json:"gap,omitempty"`
	Points []jsPoint `json:"points"`
}

// htmlStat is one series' entry in the summary row, which doubles as the
// legend: identity is never carried by colour alone.
type htmlStat struct {
	Class  string
	Name   string
	Unit   string
	Latest string
	Delta  string
	Since  string
}

type htmlRow struct {
	Class     string
	Series    string
	Timestamp string
	Value     string
	Label     string
	Href      string
	Meta      []string
}

type htmlPage struct {
	Title      string
	Count      int
	Span       string
	Multi      bool
	Stats      []htmlStat
	MetaCols   []string
	Rows       []htmlRow
	SeriesCSS  template.CSS
	Data       template.JS
	Generator  string
	LabelTitle string
}

// renderHTML writes one self-contained page: no external requests and no
// bundled charting library. The chart is drawn as SVG by a small inline
// script, which keeps the file to a few tens of kilobytes and lets it share
// the visual language of the static renderers.
func renderHTML(w io.Writer, c *Chart, opts Options) error {
	jsSeriesList := make([]jsSeries, len(c.Series))
	stats := make([]htmlStat, len(c.Series))
	var css strings.Builder

	for i, s := range c.Series {
		points := make([]jsPoint, len(s.Points))
		for j, p := range s.Points {
			points[j] = jsPoint{
				T:     p.Time.UnixMilli(),
				V:     p.Value,
				Label: p.Label,
				Href:  p.Href,
				Meta:  orderedMeta(p.Meta),
			}
		}
		name := s.Name
		if name == "" {
			name = "value"
		}
		gap := opts.Gap
		if s.Gap > 0 {
			gap = s.Gap.Duration()
		}
		jsSeriesList[i] = jsSeries{Name: name, Unit: s.Unit, Gap: gap.Milliseconds(), Points: points}

		first, last := s.Points[0], s.Points[len(s.Points)-1]
		stats[i] = htmlStat{
			Class:  fmt.Sprintf("s%d", i),
			Name:   name,
			Unit:   s.Unit,
			Latest: formatValue(last.Value),
			Delta:  formatDelta(last.Value - first.Value),
			Since:  first.Time.Local().Format(dateOnly),
		}

		// Colour is emitted as a class rather than an inline attribute so the
		// dark step can be a real media query, and so SVG marks can pick it up
		// through currentColor.
		fmt.Fprintf(&css, "  .s%d { color: %s; }\n", i, lightHex(i))
	}
	css.WriteString("  @media (prefers-color-scheme: dark) {\n")
	for i := range c.Series {
		fmt.Fprintf(&css, "    .s%d { color: %s; }\n", i, seriesHex(i))
	}
	css.WriteString("  }\n")

	payload := map[string]any{"series": jsSeriesList, "rules": c.Rules, "xAxis": string(opts.XAxis)}
	if opts.YMin != nil {
		payload["yMin"] = *opts.YMin
	}
	if opts.YMax != nil {
		payload["yMax"] = *opts.YMax
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tMin, tMax, _, _ := c.Bounds()
	metaCols := metaColumns(c)
	page := htmlPage{
		Title:      firstNonEmpty(c.Title, jsSeriesList[0].Name),
		Count:      c.Len(),
		Span:       fmt.Sprintf("%s – %s", tMin.Local().Format(dateOnly), tMax.Local().Format(dateOnly)),
		Multi:      len(c.Series) > 1,
		Stats:      stats,
		MetaCols:   metaCols,
		Rows:       tableRows(c, metaCols),
		SeriesCSS:  template.CSS(css.String()),
		Data:       template.JS(data), // encoding/json escapes <, > and &
		Generator:  opts.Generator,
		LabelTitle: labelColumnTitle(c),
	}
	return htmlTemplate.Execute(w, page)
}

// Server-rendered dates are ISO. They are exact metadata, and unlike the chart
// axis — which the browser formats in the reader's locale — they cannot know
// that locale at generation time.
const dateOnly = "2006-01-02"

// tableRows flattens every series newest-first, the way a log reads. The table
// is rendered as markup so the numbers survive with JavaScript disabled and
// are reachable without hovering.
func tableRows(c *Chart, metaCols []string) []htmlRow {
	var rows []htmlRow
	for i, s := range c.Series {
		for _, p := range s.Points {
			meta := make([]string, len(metaCols))
			for k, key := range metaCols {
				meta[k] = p.Meta[key]
			}
			rows = append(rows, htmlRow{
				Class:     fmt.Sprintf("s%d", i),
				Series:    s.Name,
				Timestamp: p.Time.Format(time.RFC3339),
				Value:     strconv.FormatFloat(p.Value, 'f', -1, 64),
				Label:     p.Label,
				Href:      p.Href,
				Meta:      meta,
			})
		}
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].Timestamp > rows[b].Timestamp })
	return rows
}

func orderedMeta(m map[string]string) [][2]string {
	if len(m) == 0 {
		return nil
	}
	keys := sortedMetaKeys(m)
	out := make([][2]string, len(keys))
	for i, k := range keys {
		out[i] = [2]string{k, m[k]}
	}
	return out
}

// labelColumnTitle avoids heading a column "Label" when every label is
// obviously something else; callers name it through the metadata instead.
func labelColumnTitle(c *Chart) string {
	for _, s := range c.Series {
		for _, p := range s.Points {
			if p.Label != "" {
				return "Label"
			}
		}
	}
	return ""
}

// formatDelta keeps the sign explicit; "no change" reads better than "+0".
func formatDelta(d float64) string {
	switch {
	case d > 0:
		return "+" + formatValue(d)
	case d < 0:
		return "−" + formatValue(-d)
	default:
		return "no change"
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
