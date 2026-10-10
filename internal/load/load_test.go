package load

import (
	"context"
	"encoding/json"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func TestHistogramPercentiles(t *testing.T) {
	var h Histogram
	vals := make([]time.Duration, 0, 20000)
	for range 20000 {
		// A long-tailed mix, like real latencies: mostly 5-50ms, some seconds.
		v := time.Duration(rand.ExpFloat64()*15e3+5e3) * time.Microsecond
		if rand.IntN(100) == 0 {
			v *= 50
		}
		vals = append(vals, v)
		h.Record(v)
	}
	slices.Sort(vals)
	for _, p := range []float64{1, 50, 90, 95, 99, 99.9} {
		exact := vals[int(math.Ceil(p/100*float64(len(vals))))-1]
		got := h.Percentile(p)
		if diff := math.Abs(float64(got-exact)) / float64(exact); diff > 0.035 {
			t.Errorf("p%v = %v, exact %v (%.1f%% off)", p, got, exact, diff*100)
		}
	}
	if h.Min() != vals[0].Truncate(time.Microsecond) || h.Max() != vals[len(vals)-1].Truncate(time.Microsecond) {
		t.Errorf("min/max %v %v, want %v %v", h.Min(), h.Max(), vals[0], vals[len(vals)-1])
	}
	var a, b Histogram
	a.Record(time.Millisecond)
	b.Record(3 * time.Millisecond)
	a.Merge(&b)
	if a.Count() != 2 || a.Mean() != 2*time.Millisecond || a.Max() != 3*time.Millisecond {
		t.Errorf("merge: n %d mean %v max %v", a.Count(), a.Mean(), a.Max())
	}
	var empty Histogram
	if empty.Percentile(95) != 0 || empty.Mean() != 0 {
		t.Error("empty histogram should report zeros")
	}
}

func TestStages(t *testing.T) {
	p, err := ParseStages("30s:50, 1m:50,30s:0")
	if err != nil || p.Rate || len(p.Stages) != 3 || p.Total() != 2*time.Minute || p.Peak() != 50 {
		t.Fatalf("%+v %v", p, err)
	}
	for _, c := range []struct {
		at   time.Duration
		want float64
	}{{0, 0}, {15 * time.Second, 25}, {30 * time.Second, 50}, {time.Minute, 50}, {105 * time.Second, 25}, {3 * time.Minute, 0}} {
		if got := p.At(c.at); got != c.want {
			t.Errorf("At(%v) = %v, want %v", c.at, got, c.want)
		}
	}
	if p.String() != "30s:50,1m0s:50,30s:0" {
		t.Errorf("String %q", p.String())
	}
	r, err := ParseStages("10s:100/s,20s:100/s")
	if err != nil || !r.Rate || r.At(5*time.Second) != 50 {
		t.Fatalf("rate stages %+v %v", r, err)
	}
	for _, bad := range []string{"", "30s", "x:5", "10s:5,10s:5/s", "10s:-1", "10s:2.5", "0s:5"} {
		if _, err := ParseStages(bad); err == nil {
			t.Errorf("ParseStages(%q) should fail", bad)
		}
	}
	if v, err := ParseRate("1200/m"); err != nil || v != 20 {
		t.Errorf("ParseRate: %v %v", v, err)
	}
	c := Constant(10, time.Minute, false)
	if c.At(0) != 10 || c.At(59*time.Second) != 10 {
		t.Error("constant plan should hold its target from the start")
	}
}

func TestThresholds(t *testing.T) {
	ts, err := ParseThresholds("p95<300ms, p(99.9)<=2s, avg<200, max<5s, med<100ms, errors<1%, checks>=99%, rps>10")
	if err != nil || len(ts) != 8 {
		t.Fatalf("%v %v", ts, err)
	}
	var h Histogram
	for i := range 100 {
		h.Record(time.Duration(i+1) * time.Millisecond)
	}
	s := &Summary{Requests: 1000, Failed: 20, RPS: 50, Latency: latencyOf(&h), hist: &h,
		Checks: []CheckSummary{{Passed: 995, Failed: 5}}}
	res := evaluate(ts, s)
	ok := map[string]bool{}
	for _, r := range res {
		ok[r.Source] = r.OK
	}
	if !ok["p95<300ms"] || !ok["p(99.9)<=2s"] || !ok["avg<200"] || !ok["max<5s"] || !ok["med<100ms"] || ok["errors<1%"] || !ok["checks>=99%"] || !ok["rps>10"] {
		t.Fatalf("results %+v", res)
	}
	if res[5].Actual != "2.00%" {
		t.Errorf("errors actual %q", res[5].Actual)
	}
	for _, bad := range []string{"p95", "foo<1", "errors<1", "p95<fast", "p(200)<1s"} {
		if _, err := ParseThresholds(bad); err == nil {
			t.Errorf("ParseThresholds(%q) should fail", bad)
		}
	}
}

func steps(base string, reqs ...models.Request) []Step {
	var out []Step
	for _, r := range reqs {
		out = append(out, Step{Name: r.Name, Request: r})
	}
	return out
}

func req(name, method, url, tests string) models.Request {
	r := models.NewRequest(name)
	r.Method, r.URL, r.Tests = method, url, tests
	return r
}

func TestRunVirtualUsersWithFlow(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	// Each virtual user "logs in" with its own random token, then must send
	// exactly that token back: captured values stay per user.
	login := req("Login", "POST", "{{base}}/post", "status == 200\nset tok = json.json.token")
	login.BodyType, login.Body = models.BodyJSON, `{"token":"t{{$randomInt}}"}`
	login.PreRequest = "set n = {{$randomInt}}"
	use := req("Use token", "GET", "{{base}}/bearer", "status == 200\njson.token == {{tok}}")
	use.Auth = models.Auth{Type: models.AuthBearer, Token: "{{tok}}"}

	var ticks []Progress
	s, err := Run(context.Background(), Config{
		Steps: steps(srv.URL, login, use), Variables: map[string]string{"base": srv.URL},
		Plan: Constant(4, 1500*time.Millisecond, false),
		Tick: func(p Progress) { ticks = append(ticks, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Requests < 40 || s.Failed != 0 || s.Iterations < 20 || s.Steps[0].Requests != s.Steps[1].Requests && s.Steps[0].Requests != s.Steps[1].Requests+4 {
		t.Fatalf("requests %d failed %d iterations %d steps %+v errors %+v", s.Requests, s.Failed, s.Iterations, s.Steps, s.Errors)
	}
	if rate, any := s.CheckRate(); !any || rate != 1 || len(s.Checks) != 3 {
		t.Fatalf("checks %+v", s.Checks)
	}
	if s.Statuses[0].Label != "200" || s.Latency.P50 <= 0 || s.Latency.Max < s.Latency.P95 || s.BytesRecv == 0 || s.BytesSent == 0 {
		t.Fatalf("summary %+v", s)
	}
	if len(ticks) < 2 || !ticks[len(ticks)-1].Done || ticks[0].VUs > 4 || len(s.Timeline) < 1 || s.Timeline[0].Requests == 0 || s.PeakVUs != 4 {
		t.Fatalf("ticks %+v timeline %+v peak %d", ticks, s.Timeline, s.PeakVUs)
	}
}

func TestRunRampAndThink(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	plan, _ := ParseStages("1s:6,1s:6,1s:0")
	s, err := Run(context.Background(), Config{
		Steps: steps("", req("Get", "GET", srv.URL+"/get", "")), Plan: plan, Think: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Up to 6 users, each about 20 iterations a second with the pause.
	if s.PeakVUs < 4 || s.PeakVUs > 6 || s.Requests < 60 || s.Requests > 400 || s.Failed != 0 {
		t.Fatalf("peak %d requests %d failed %d timeline %+v", s.PeakVUs, s.Requests, s.Failed, s.Timeline)
	}
	sum := 0
	for _, p := range s.Timeline {
		sum += p.Requests
	}
	most := 0
	for _, p := range s.Timeline {
		most = max(most, p.VUs)
	}
	for i, p := range s.Timeline {
		if p.Second != i+1 {
			t.Fatalf("timeline seconds out of order: %+v", s.Timeline)
		}
	}
	if uint64(sum) != s.Requests || most < 5 || most > 6 || s.Duration > 4*time.Second {
		t.Fatalf("ramp %+v in %v", s.Timeline, s.Duration)
	}
}

func TestRunRate(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	s, err := Run(context.Background(), Config{
		Steps: steps("", req("Get", "GET", srv.URL+"/get", "")), Plan: Constant(200, 2*time.Second, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Requests < 340 || s.Requests > 420 || s.Dropped != 0 || math.Abs(s.RPS-200) > 30 {
		t.Fatalf("requests %d rps %.1f dropped %d", s.Requests, s.RPS, s.Dropped)
	}

	// A slow server with too few virtual users drops arrivals instead of
	// quietly lowering the rate.
	s, err = Run(context.Background(), Config{
		Steps: steps("", req("Slow", "GET", srv.URL+"/slow?ms=300", "")), Plan: Constant(40, time.Second, true), MaxVUs: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Dropped < 20 || s.Requests > 12 {
		t.Fatalf("dropped %d requests %d", s.Dropped, s.Requests)
	}
}

func TestRunFailuresAndThresholds(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	th, _ := ParseThresholds("errors<1%, p95<5s")
	s, err := Run(context.Background(), Config{
		Steps:      steps("", req("Flaky", "GET", srv.URL+"/flaky?fail=50&ms=1", "status == 200")),
		Plan:       Constant(4, time.Second, false),
		Thresholds: th,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r := s.FailedRate(); r < 0.3 || r > 0.7 || len(s.Statuses) != 2 {
		t.Fatalf("failed rate %.2f statuses %+v", r, s.Statuses)
	}
	if s.OK() || s.Thresholds[0].OK || !s.Thresholds[1].OK {
		t.Fatalf("thresholds %+v", s.Thresholds)
	}
	if rate, _ := s.CheckRate(); rate > 0.7 {
		t.Fatalf("check rate %.2f", rate)
	}

	// Nothing listening: every request fails with a readable reason.
	s, _ = Run(context.Background(), Config{
		Steps: steps("", req("Down", "GET", "http://127.0.0.1:1/x", "")), Plan: Constant(2, 300*time.Millisecond, false),
	})
	if s.Requests == 0 || s.Failed != s.Requests || len(s.Errors) == 0 || s.Errors[0].Label != "connection refused" {
		t.Fatalf("down: %+v", s.Errors)
	}
	// A request that can't be built is counted as failed, without a time.
	s, _ = Run(context.Background(), Config{
		Steps: steps("", req("Bad", "GET", "", "")), Plan: Constant(1, 100*time.Millisecond, false),
	})
	if s.Requests == 0 || s.Failed != s.Requests || !strings.Contains(s.Errors[0].Label, "URL is empty") || s.Latency.Max != 0 {
		t.Fatalf("bad: %+v %+v", s.Errors, s.Latency)
	}
}

func TestRunStopAndGracefulCutoff(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	s, err := Run(ctx, Config{Steps: steps("", req("Get", "GET", srv.URL+"/get", "")), Plan: Constant(3, time.Minute, false)})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Stopped || s.Duration > 3*time.Second || s.Requests == 0 {
		t.Fatalf("stopped %v after %v with %d requests", s.Stopped, s.Duration, s.Requests)
	}

	// Requests still running after the graceful stop are cut off and
	// counted as interrupted, not failed.
	s, _ = Run(context.Background(), Config{
		Steps: steps("", req("Slow", "GET", srv.URL+"/slow?ms=5000", "")), Plan: Constant(3, 200*time.Millisecond, false),
		GracefulStop: 300 * time.Millisecond,
	})
	if s.Interrupted != 3 || s.Failed != 0 || s.Duration > 2*time.Second {
		t.Fatalf("interrupted %d failed %d in %v", s.Interrupted, s.Failed, s.Duration)
	}
}

func TestValidate(t *testing.T) {
	ws := req("Socket", "GET", "ws://x", "")
	ws.Type = models.TypeWebSocket
	for _, c := range []Config{
		{Plan: Constant(1, time.Second, false)},
		{Steps: steps("", ws), Plan: Constant(1, time.Second, false)},
		{Steps: steps("", req("a", "GET", "http://x", "")), Plan: Constant(0, time.Second, false)},
	} {
		if _, err := Run(context.Background(), c); err == nil {
			t.Errorf("%+v should not run", c)
		}
	}
}

func TestJSONInMilliseconds(t *testing.T) {
	var h Histogram
	h.Record(1500 * time.Microsecond)
	s := &Summary{Duration: 2 * time.Second, Planned: 2 * time.Second, Latency: latencyOf(&h),
		Plan: Constant(5, 30*time.Second, false), Timeline: []Point{{Second: 1, P95: 3 * time.Millisecond}}}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{`"durationMs":2000`, `"plannedMs":2000`, `"p95Ms":1.5`, `"durationMs":30000,"target":5`, `"second":1`, `"p95Ms":3`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	var p Plan
	if err := json.Unmarshal([]byte(`{"rate":true,"start":0,"stages":[{"durationMs":1500,"target":20}]}`), &p); err != nil || p.Stages[0].Duration != 1500*time.Millisecond || !p.Rate {
		t.Fatalf("plan from JSON: %+v %v", p, err)
	}
}

func TestDescribeAndLocal(t *testing.T) {
	for _, c := range []struct {
		p    Plan
		want string
	}{
		{Constant(50, 2*time.Minute, false), "50 virtual users for 2m"},
		{Constant(1, 90*time.Second, false), "1 virtual user for 1m30s"},
		{Constant(200, time.Minute, true), "200 requests/s for 1m"},
	} {
		if got := c.p.Describe(); got != c.want {
			t.Errorf("Describe = %q, want %q", got, c.want)
		}
	}
	p, _ := ParseStages("30s:50,1m:50,30s:0")
	if got := p.Describe(); got != "ramping 0 → 50 → 0 virtual users over 2m" {
		t.Errorf("ramp: %q", got)
	}
	for h, want := range map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true, "10.1.2.3": true, "192.168.1.20": true,
		"api.local": true, "example.com": false, "8.8.8.8": false, "api.example.com": false} {
		if IsLocal(h) != want {
			t.Errorf("IsLocal(%q) = %v", h, !want)
		}
	}
}

func TestTimelineHasNoShortTail(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	s, err := Run(context.Background(), Config{
		Steps: steps("", req("Get", "GET", srv.URL+"/slow?ms=30", "")), Plan: Constant(4, 2*time.Second, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, p := range s.Timeline {
		sum += p.Requests
	}
	if len(s.Timeline) != 2 || uint64(sum) != s.Requests || s.Timeline[1].VUs == 0 {
		t.Fatalf("timeline %+v (total %d)", s.Timeline, s.Requests)
	}
}
