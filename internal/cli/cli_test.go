package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/postman"
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

func TestImportCommand(t *testing.T) {
	ws := storage.SampleWorkspace()
	store := &storage.Store{Path: filepath.Join(t.TempDir(), "ws.json")}
	var out, errOut bytes.Buffer
	code := Import(&out, &errOut, ws, store, []string{
		"../postman/testdata/shop.postman_collection.json",
		"../postman/testdata/staging.postman_environment.json",
		"missing.json",
	})
	if code != 1 || !strings.Contains(errOut.String(), "missing.json") {
		t.Fatalf("a missing file should fail the command: %d %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `Imported collection "Shop API": 9 request(s) in 3 folder(s)`) ||
		!strings.Contains(out.String(), `Imported environment "Staging" with 3 variable(s)`) ||
		!strings.Contains(out.String(), "  note: ") {
		t.Fatalf("output:\n%s", out.String())
	}
	saved, _, err := store.Load()
	if err != nil || saved.Environment("Staging") == nil || saved.Collections[len(saved.Collections)-1].Name != "Shop API" {
		t.Fatalf("not saved: %v", err)
	}
	if Import(&out, &errOut, ws, store, nil) != 2 {
		t.Fatal("no files should be a usage error")
	}
}

func TestExportCommand(t *testing.T) {
	ws := storage.SampleWorkspace()
	dir := t.TempDir()
	var out, errOut bytes.Buffer

	file := filepath.Join(dir, "users.json")
	if code := Export(&out, &errOut, ws, []string{"-o", file, "user service/users"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `Exported "Users": 3 request(s), 1 folder(s)`) {
		t.Fatalf("summary: %s", out.String())
	}
	// The file imports back.
	data, _ := os.ReadFile(file)
	back := &models.Workspace{}
	res, err := postman.Import(data, back)
	if err != nil || res.Requests != 3 || res.Collection.Name != "Users" {
		t.Fatalf("re-import: %v %+v", err, res)
	}

	out.Reset()
	if Export(&out, &errOut, ws, []string{"-env", "Local"}) != 0 || !strings.Contains(out.String(), `"_postman_variable_scope": "environment"`) {
		t.Fatalf("env to stdout: %s", out.String())
	}
	out.Reset()
	if Export(&out, &errOut, ws, []string{"-globals"}) != 0 || !strings.Contains(out.String(), `"key": "token"`) {
		t.Fatalf("globals: %s", out.String())
	}
	for _, args := range [][]string{nil, {"Nope"}, {"-env", "Nope"}} {
		if Export(&out, &errOut, ws, args) != 2 {
			t.Errorf("%v should be a usage error", args)
		}
	}
}

func TestCurlCommand(t *testing.T) {
	ws := storage.SampleWorkspace()
	store := &storage.Store{Path: filepath.Join(t.TempDir(), "ws.json")}
	var out, errOut bytes.Buffer

	// As arguments (the leading "curl" is optional), into a folder, with a name.
	code := Curl(strings.NewReader(""), &out, &errOut, ws, store,
		[]string{"-into", "user service/users", "-name", "Promote", "-X", "POST", "https://x.test/users/1/promote", "-H", "X-A: 1", "-k"})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	users := findCollection(ws, "User Service/Users")
	last := users.Requests[len(users.Requests)-1]
	if last.Name != "Promote" || last.Method != "POST" || !strings.Contains(out.String(), "note: -k") {
		t.Fatalf("saved %+v\n%s", last, out.String())
	}
	if saved, _, _ := store.Load(); findCollection(saved, "User Service/Users").CountRequests() != users.CountRequests() {
		t.Fatal("not saved")
	}

	// From stdin, several commands, to the top level.
	out.Reset()
	before := len(ws.Requests)
	if Curl(strings.NewReader("curl https://x.test/a\ncurl https://x.test/b"), &out, &errOut, ws, nil, []string{"-"}) != 0 || len(ws.Requests) != before+2 {
		t.Fatalf("stdin: %s", out.String())
	}
	if Curl(strings.NewReader(""), &out, &errOut, ws, nil, []string{"-into", "Nope", "https://x"}) != 2 {
		t.Fatal("unknown folder should be a usage error")
	}

	// import detects a file of curl commands.
	f := filepath.Join(t.TempDir(), "calls.sh")
	os.WriteFile(f, []byte("curl -X PUT https://x.test/c -d 'k=v'\n"), 0o644)
	out.Reset()
	if Import(&out, &errOut, ws, nil, []string{f}) != 0 || !strings.Contains(out.String(), "Saved PUT https://x.test/c") {
		t.Fatalf("import of curl file: %s %s", out.String(), errOut.String())
	}
}
