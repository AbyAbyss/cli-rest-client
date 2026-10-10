package load

import (
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
)

// Hosts lists the hosts the steps send to, with variables resolved.
func Hosts(steps []Step, vars map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range steps {
		p, err := engine.Prepare(s.Request, vars)
		if err != nil {
			continue
		}
		h := p.Request.URL.Hostname()
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

// IsLocal reports whether host is this machine or a private network
// address, where load testing needs no confirmation.
func IsLocal(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

// Remote is the hosts that aren't local.
func Remote(hosts []string) []string {
	var out []string
	for _, h := range hosts {
		if !IsLocal(h) {
			out = append(out, h)
		}
	}
	return out
}

// Describe writes the plan in words: "50 virtual users for 2m0s" or
// "ramping 0 → 50 → 0 virtual users over 2m0s".
func (p Plan) Describe() string {
	unit := " virtual users"
	if p.Rate {
		unit = " requests/s"
	}
	format := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	if len(p.Stages) == 1 && p.Start == p.Stages[0].Target {
		if p.Rate {
			return format(p.Start) + unit + " for " + shortDur(p.Total())
		}
		n := format(p.Start)
		if n == "1" {
			unit = " virtual user"
		}
		return n + unit + " for " + shortDur(p.Total())
	}
	parts := []string{format(p.Start)}
	for _, s := range p.Stages {
		if v := format(s.Target); v != parts[len(parts)-1] || len(p.Stages) == 1 {
			parts = append(parts, v)
		}
	}
	return "ramping " + strings.Join(parts, " → ") + unit + " over " + shortDur(p.Total())
}

// shortDur writes 2m, 1m30s, 45s or 500ms.
func shortDur(d time.Duration) string {
	if d < time.Second {
		return d.String()
	}
	if d < time.Minute && d%time.Second != 0 {
		return strconv.FormatFloat(d.Round(100*time.Millisecond).Seconds(), 'f', -1, 64) + "s"
	}
	d = d.Round(time.Second)
	h, m, sec := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if sec > 0 || b.Len() == 0 {
		fmt.Fprintf(&b, "%ds", sec)
	}
	return b.String()
}

func group(n uint64) string {
	s := strconv.FormatUint(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func bytesText(n uint64) string {
	switch {
	case n < 1024:
		return strconv.FormatUint(n, 10) + " B"
	case n < 1<<20:
		return strconv.FormatFloat(float64(n)/1024, 'f', 1, 64) + " KB"
	case n < 1<<30:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	}
	return strconv.FormatFloat(float64(n)/(1<<30), 'f', 2, 64) + " GB"
}

// WriteSummary prints the results for a terminal, like k6's end-of-test
// summary.
func WriteSummary(w io.Writer, s *Summary) {
	line := func(label, value string) { fmt.Fprintf(w, "  %-15s %s\n", label, value) }
	how := "ran " + shortDur(s.Duration)
	if s.Stopped {
		how = "stopped after " + shortDur(s.Duration) + " of " + shortDur(s.Planned)
	}
	fmt.Fprintf(w, "\n  %s, %s\n\n", s.Plan.Describe(), how)
	line("requests", fmt.Sprintf("%-10s %s/s", group(s.Requests), rateText(s.RPS)))
	line("failed", fmt.Sprintf("%-10s %s of %s (no response, or 4xx/5xx)", FormatPercent(s.FailedRate()), group(s.Failed), group(s.Requests)))
	line("iterations", group(s.Iterations))
	l := s.Latency
	line("response time", fmt.Sprintf("avg %s  p50 %s  p90 %s  p95 %s  p99 %s  max %s",
		FormatDuration(l.Mean), FormatDuration(l.P50), FormatDuration(l.P90), FormatDuration(l.P95), FormatDuration(l.P99), FormatDuration(l.Max)))
	line("data", fmt.Sprintf("%s received, %s sent", bytesText(s.BytesRecv), bytesText(s.BytesSent)))
	if s.Plan.Rate {
		line("busy users", fmt.Sprintf("%d at peak", s.PeakVUs))
	} else {
		line("virtual users", fmt.Sprintf("%d at peak", s.PeakVUs))
	}
	if s.Dropped > 0 {
		line("dropped", fmt.Sprintf("%s iterations had no free virtual user (raise -max-vus, or the server is too slow for this rate)", group(s.Dropped)))
	}
	if s.Interrupted > 0 {
		line("interrupted", fmt.Sprintf("%s requests were still running at the end and were cut off", group(s.Interrupted)))
	}

	if len(s.Steps) > 1 {
		fmt.Fprintf(w, "\n  Requests\n")
		width := 0
		for _, st := range s.Steps {
			width = max(width, len([]rune(st.Name)))
		}
		width = min(width, 40)
		for _, st := range s.Steps {
			fmt.Fprintf(w, "    %-*s %9s  avg %-8s p95 %-8s failed %s\n", width, trunc(st.Name, width), group(st.Requests),
				FormatDuration(st.Latency.Mean), FormatDuration(st.Latency.P95), group(st.Failed))
		}
	}
	if len(s.Statuses) > 0 {
		var parts []string
		for _, c := range s.Statuses {
			parts = append(parts, c.Label+" × "+group(c.N))
		}
		fmt.Fprintf(w, "\n  %-15s %s\n", "Status codes", strings.Join(parts, "   "))
	}
	if len(s.Errors) > 0 {
		fmt.Fprintf(w, "  %-15s", "Errors")
		for i, c := range s.Errors {
			if i == 5 {
				fmt.Fprintf(w, "\n  %-15s and %d more", "", len(s.Errors)-5)
				break
			}
			if i > 0 {
				fmt.Fprintf(w, "\n  %-15s", "")
			}
			fmt.Fprintf(w, " %s × %s", trunc(c.Label, 70), group(c.N))
		}
		fmt.Fprintln(w)
	}
	if rate, any := s.CheckRate(); any {
		fmt.Fprintf(w, "\n  Checks %s passed\n", FormatPercent(rate))
		width := 0
		for _, c := range s.Checks {
			width = max(width, len([]rune(c.Source)))
		}
		width = min(width, 50)
		multi := len(s.Steps) > 1
		stepWidth := 0
		for _, c := range s.Checks {
			stepWidth = max(stepWidth, len([]rune(c.Step)))
		}
		stepWidth = min(stepWidth, 32)
		for _, c := range s.Checks {
			mark := "✓"
			if c.Failed > 0 {
				mark = "✗"
			}
			where := ""
			if multi {
				where = fmt.Sprintf("%-*s  ", stepWidth, trunc(c.Step, stepWidth))
			}
			extra := ""
			if c.Failed > 0 {
				extra = fmt.Sprintf("  (%s failed)", group(c.Failed))
			}
			fmt.Fprintf(w, "    %s %s%-*s %s / %s%s\n", mark, where, width, trunc(c.Source, width), group(c.Passed), group(c.Passed+c.Failed), extra)
		}
	}
	if len(s.Thresholds) > 0 {
		fmt.Fprintf(w, "\n  Thresholds\n")
		width := 0
		for _, t := range s.Thresholds {
			width = max(width, len(t.Source))
		}
		for _, t := range s.Thresholds {
			mark := "✓"
			if !t.OK {
				mark = "✗"
			}
			fmt.Fprintf(w, "    %s %-*s  %s\n", mark, width, t.Source, t.Actual)
		}
	}
	fmt.Fprintln(w)
}

// rateText writes 35,294.7 or 12.5.
func rateText(r float64) string {
	whole := uint64(r)
	return group(whole) + "." + strconv.Itoa(int((r-float64(whole))*10))
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ProgressLine is the one-line live status for a terminal.
func ProgressLine(p Progress, rate bool) string {
	frac := 0.0
	if p.Total > 0 {
		frac = min(float64(p.Elapsed)/float64(p.Total), 1)
	}
	const width = 16
	filled := int(frac * width)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	who := fmt.Sprintf("VUs %d/%d", p.VUs, int(p.Target+0.5))
	if rate {
		who = fmt.Sprintf("target %.0f/s  busy %d", p.Target, p.VUs)
	}
	failed := "0%"
	if p.Requests > 0 {
		failed = FormatPercent(float64(p.Failed) / float64(p.Requests))
	}
	return fmt.Sprintf("%s / %s  %s  %s  %d req/s  p95 %s  failed %s  (%s requests)",
		clock(p.Elapsed), clock(p.Total), bar, who, p.Point.Requests, FormatDuration(p.Point.P95), failed, group(p.Requests))
}

func clock(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// WriteCSV writes the per-second timeline.
func WriteCSV(w io.Writer, s *Summary) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"second", "requests", "failed", "vus", "target", "mean_ms", "p50_ms", "p95_ms", "p99_ms", "max_ms"})
	f := func(d time.Duration) string { return strconv.FormatFloat(ms(d), 'f', 3, 64) }
	for _, p := range s.Timeline {
		_ = cw.Write([]string{strconv.Itoa(p.Second), strconv.Itoa(p.Requests), strconv.Itoa(p.Failed), strconv.Itoa(p.VUs),
			strconv.FormatFloat(p.Target, 'f', 2, 64), f(p.Mean), f(p.P50), f(p.P95), f(p.P99), f(p.Max)})
	}
	cw.Flush()
	return cw.Error()
}
