package load

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/script"
)

// sample is one request's outcome.
type sample struct {
	at     time.Time
	dur    time.Duration
	status int    // 0 when no response arrived
	err    string // set when no response arrived
	sent   int
	recv   int
	checks []script.Result
}

// failed is k6's definition: no response, or a 4xx/5xx status.
func (s sample) failed() bool { return s.err != "" || s.status >= 400 }

type stepMetrics struct {
	hist     Histogram // timed requests only
	requests uint64
	failed   uint64
}

type checkCount struct {
	step         int
	source       string
	pass, failed uint64
}

type metrics struct {
	mu        sync.Mutex
	steps     []Step
	all       Histogram // timed requests only
	requests  uint64    // all requests, including ones that failed before sending
	per       []stepMetrics
	failed    uint64
	statuses  map[int]uint64
	errors    map[string]uint64
	checks    map[string]*checkCount // step + "\x00" + source
	checkList []*checkCount
	sent      uint64
	recv      uint64

	// The second being filled, and the closed ones.
	sec      Histogram
	secCount int
	secFail  int
	timeline []Point

	iterations  atomic.Uint64
	dropped     atomic.Uint64
	interrupted atomic.Uint64
}

func newMetrics(steps []Step) *metrics {
	return &metrics{
		steps: steps, per: make([]stepMetrics, len(steps)),
		statuses: map[int]uint64{}, errors: map[string]uint64{}, checks: map[string]*checkCount{},
	}
}

func (m *metrics) record(step int, s sample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// A request that failed before sending (a script or URL problem) is
	// counted but has no time.
	if s.dur > 0 || s.status > 0 {
		m.all.Record(s.dur)
		m.per[step].hist.Record(s.dur)
		m.sec.Record(s.dur)
	}
	m.requests++
	m.per[step].requests++
	m.secCount++
	if s.failed() {
		m.failed++
		m.per[step].failed++
		m.secFail++
	}
	if s.status > 0 {
		m.statuses[s.status]++
	}
	if s.err != "" {
		m.errors[s.err]++
	}
	m.sent += uint64(s.sent)
	m.recv += uint64(s.recv)
	for _, c := range s.checks {
		if c.Capture != nil {
			continue // captures aren't checks
		}
		key := string(rune(step)) + "\x00" + c.Source
		cc := m.checks[key]
		if cc == nil {
			cc = &checkCount{step: step, source: c.Source}
			m.checks[key] = cc
			m.checkList = append(m.checkList, cc)
		}
		if c.Passed {
			cc.pass++
		} else {
			cc.failed++
		}
	}
}

// closeSecond ends the current second of the timeline. Seconds are
// numbered in order (1 is the first), whatever the clock says, so a final
// partial second never leaves a gap.
func (m *metrics) closeSecond(vus int, target float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := Point{Second: len(m.timeline) + 1, Requests: m.secCount, Failed: m.secFail, VUs: vus, Target: target}
	if m.sec.Count() > 0 {
		p.Mean, p.P50, p.P95, p.P99, p.Max = m.sec.Mean(), m.sec.Percentile(50), m.sec.Percentile(95), m.sec.Percentile(99), m.sec.Max()
	}
	m.timeline = append(m.timeline, p)
	m.sec = Histogram{}
	m.secCount, m.secFail = 0, 0
}

// closeLast closes the final, partial second if anything happened in it,
// so the timeline adds up to the totals.
func (m *metrics) closeLast(vus int, target float64) {
	m.mu.Lock()
	pending := m.secCount > 0
	m.mu.Unlock()
	if pending {
		m.closeSecond(vus, target)
	}
}

// Latency is a set of response time statistics.
type Latency struct {
	Min  time.Duration `json:"min"`
	Mean time.Duration `json:"mean"`
	P50  time.Duration `json:"p50"`
	P90  time.Duration `json:"p90"`
	P95  time.Duration `json:"p95"`
	P99  time.Duration `json:"p99"`
	Max  time.Duration `json:"max"`
}

func latencyOf(h *Histogram) Latency {
	return Latency{h.Min(), h.Mean(), h.Percentile(50), h.Percentile(90), h.Percentile(95), h.Percentile(99), h.Max()}
}

