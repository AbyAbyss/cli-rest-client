package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func TestLoadCommand(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	testutil.UseLocal(ws, srv.URL)
	dir := t.TempDir()

	var out, errOut bytes.Buffer
	jsonPath := filepath.Join(dir, "r.json")
	code := Load(context.Background(), strings.NewReader(""), &out, &errOut, ws,
		[]string{"-vus", "3", "-duration", "1500ms", "-threshold", "p95<5s, errors<1%", "-threshold", "checks>99%", "-out", jsonPath, "Auth API"})
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errOut.String())
	}
	s := out.String()
	for _, want := range []string{"Load test: 2 requests per iteration, starting with Auth API/Login", "Plan: 3 virtual users for 1.5s",
		"requests ", "response time   avg", "Auth API/Bearer Token", "Status codes    200 ×", "Checks 100% passed",
		"✓ p95<5s", "✓ errors<1%", "✓ checks>99%", "Results written to"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	var res struct {
		Requests uint64 `json:"requests"`
		Latency  struct {
			P95 float64 `json:"p95Ms"`
		} `json:"latency"`
		Timeline []struct {
			Second int `json:"second"`
		} `json:"timeline"`
		Thresholds []struct {
			OK bool `json:"ok"`
		} `json:"thresholds"`
	}
	b, _ := os.ReadFile(jsonPath)
	if err := json.Unmarshal(b, &res); err != nil || res.Requests == 0 || res.Latency.P95 <= 0 || len(res.Timeline) == 0 || len(res.Thresholds) != 3 {
		t.Fatalf("results file %v: %s", err, b)
	}

	// A threshold that can't hold fails the command; ramping stages and a
	// CSV timeline.
	out.Reset()
	csvPath := filepath.Join(dir, "r.csv")
	code = Load(context.Background(), strings.NewReader(""), &out, &errOut, ws,
		[]string{"-stages", "500ms:4,500ms:0", "-threshold", "p99<1µs", "-out", csvPath, "-q", "User Service/Lookup/Get JSON"})
	if code != 1 || !strings.Contains(out.String(), "✗ p99<1µs") || !strings.Contains(out.String(), "ramping 0 → 4 → 0 virtual users") {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	f, _ := os.Open(csvPath)
	rows, err := csv.NewReader(f).ReadAll()
	f.Close()
	if err != nil || len(rows) < 2 || rows[0][0] != "second" || rows[0][7] != "p95_ms" {
		t.Fatalf("csv %v %v", rows, err)
	}

	// A fixed rate.
	out.Reset()
	code = Load(context.Background(), strings.NewReader(""), &out, &errOut, ws, []string{"-rate", "100/s", "-duration", "1s", "-q", "Health Check"})
	if code != 0 || !strings.Contains(out.String(), "100 requests/s for 1s") || !strings.Contains(out.String(), "busy users") {
		t.Fatalf("rate: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}

func TestLoadCommandStopsOnCancel(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	testutil.UseLocal(ws, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(400*time.Millisecond, cancel)
	var out, errOut bytes.Buffer
	start := time.Now()
	code := Load(ctx, strings.NewReader(""), &out, &errOut, ws, []string{"-vus", "2", "-duration", "1m", "-q", "Health Check"})
	if code != 0 || time.Since(start) > 5*time.Second || !strings.Contains(out.String(), "stopped after") {
		t.Fatalf("exit %d after %v\n%s", code, time.Since(start), out.String())
	}
}

func TestLoadCommandErrors(t *testing.T) {
	ws := storage.SampleWorkspace()
	cases := []struct {
		args []string
		want string
	}{
		{nil, "name at least one request"},
		{[]string{"-stages", "10s:5", "-vus", "3", "Health Check"}, "drop -vus"},
		{[]string{"-rate", "10/s", "-vus", "3", "Health Check"}, "don't mix"},
		{[]string{"-stages", "10s", "Health Check"}, "expected duration:target"},
		{[]string{"-threshold", "speed<fast", "Health Check"}, "unknown metric"},
		{[]string{"-out", "r.txt", "Health Check"}, "must end in .json or .csv"},
		{[]string{"Nope"}, "no request, collection or folder"},
		{[]string{"-env", "Mars", "Health Check"}, "no environment named"},
		{[]string{"API Types/Echo (WebSocket)"}, "no HTTP requests"},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		code := Load(context.Background(), strings.NewReader(""), &out, &errOut, ws, c.args)
		if code != 2 || !strings.Contains(errOut.String(), c.want) {
			t.Errorf("%v: exit %d, want 2 and %q in:\n%s", c.args, code, c.want, errOut.String())
		}
	}

	// Another server needs -yes when nobody can be asked (CI, a pipe).
	var out, errOut bytes.Buffer
	code := Load(context.Background(), strings.NewReader("y\n"), &out, &errOut, ws,
		[]string{"-set", "baseUrl=https://api.example.com", "-duration", "1s", "Health Check"})
	if code != 2 || !strings.Contains(errOut.String(), "api.example.com isn't on this machine") || !strings.Contains(errOut.String(), "-yes") {
		t.Fatalf("remote: exit %d\n%s", code, errOut.String())
	}
}
