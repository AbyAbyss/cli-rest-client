// Package load runs saved requests under load: virtual users ramping along
// stages, or a fixed or ramping request rate, with latency percentiles, a
// per-second timeline, the requests' tests as checks, and thresholds.
package load

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
)

// Step is one request of the flow each iteration runs.
type Step struct {
	Name    string // shown in results, e.g. "Shop API/Login"
	Request models.Request
}

// Config is a load test.
type Config struct {
	Steps []Step
	Plan  Plan
	// Variables seed every virtual user; each one then keeps its own copy,
	// so values captured by tests (a login token) stay with that user.
	Variables map[string]string
	// MaxVUs caps concurrent iterations in rate mode. Arrivals beyond it
	// are dropped and counted. Zero picks a default from the peak rate.
	MaxVUs int
	// Think is a pause after each iteration in virtual-user mode.
	Think time.Duration
	// Timeout per request; zero means 30 seconds.
	Timeout          time.Duration
	InsecureTLS      bool
	DisableRedirects bool
	// GracefulStop is how long in-flight requests may finish after the
	// plan ends (or Stop is called) before they are cut off. Zero means
	// the smaller of Timeout and 10 seconds.
	GracefulStop time.Duration
	Thresholds   []Threshold
	// Tick is called about once a second, and once more at the end, from
	// the runner's goroutine. It must not block for long.
	Tick func(Progress)
}

// Progress is the live state passed to Tick.
type Progress struct {
	Elapsed  time.Duration `json:"elapsedMs"`
	Total    time.Duration `json:"totalMs"`
	VUs      int           `json:"vus"`    // iterations running now
	Target   float64       `json:"target"` // VUs or requests/s the plan wants now
	Requests uint64        `json:"requests"`
	Failed   uint64        `json:"failed"`
	Point    Point         `json:"point"` // the last full second
	P95      time.Duration `json:"p95"`   // over the whole run so far
	Done     bool          `json:"done"`
}

// Point is one second of the timeline.
type Point struct {
	Second   int           `json:"second"`
	Requests int           `json:"requests"`
	Failed   int           `json:"failed"`
	VUs      int           `json:"vus"`
	Target   float64       `json:"target"`
	Mean     time.Duration `json:"mean"`
	P50      time.Duration `json:"p50"`
	P95      time.Duration `json:"p95"`
	P99      time.Duration `json:"p99"`
	Max      time.Duration `json:"max"`
}

// Validate checks a config before Run.
func (c *Config) Validate() error {
	if len(c.Steps) == 0 {
		return errors.New("nothing to send: no HTTP requests in the selection")
	}
	for _, s := range c.Steps {
		if s.Request.Type == models.TypeWebSocket {
			return fmt.Errorf("%s is a WebSocket request; load tests send HTTP requests only", s.Name)
		}
	}
	if c.Plan.Total() <= 0 {
		return errors.New("the load plan has no duration")
	}
	if c.Plan.Peak() <= 0 {
		return errors.New("the load plan never goes above zero")
	}
	return nil
}

type runner struct {
	cfg    Config
	client *http.Client
	m      *metrics
	start  time.Time
	active atomic.Int64 // iterations in progress
	vus    atomic.Int64 // virtual users running (VU mode)
}

// running is what "VUs" shows: virtual users in VU mode, iterations in
// progress in rate mode.
func (r *runner) running() int {
	if r.cfg.Plan.Rate {
		return int(r.active.Load())
	}
	return int(r.vus.Load())
}

// errHardStop is the cause given to in-flight requests cut off after the
// graceful stop; they are counted as interrupted, not failed.
var errHardStop = errors.New("stopped")

