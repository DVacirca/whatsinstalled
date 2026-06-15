package scanner

import (
	"strconv"
	"strings"
	"time"
)

// dockerSizeUnits maps docker's human-readable size suffixes to byte multipliers.
// docker formats sizes in decimal (1000-based) units via go-units.
var dockerSizeUnits = []struct {
	suffix string
	mult   float64
}{
	{"PB", 1e15}, {"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1},
}

// parseDockerSize converts a docker image size string like "1.23GB" or "456MB"
// into bytes. Returns nil if it can't be parsed.
func parseDockerSize(s string) *int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, u := range dockerSizeUnits {
		if strings.HasSuffix(s, u.suffix) {
			num := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return nil
			}
			b := int64(f * u.mult)
			return &b
		}
	}
	return nil
}

// parseDockerCreated parses docker's CreatedAt field, e.g.
// "2024-01-15 10:30:00 +0000 UTC". Returns nil if it can't be parsed.
func parseDockerCreated(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}
