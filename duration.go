package charty

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Duration is a time.Duration that reads and writes as a string in JSON, so a
// chart can carry "168h" rather than a count of nanoseconds nobody can check
// by eye.
type Duration time.Duration

// Duration returns the value as a time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// MarshalJSON writes the duration as a string, e.g. "168h0m0s".
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON reads a duration written as a string, in Go's syntax or with
// the d and w suffixes.
func (d *Duration) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"24h\" or \"7d\": %w", err)
	}
	v, err := ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

var dayWeekRe = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(d|w)$`)

// ParseDuration reads Go's duration syntax, plus the d and w suffixes that
// time.ParseDuration lacks and that anyone describing a metrics history
// reaches for first.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if m := dayWeekRe.FindStringSubmatch(s); m != nil {
		n, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, fmt.Errorf("cannot read %q as a duration: %w", s, err)
		}
		unit := 24 * time.Hour
		if m[2] == "w" {
			unit = 7 * 24 * time.Hour
		}
		d := time.Duration(n * float64(unit))
		if d <= 0 {
			return 0, fmt.Errorf("duration %q must be positive", s)
		}
		return d, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("cannot read %q as a duration (try 1d, 1w, 12h)", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration %q must be positive", s)
	}
	return d, nil
}