// Run applies the plan until it ends or ctx is cancelled (a graceful stop:
// no new iterations, in-flight ones get GracefulStop to finish). It returns
// the summary, with thresholds evaluated.
func Run(ctx context.Context, cfg Config) (*Summary, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.GracefulStop <= 0 {
		cfg.GracefulStop = min(cfg.Timeout, 10*time.Second)
	}
	if cfg.Plan.Rate && cfg.MaxVUs <= 0 {
		cfg.MaxVUs = max(100, int(cfg.Plan.Peak()*2))
	}
	r := &runner{cfg: cfg, m: newMetrics(cfg.Steps), client: newClient(cfg)}
	defer r.client.CloseIdleConnections()

	// stopNew ends the plan (no new iterations); hard cancels in-flight
	// requests after the graceful period.
	stopNew, endPlan := context.WithCancel(context.Background())
	hard, cutOff := context.WithCancelCause(context.Background())
	defer cutOff(nil)
	var stoppedByUser atomic.Bool
	go func() {
		select {
		case <-ctx.Done():
			stoppedByUser.Store(true)
			endPlan()
		case <-stopNew.Done():
		}
	}()

	r.start = time.Now()
	total := cfg.Plan.Total()
	var wg sync.WaitGroup
	done := make(chan struct{})
	go r.ticker(done)
	if cfg.Plan.Rate {
		r.runRate(stopNew, hard, &wg)
	} else {
		r.runVUs(stopNew, hard, &wg)
	}
	endPlan()
	// Graceful stop: let in-flight iterations finish, then cut them off.
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(cfg.GracefulStop):
		cutOff(errHardStop)
		<-finished
	}
	close(done)
	elapsed := time.Since(r.start)
	r.m.closeLast(r.running(), cfg.Plan.At(elapsed))
	s := r.m.summary(elapsed, cfg)
	s.Stopped = stoppedByUser.Load()
	s.Planned = total
	if cfg.Tick != nil {
		cfg.Tick(r.progress(true))
	}
	return s, nil
}

