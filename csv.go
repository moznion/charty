package charty

import (
	"encoding/csv"
	"io"
	"sort"
	"strconv"
	"time"
)

// renderCSV writes one row per point, at full precision. Metadata keys become
// columns: the union across every point, so a sparse key simply leaves blanks
// rather than shifting the row.
func renderCSV(w io.Writer, c *Chart) error {
	metaKeys := metaColumns(c)

	header := append([]string{"series", "timestamp", "value", "label", "href"}, metaKeys...)
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, s := range c.Series {
		for _, p := range s.Points {
			row := []string{
				s.Name,
				p.Time.Format(time.RFC3339Nano),
				strconv.FormatFloat(p.Value, 'f', -1, 64),
				p.Label,
				p.Href,
			}
			for _, k := range metaKeys {
				row = append(row, p.Meta[k])
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func metaColumns(c *Chart) []string {
	seen := map[string]struct{}{}
	for _, s := range c.Series {
		for _, p := range s.Points {
			for k := range p.Meta {
				seen[k] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
