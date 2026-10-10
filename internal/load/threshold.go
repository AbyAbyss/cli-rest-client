package load

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Threshold is a pass/fail rule such as "p95<300ms" or "errors<1%".
type Threshold struct {
	Source string
	metric string // p50 p90 p95 p99 p(N) avg min max med errors checks rps
	pct    float64
	op     string
	value  float64 // ms for times, a fraction for rates, per second for rps
}

// ThresholdResult is a threshold with the measured value.
type ThresholdResult struct {
	Source string `json:"source"`
	Actual string `json:"actual"`
	OK     bool   `json:"ok"`
}

// ParseThresholds reads a comma-separated list: p95<300ms, p(99.9)<1s,
// avg<200ms, max<2s, errors<1%, checks>99%, rps>100.
func ParseThresholds(spec string) ([]Threshold, error) {
	var out []Threshold
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		t, err := parseThreshold(part)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func parseThreshold(s string) (Threshold, error) {
	t := Threshold{Source: s}
	i := strings.IndexAny(s, "<>=!")
	if i <= 0 {
		return t, fmt.Errorf("threshold %q: expected something like p95<300ms or errors<1%%", s)
	}
	name := strings.ToLower(strings.TrimSpace(s[:i]))
	rest := s[i:]
	for _, op := range []string{"<=", ">=", "==", "!=", "<", ">"} {
		if strings.HasPrefix(rest, op) {
			t.op = op
			rest = strings.TrimSpace(rest[len(op):])
			break
		}
	}
	if t.op == "" {
		return t, fmt.Errorf("threshold %q: unknown comparison", s)
	}
	switch {
	case name == "avg" || name == "mean" || name == "min" || name == "max":
		t.metric = map[string]string{"mean": "avg"}[name]
		if t.metric == "" {
			t.metric = name
		}
	case name == "med" || name == "median":
		t.metric, t.pct = "p", 50
	case strings.HasPrefix(name, "p(") && strings.HasSuffix(name, ")"):
		t.metric = "p"
		v, err := strconv.ParseFloat(name[2:len(name)-1], 64)
		if err != nil || v <= 0 || v > 100 {
			return t, fmt.Errorf("threshold %q: percentile must be between 0 and 100", s)
		}
		t.pct = v
	case strings.HasPrefix(name, "p") && len(name) > 1:
		v, err := strconv.ParseFloat(name[1:], 64)
		if err != nil || v <= 0 || v > 100 {
			return t, fmt.Errorf("threshold %q: unknown metric %q", s, name)
		}
		t.metric, t.pct = "p", v
	case name == "errors" || name == "failed" || name == "error_rate":
		t.metric = "errors"
	case name == "checks":
		t.metric = "checks"
	case name == "rps":
		t.metric = "rps"
	default:
		return t, fmt.Errorf("threshold %q: unknown metric %q (use p95, p(99.9), avg, min, max, med, errors, checks or rps)", s, name)
	}
	var err error
	switch t.metric {
	case "errors", "checks":
		if !strings.HasSuffix(rest, "%") {
			return t, fmt.Errorf("threshold %q: give %s as a percentage, like 1%%", s, t.metric)
		}
		t.value, err = strconv.ParseFloat(strings.TrimSuffix(rest, "%"), 64)
		t.value /= 100
	case "rps":
		t.value, err = strconv.ParseFloat(strings.TrimSuffix(rest, "/s"), 64)
	default:
		var d time.Duration
		if n, perr := strconv.ParseFloat(rest, 64); perr == nil {
			d = time.Duration(n * float64(time.Millisecond)) // a bare number is milliseconds
		} else {
			d, err = time.ParseDuration(rest)
		}
		t.value = float64(d) / float64(time.Millisecond)
	}
	if err != nil {
		return t, fmt.Errorf("threshold %q: %q is not a valid value", s, rest)
	}
	return t, nil
}

func evaluate(ts []Threshold, s *Summary) []ThresholdResult {
	var out []ThresholdResult
	for _, t := range ts {
		var actual float64
		var shown string
		ms := func(d time.Duration) {
			actual = float64(d) / float64(time.Millisecond)
			shown = FormatDuration(d)
		}
		switch t.metric {
		case "p":
			ms(s.percentile(t.pct))
		case "avg":
			ms(s.Latency.Mean)
		case "min":
			ms(s.Latency.Min)
		case "max":
			ms(s.Latency.Max)
		case "errors":
			actual = s.FailedRate()
			shown = FormatPercent(actual)
		case "checks":
			actual, _ = s.CheckRate()
			shown = FormatPercent(actual)
		case "rps":
			actual = s.RPS
			shown = strconv.FormatFloat(actual, 'f', 1, 64) + "/s"
		}
		out = append(out, ThresholdResult{Source: t.Source, Actual: shown, OK: compare(actual, t.op, t.value)})
	}
	return out
}

func compare(a float64, op string, b float64) bool {
	switch op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "==":
		return a == b
	case "!=":
		return a != b
	}
	return false
}

// percentile reads the summary's latency, or recomputes it from the
// overall histogram for other percentiles.
func (s *Summary) percentile(p float64) time.Duration {
	switch p {
	case 50:
		return s.Latency.P50
	case 90:
		return s.Latency.P90
	case 95:
		return s.Latency.P95
	case 99:
		return s.Latency.P99
	}
	if s.hist != nil {
		return s.hist.Percentile(p)
	}
	return 0
}

// FormatDuration writes a latency compactly: 840µs, 12.3ms, 1.24s.
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return strconv.FormatInt(d.Microseconds(), 10) + "µs"
	case d < 10*time.Millisecond:
		return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 2, 64) + "ms"
	case d < time.Second:
		return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 1, 64) + "ms"
	case d < time.Minute:
		return strconv.FormatFloat(d.Seconds(), 'f', 2, 64) + "s"
	}
	return d.Round(time.Second).String()
}

// FormatPercent writes a 0..1 rate as a percentage.
func FormatPercent(r float64) string {
	switch {
	case r == 0 || r == 1:
		return strconv.FormatFloat(r*100, 'f', 0, 64) + "%"
	case r < 0.001 || r > 0.999:
		return strconv.FormatFloat(r*100, 'f', 3, 64) + "%"
	}
	return strconv.FormatFloat(r*100, 'f', 2, 64) + "%"
}
