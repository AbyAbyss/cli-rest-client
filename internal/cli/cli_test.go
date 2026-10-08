package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

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
	if code != 0 || !strings.Contains(out.String(), `"token":"abc"`) {
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