func newClient(cfg Config) *http.Client {
	conns := 100
	if cfg.Plan.Rate {
		conns = max(conns, cfg.MaxVUs)
	} else {
		conns = max(conns, int(cfg.Plan.Peak()))
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = conns * 2
	t.MaxIdleConnsPerHost = conns
	t.IdleConnTimeout = 90 * time.Second
	if cfg.InsecureTLS {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user opted in
	}
	c := &http.Client{Transport: t}
	if cfg.DisableRedirects {
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return c
}

// runVUs keeps the plan's number of virtual users running. Ramping down
// asks the newest ones to stop after their current iteration.
func (r *runner) runVUs(stopNew, hard context.Context, wg *sync.WaitGroup) {
	var stops []chan struct{}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	total := r.cfg.Plan.Total()
	for {
		t := time.Since(r.start)
		if t >= total || stopNew.Err() != nil {
			break
		}
		want := int(math.Round(r.cfg.Plan.At(t)))
		for len(stops) < want {
			stop := make(chan struct{})
			stops = append(stops, stop)
			wg.Add(1)
			go r.vu(stop, stopNew, hard, wg)
		}
		for len(stops) > want {
			close(stops[len(stops)-1])
			stops = stops[:len(stops)-1]
		}
		select {
		case <-tick.C:
		case <-stopNew.Done():
		}
	}
	for _, s := range stops {
		close(s)
	}
}

func (r *runner) vu(stop <-chan struct{}, stopNew, hard context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	r.vus.Add(1)
	defer r.vus.Add(-1)
	vars := maps.Clone(r.cfg.Variables)
	if vars == nil {
		vars = map[string]string{}
	}
	for {
		select {
		case <-stop:
			return
		case <-stopNew.Done():
			return
		default:
		}
		r.iteration(hard, vars)
		if r.cfg.Think > 0 {
			select {
			case <-time.After(r.cfg.Think):
			case <-stop:
				return
			case <-stopNew.Done():
				return
			}
		}
	}
}

// runRate starts iterations on schedule. Each takes a virtual user from a
// pool of MaxVUs (so their variables carry over between iterations); when
// none is free the arrival is dropped and counted.
func (r *runner) runRate(stopNew, hard context.Context, wg *sync.WaitGroup) {
	pool := make(chan map[string]string, r.cfg.MaxVUs)
	for range r.cfg.MaxVUs {
		pool <- maps.Clone(r.cfg.Variables)
	}
	const step = 5 * time.Millisecond
	tick := time.NewTicker(step)
	defer tick.Stop()
	total := r.cfg.Plan.Total()
	due := 0.0
	last := time.Since(r.start)
	for {
		now := time.Since(r.start)
		if now >= total || stopNew.Err() != nil {
			return
		}
		// Integrate the rate over the time since the last tick.
		mid := last + (now-last)/2
		due += r.cfg.Plan.At(mid) * (now - last).Seconds()
		last = now
		for ; due >= 1; due-- {
			select {
			case vars := <-pool:
				wg.Add(1)
				go func() {
					defer wg.Done()
					if vars == nil {
						vars = map[string]string{}
					}
					r.iteration(hard, vars)
					pool <- vars
				}()
			default:
				r.m.dropped.Add(1)
			}
		}
		select {
		case <-tick.C:
		case <-stopNew.Done():
		}
	}
}

// iteration runs the flow once with a virtual user's variables.
func (r *runner) iteration(hard context.Context, vars map[string]string) {
	r.active.Add(1)
	defer r.active.Add(-1)
	for i, s := range r.cfg.Steps {
		if hard.Err() != nil {
			return
		}
		r.step(hard, i, s, vars)
	}
	r.m.iterations.Add(1)
}

func (r *runner) step(hard context.Context, i int, s Step, vars map[string]string) {
	if strings.TrimSpace(s.Request.PreRequest) != "" {
		if _, errs := script.RunPre(s.Request.PreRequest, vars); len(errs) > 0 {
			r.m.record(i, sample{err: "pre-request script: " + errs[0].Error()})
			return
		}
	}
	p, err := engine.Prepare(s.Request, vars)
	if err != nil {
		r.m.record(i, sample{err: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(hard, r.cfg.Timeout)
	defer cancel()
	started := time.Now()
	resp, err := r.client.Do(p.Request.WithContext(ctx))
	if err != nil {
		if context.Cause(hard) == errHardStop {
			r.m.interrupted.Add(1)
			return
		}
		r.m.record(i, sample{at: started, dur: time.Since(started), err: classify(err, ctx), sent: len(p.Body)})
		return
	}
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	extra, _ := io.Copy(io.Discard, resp.Body) // drain so the connection is reused
	resp.Body.Close()
	dur := time.Since(started)
	if rerr != nil {
		if context.Cause(hard) == errHardStop {
			r.m.interrupted.Add(1)
			return
		}
		r.m.record(i, sample{at: started, dur: dur, err: classify(rerr, ctx), sent: len(p.Body)})
		return
	}
	smp := sample{at: started, dur: dur, status: resp.StatusCode, recv: len(body) + int(extra), sent: len(p.Body)}
	if strings.TrimSpace(s.Request.Tests) != "" {
		smp.checks = script.RunTests(s.Request.Tests, script.Response{
			Status: resp.StatusCode, Headers: resp.Header, Body: body, Duration: dur,
		}, vars)
	}
	r.m.record(i, smp)
}

// maxBody is how much of each response body is kept for the tests.
const maxBody = 10 << 20

// classify turns a transport error into a short, countable reason.
func classify(err error, ctx context.Context) string {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection reset"
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return "connection closed early"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "DNS: " + dns.Err
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && len(msg)-i < 60 {
		msg = msg[i+2:]
	}
	return msg
}

func (r *runner) ticker(done <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			r.m.closeSecond(r.running(), r.cfg.Plan.At(time.Since(r.start)))
			if r.cfg.Tick != nil {
				r.cfg.Tick(r.progress(false))
			}
		}
	}
}

func (r *runner) progress(done bool) Progress {
	el := time.Since(r.start)
	p := Progress{Elapsed: el, Total: r.cfg.Plan.Total(), VUs: r.running(), Target: r.cfg.Plan.At(el), Done: done}
	r.m.mu.Lock()
	p.Requests, p.Failed = r.m.requests, r.m.failed
	p.P95 = r.m.all.Percentile(95)
	if n := len(r.m.timeline); n > 0 {
		p.Point = r.m.timeline[n-1]
	}
	r.m.mu.Unlock()
	return p
}