// StepSummary is one request's results.
type StepSummary struct {
	Name     string  `json:"name"`
	Requests uint64  `json:"requests"`
	Failed   uint64  `json:"failed"`
	Latency  Latency `json:"latency"`
}

// CheckSummary is how often one test line passed.
type CheckSummary struct {
	Step   string `json:"step"`
	Source string `json:"source"`
	Passed uint64 `json:"passed"`
	Failed uint64 `json:"failed"`
}

// Count is a labelled number, for status codes and error reasons.
type Count struct {
	Label string `json:"label"`
	N     uint64 `json:"n"`
}

// Summary is the result of a run.
type Summary struct {
	Planned     time.Duration     `json:"plannedMs"`
	Duration    time.Duration     `json:"durationMs"`
	Stopped     bool              `json:"stopped"` // ended early by the caller
	Plan        Plan              `json:"plan"`
	Requests    uint64            `json:"requests"`
	Failed      uint64            `json:"failed"`
	RPS         float64           `json:"rps"`
	Latency     Latency           `json:"latency"`
	Steps       []StepSummary     `json:"steps"`
	Statuses    []Count           `json:"statuses"`
	Errors      []Count           `json:"errors"`
	Checks      []CheckSummary    `json:"checks"`
	Iterations  uint64            `json:"iterations"`
	Dropped     uint64            `json:"dropped"`     // rate mode: arrivals with no free virtual user
	Interrupted uint64            `json:"interrupted"` // cut off at the end of the graceful stop
	PeakVUs     int               `json:"peakVus"`
	BytesSent   uint64            `json:"bytesSent"`
	BytesRecv   uint64            `json:"bytesReceived"`
	Timeline    []Point           `json:"timeline"`
	Thresholds  []ThresholdResult `json:"thresholds"`

	hist *Histogram // for thresholds on other percentiles
}

// FailedRate is the share of failed requests, 0..1.
func (s *Summary) FailedRate() float64 {
	if s.Requests == 0 {
		return 0
	}
	return float64(s.Failed) / float64(s.Requests)
}

// CheckRate is the share of passed checks, 0..1, and whether there were any.
func (s *Summary) CheckRate() (float64, bool) {
	var p, f uint64
	for _, c := range s.Checks {
		p += c.Passed
		f += c.Failed
	}
	if p+f == 0 {
		return 1, false
	}
	return float64(p) / float64(p+f), true
}

// OK reports whether every threshold passed.
func (s *Summary) OK() bool {
	for _, t := range s.Thresholds {
		if !t.OK {
			return false
		}
	}
	return true
}

func (m *metrics) summary(elapsed time.Duration, cfg Config) *Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := &Summary{
		Duration: elapsed, Plan: cfg.Plan, Requests: m.requests, Failed: m.failed,
		Latency: latencyOf(&m.all), Iterations: m.iterations.Load(), Dropped: m.dropped.Load(),
		Interrupted: m.interrupted.Load(), BytesSent: m.sent, BytesRecv: m.recv,
		Timeline: append([]Point(nil), m.timeline...),
	}
	all := m.all
	s.hist = &all
	if elapsed > 0 {
		s.RPS = float64(s.Requests) / elapsed.Seconds()
	}
	for i := range m.per {
		s.Steps = append(s.Steps, StepSummary{m.steps[i].Name, m.per[i].requests, m.per[i].failed, latencyOf(&m.per[i].hist)})
	}
	for code, n := range m.statuses {
		s.Statuses = append(s.Statuses, Count{itoa(code), n})
	}
	sort.Slice(s.Statuses, func(i, j int) bool { return s.Statuses[i].Label < s.Statuses[j].Label })
	for e, n := range m.errors {
		s.Errors = append(s.Errors, Count{e, n})
	}
	sort.Slice(s.Errors, func(i, j int) bool { return s.Errors[i].N > s.Errors[j].N })
	for _, c := range m.checkList {
		s.Checks = append(s.Checks, CheckSummary{m.steps[c.step].Name, c.source, c.pass, c.failed})
	}
	for _, p := range m.timeline {
		s.PeakVUs = max(s.PeakVUs, p.VUs)
	}
	s.Thresholds = evaluate(cfg.Thresholds, s)
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
