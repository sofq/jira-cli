package duration

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Jira time conventions.
const (
	minuteSecs = 60
	hourSecs   = 3600
	daySecs    = 8 * hourSecs  // 1d = 8h
	weekSecs   = 5 * daySecs   // 1w = 5d
)

var unitPattern = regexp.MustCompile(`(\d+)\s*(w|d|h|m)`)

// fullPattern validates the entire string is composed only of valid duration tokens
// separated by optional whitespace.
var fullPattern = regexp.MustCompile(`^(\d+\s*(w|d|h|m)\s*)+$`)

// Parse converts a human duration string (e.g. "2h", "1d 3h", "30m") to seconds.
// Supported units: w (weeks), d (days), h (hours), m (minutes).
// Jira convention: 1d = 8h, 1w = 5d = 40h.
func Parse(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	if !fullPattern.MatchString(s) {
		return 0, fmt.Errorf("invalid duration %q: expected format like 2h, 1d 3h, 30m", s)
	}

	matches := unitPattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration %q: expected format like 2h, 1d 3h, 30m", s)
	}

	total := 0
	for _, m := range matches {
		n, _ := strconv.Atoi(m[1]) // regex guarantees digits
		switch m[2] {
		case "w":
			total += n * weekSecs
		case "d":
			total += n * daySecs
		case "h":
			total += n * hourSecs
		case "m":
			total += n * minuteSecs
		}
	}

	return total, nil
}
