package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

// TestSampleWorkspace sends every sample request to a local httpbin clone
// and requires all of their tests to pass.
func TestSampleWorkspace(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()

	ws := storage.SampleWorkspace()
	ws.SetVariable("baseUrl", srv.URL)
	store := &storage.Store{Path: filepath.Join(t.TempDir(), "ws.json")}

	var out, errOut bytes.Buffer
	var names []string
	for _, c := range ws.Collections {
		names = append(names, c.Name)
	}
	names = append(names, "Health Check")
	code := Run(&out, &errOut, ws, store, names)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String(), "✗") || strings.Contains(out.String(), "warning") {
		t.Fatalf("unexpected failures:\n%s", out.String())
	}
	// Captures and pre-request assignments are persisted.
	vars := ws.VariableMap()
	if vars["lastUser"] != "Aby" || !strings.HasPrefix(vars["orderId"], "order-") {
		t.Fatalf("variables not captured: %v", vars)
	}
	saved, _, err := store.Load()
	if err != nil || saved.VariableMap()["lastUser"] != "Aby" {
		t.Fatalf("variables not saved: %v", err)
	}
}

func TestRunFailuresAndOverrides(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()

	var out, errOut bytes.Buffer
	// Wrong token: the bearer test compares against {{token}}, so override
	// only the request side by pointing baseUrl at the server.
	code := Run(&out, &errOut, ws, nil, []string{"-v", "-set", "baseUrl=" + srv.URL, "-set", "token=abc", "auth api/bearer token"})
	if code != 0 || !strings.Contains(out.String(), `"token":"abc"`) ||
		!strings.Contains(out.String(), "  > Authorization: Bearer abc") || !strings.Contains(out.String(), "  < Content-Type: application/json") {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	if ws.VariableMap()["token"] != "my-secret-token" {
		t.Fatalf("-set must not be persisted")
	}

	out.Reset()
	ws.Collections[0].Requests[0].Tests = "status == 418"
	out.Reset()
	if code := Run(&out, &errOut, ws, nil, []string{"-set", "baseUrl=" + srv.URL, "user service/users"}); code != 0 ||
		strings.Count(out.String(), "\n  200 OK") != 3 {
		t.Fatalf("folder run should send the 3 requests under Users (including Admin): %d\n%s", code, out.String())
	}
	if code := Run(&out, &errOut, ws, nil, []string{"-set", "baseUrl=" + srv.URL, "Auth API/Login"}); code != 1 {
		t.Fatalf("expected failing exit code, got %d\n%s", code, out.String())
	}

	if code := Run(&out, &errOut, ws, nil, []string{"Nope/Nothing"}); code != 2 {
		t.Fatalf("expected usage exit code for unknown request, got %d", code)
	}
}

func TestListPaths(t *testing.T) {
	var out bytes.Buffer
	List(&out, storage.SampleWorkspace())
	for _, want := range []string{"User Service/Users/Admin/Delete User", "Payment Gateway/Charges/Charge", "GET     Health Check"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list missing %q:\n%s", want, out.String())
		}
	}
}

func TestHistoryCommand(t *testing.T) {
	store := &storage.Store{Path: filepath.Join(t.TempDir(), "ws.json")}
	var out, errOut bytes.Buffer
	if History(&out, &errOut, store, nil) != 0 || !strings.Contains(out.String(), "No history yet") {
		t.Fatalf("empty: %s", out.String())
	}
	h := &models.History{}
	h.Add(&models.HistoryEntry{Time: time.Now(), Method: "GET", URL: "http://a/1", Status: 200, DurationMs: 5, Source: "C / R"})
	h.Add(&models.HistoryEntry{Time: time.Now(), Method: "POST", URL: "http://a/2", Error: "refused"})
	store.SaveHistory(h)
	out.Reset()
	History(&out, &errOut, store, []string{"-n", "1"})
	if !strings.Contains(out.String(), "POST    ERR") || strings.Contains(out.String(), "http://a/1") {
		t.Fatalf("-n 1: %s", out.String())
	}
	out.Reset()
	History(&out, &errOut, store, nil)
	if !strings.Contains(out.String(), "GET     200      5 ms  http://a/1  (C / R)") {
		t.Fatalf("all: %s", out.String())
	}
}

func TestEnvSelection(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	ws.Environment("Local").Variables[0].Value = srv.URL // Local's baseUrl -> test server
	store := &storage.Store{Path: filepath.Join(t.TempDir(), "ws.json")}

	var out, errOut bytes.Buffer
	// -env Local for one run: uses Local's token, captures go into Local,
	// and the active environment stays httpbin.org.
	code := Run(&out, &errOut, ws, store, []string{"-v", "-env", "local", "Auth API/Bearer Token", "User Service/Users/Create User"})
	if code != 0 || !strings.Contains(out.String(), "Environment: Local") || !strings.Contains(out.String(), "> Authorization: Bearer local-dev-token") {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errOut.String())
	}
	if ws.ActiveEnvironment != "httpbin.org" {
		t.Fatalf("active env changed to %q", ws.ActiveEnvironment)
	}
	saved, _, _ := store.Load()
	if saved.ActiveEnvironment != "httpbin.org" {
		t.Fatalf("saved active env %q", saved.ActiveEnvironment)
	}
	if !hasVar(saved.Environment("Local").Variables, "lastUser") || hasVar(saved.Variables, "lastUser") {
		t.Fatal("captured variable should go into the environment used")
	}
	if code := Run(&out, &errOut, ws, nil, []string{"-env", "Staging", "Health Check"}); code != 2 {
		t.Fatalf("unknown env should be a usage error, got %d", code)
	}

	// env lists and switches.
	out.Reset()
	Env(&out, &errOut, ws, store, nil)
	if !strings.Contains(out.String(), "* httpbin.org") || !strings.Contains(out.String(), "  Local (") {
		t.Fatalf("list:\n%s", out.String())
	}
	out.Reset()
	if Env(&out, &errOut, ws, store, []string{"Local"}) != 0 || ws.ActiveEnvironment != "Local" {
		t.Fatal("switch failed")
	}
	if saved, _, _ := store.Load(); saved.ActiveEnvironment != "Local" {
		t.Fatal("switch not saved")
	}
	Env(&out, &errOut, ws, store, []string{"none"})
	if ws.Active() != nil {
		t.Fatal("none should clear the environment")
	}
	if Env(&out, &errOut, ws, store, []string{"Nope"}) != 2 {
		t.Fatal("unknown env should fail")
	}
}

func hasVar(kvs []models.KeyValue, key string) bool {
	for _, kv := range kvs {
		if kv.Key == key {
			return true
		}
	}
	return false
}
