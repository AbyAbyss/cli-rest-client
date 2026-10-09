package postman

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
	"github.com/AbyAbyss/cli-rest-client/pkg/httpclient"
)

// requestsByPath lists the requests under c keyed by their path inside it.
func requestsByPath(c *models.Collection) map[string]*models.Request {
	out := map[string]*models.Request{}
	var walk func(prefix string, c *models.Collection)
	walk = func(prefix string, c *models.Collection) {
		for _, f := range c.Folders {
			walk(prefix+f.Name+"/", f)
		}
		for _, r := range c.Requests {
			out[prefix+r.Name] = r
		}
	}
	walk("", c)
	return out
}

// TestRoundTripPostmanCollection: Postman -> app -> Postman -> app gives
// the same requests, scripts included (the original JavaScript is reused).
func TestRoundTripPostmanCollection(t *testing.T) {
	ws := &models.Workspace{}
	first, err := Import(load(t, "shop.postman_collection.json"), ws)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := ExportCollection(first.Collection)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Requests != 9 || exp.Folders != 3 || len(exp.Notes) != 0 {
		t.Fatalf("export counts/notes: %d %d %v", exp.Requests, exp.Folders, exp.Notes)
	}
	second, err := Import(exp.Data, ws)
	if err != nil {
		t.Fatal(err)
	}
	a, b := requestsByPath(first.Collection), requestsByPath(second.Collection)
	if len(a) != len(b) {
		t.Fatalf("request count %d vs %d", len(a), len(b))
	}
	for path, r1 := range a {
		r2 := b[path]
		if r2 == nil {
			t.Errorf("%s missing after round trip", path)
			continue
		}
		if !r1.Equal(*r2) {
			j1, _ := json.MarshalIndent(r1, "", " ")
			j2, _ := json.MarshalIndent(r2, "", " ")
			t.Errorf("%s changed in round trip:\n%s\n---\n%s", path, j1, j2)
		}
	}

	// The exported file restores Postman's original script text.
	if !strings.Contains(string(exp.Data), `"pm.environment.set(\"firstId\", jsonData.items[0].id);"`) {
		t.Fatal("original JavaScript not restored")
	}
}

// TestRoundTripSampleWorkspace: app -> Postman -> app keeps every request
// field except scripts, which are compared by behaviour in
// TestExportedTestsBehaveTheSame.
func TestRoundTripSampleWorkspace(t *testing.T) {
	src := storage.SampleWorkspace()
	for _, c := range src.Collections {
		exp, err := ExportCollection(c)
		if err != nil {
			t.Fatal(err)
		}
		dst := &models.Workspace{}
		res, err := Import(exp.Data, dst)
		if err != nil {
			t.Fatal(err)
		}
		before, after := requestsByPath(c), requestsByPath(res.Collection)
		for path, r1 := range before {
			if r1.Type == models.TypeWebSocket {
				continue // not representable in a collection file (see TestExportSkipsWebSockets)
			}
			r2 := after[path]
			if r2 == nil {
				t.Fatalf("%s/%s missing", c.Name, path)
			}
			c1, c2 := r1.Clone(), r2.Clone()
			c1.PreRequest, c1.Tests, c2.PreRequest, c2.Tests = "", "", "", ""
			if !c1.Equal(c2) {
				j1, _ := json.MarshalIndent(c1, "", " ")
				j2, _ := json.MarshalIndent(c2, "", " ")
				t.Errorf("%s/%s changed:\n%s\n---\n%s", c.Name, path, j1, j2)
			}
		}
	}
}

func TestTestLineTranslations(t *testing.T) {
	cases := map[string]string{
		"status == 200":                     `pm.response.to.have.status(200);`,
		"status < 300":                      `pm.expect(pm.response.code).to.be.below(300);`,
		"time <= 500":                       `pm.expect(pm.response.responseTime).to.be.at.most(500);`,
		"header Content-Type contains json": `pm.expect(pm.response.headers.get("Content-Type")).to.include("json");`,
		"header X-Id exists":                `pm.expect(pm.response.headers.has("X-Id")).to.be.true;`,
		`body contains "two words"`:         `pm.expect(pm.response.text()).to.include("two words");`,
		"json.items[0].id == 42":            `pm.expect(Number(pm.response.json().items[0].id)).to.eql(42);`,
		"json.name == Aby":                  `pm.expect(pm.response.json().name).to.eql("Aby");`,
		"json.token == {{token}}":           `pm.expect(String(pm.response.json().token)).to.eql(pm.variables.replaceIn("{{token}}"));`,
		"json.ok == true":                   `pm.expect(String(pm.response.json().ok)).to.eql("true");`,
		"json.name matches ^A":              `pm.expect(pm.response.json().name).to.match(new RegExp("^A"));`,
		"json.error !exists":                `pm.expect(pm.response.json().error).to.be.undefined;`,
		"set token = json.data.token":       `pm.environment.set("token", pm.response.json().data.token);`,
		"set loc = header Location":         `pm.environment.set("loc", pm.response.headers.get("Location"));`,
	}
	for line, want := range cases {
		got, ok := testLineToJS(line)
		if !ok || !strings.Contains(got, want) {
			t.Errorf("%q:\n got  %s\n want %s", line, got, want)
		}
	}
	if _, ok := testLineToJS("nonsense here"); ok {
		t.Error("unknown lines must not translate")
	}
	if got, _ := preLineToJS("set id = {{$uuid}}"); got != `pm.environment.set("id", pm.variables.replaceIn("{{$randomUUID}}"));` {
		t.Errorf("pre set: %s", got)
	}

	res := &ExportResult{}
	js := exportScript("status == 200\n# a comment\nwhat is this", false, res, "C / R")
	if len(js) != 3 || js[1] != "// a comment" || js[2] != "// term-rest-client: what is this" || len(res.Notes) != 1 {
		t.Fatalf("script export: %v notes %v", js, res.Notes)
	}
}

func TestExportVariables(t *testing.T) {
	env := &models.Environment{Name: "Prod", Variables: []models.KeyValue{{Key: "a", Value: "1"}, {Key: "off", Value: "x", Disabled: true}, {Key: "id", Value: "{{$uuid}}"}}}
	data, err := ExportEnvironment(env)
	if err != nil {
		t.Fatal(err)
	}
	ws := &models.Workspace{}
	res, err := Import(data, ws)
	if err != nil || res.Kind != "environment" {
		t.Fatalf("%v %+v", err, res)
	}
	got := ws.Environment("Prod")
	if got == nil || len(got.Variables) != 3 || !got.Variables[1].Disabled || got.Variables[2].Value != "{{$uuid}}" {
		t.Fatalf("env round trip: %+v", got)
	}
	data, _ = ExportGlobals([]models.KeyValue{{Key: "g", Value: "v"}})
	res, err = Import(data, ws)
	if err != nil || res.Kind != "globals" || ws.VariableMap()["g"] != "v" {
		t.Fatalf("globals round trip: %v %+v", err, res)
	}
}

// pmStub is a small stand-in for Postman's sandbox: pm.test, pm.expect
// (the chai chains the exporter emits), pm.response and pm.environment.
const pmStub = `
const fs = require('fs');
const ctx = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const vars = ctx.vars;
const results = [];
function deepEql(a, b) { return JSON.stringify(a) === JSON.stringify(b); }
function expect(v) {
  const neg = { v: false };
  const check = (ok, msg) => { if (neg.v ? ok : !ok) throw new Error(msg); };
  const api = {
    get to() { return api; }, get be() { return api; }, get have() { return api; }, get at() { return api; },
    get not() { neg.v = !neg.v; return api; },
    eql: (x) => check(deepEql(v, x), 'eql'),
    below: (x) => check(v < x, 'below'), above: (x) => check(v > x, 'above'),
    most: (x) => check(v <= x, 'most'), least: (x) => check(v >= x, 'least'),
    include: (x) => check(String(v).includes(x), 'include'),
    match: (re) => check(re.test(String(v)), 'match'),
    get true() { check(v === true, 'true'); return api; }, get false() { check(v === false, 'false'); return api; },
    get undefined() { check(v === undefined, 'undefined'); return api; },
  };
  return api;
}
const headers = Object.fromEntries(Object.entries(ctx.headers).map(([k, v]) => [k.toLowerCase(), v]));
const pm = {
  test: (name, fn) => { try { fn(); results.push([name, true]); } catch (e) { results.push([name, false]); } },
  expect,
  response: {
    code: ctx.status, responseTime: ctx.time, responseSize: ctx.body.length,
    text: () => ctx.body, json: () => JSON.parse(ctx.body),
    headers: { get: (k) => headers[k.toLowerCase()], has: (k) => k.toLowerCase() in headers },
    to: { have: { status: (s) => { if (ctx.status !== s) throw new Error('status'); } } },
  },
  environment: { set: (k, v) => { vars[k] = String(v); }, unset: (k) => { delete vars[k]; } },
  variables: { replaceIn: (s) => s.replace(/\{\{\s*([^}]+?)\s*\}\}/g, (m, k) => (k in vars ? vars[k] : m)) },
};
eval(ctx.script);
process.stdout.write(JSON.stringify({ results, vars }));
`

// TestExportedTestsBehaveTheSame sends every sample request to a local
// httpbin clone, runs its tests natively and as the exported Postman
// JavaScript (in node, with pmStub), and requires the same outcome for
// each assertion and the same captured variables.
func TestExportedTestsBehaveTheSame(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	testutil.UseLocal(ws, srv.URL)
	dir := t.TempDir()
	stub := filepath.Join(dir, "pm.js")
	os.WriteFile(stub, []byte(pmStub), 0o644)
	client := httpclient.NewClient(httpclient.Options{})

	checked := 0
	ws.WalkRequests(func(_ []*models.Collection, r *models.Request) {
		if strings.TrimSpace(r.Tests) == "" || r.Type == models.TypeWebSocket {
			return
		}
		vars := ws.VariableMap()
		script.RunPre(r.PreRequest, vars)
		p, err := engine.Prepare(*r, vars)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(context.Background(), p.Request)
		if err != nil {
			t.Fatal(err)
		}

		nativeVars := map[string]string{}
		for k, v := range vars {
			nativeVars[k] = v
		}
		var events []string
		for _, e := range resp.Events {
			events = append(events, e.Data)
		}
		native := script.RunTests(r.Tests, script.Response{Status: resp.StatusCode, Headers: resp.Headers, Body: resp.Body, Duration: resp.Duration, Events: events}, nativeVars)

		res := &ExportResult{}
		js := exportScript(r.Tests, false, res, r.Name)
		if len(res.Notes) > 0 {
			t.Fatalf("%s: untranslated lines %v", r.Name, res.Notes)
		}
		hdrs := map[string]string{}
		for k, v := range resp.Headers {
			hdrs[k] = strings.Join(v, ", ")
		}
		ctx, _ := json.Marshal(map[string]any{
			"script": strings.Join(js, "\n"), "status": resp.StatusCode, "time": resp.Duration.Milliseconds(),
			"body": string(resp.Body), "headers": hdrs, "vars": vars,
		})
		ctxFile := filepath.Join(dir, "ctx.json")
		os.WriteFile(ctxFile, ctx, 0o644)
		out, err := exec.Command(node, stub, ctxFile).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: node failed: %v\n%s\nscript:\n%s", r.Name, err, out, strings.Join(js, "\n"))
		}
		var got struct {
			Results [][2]any          `json:"results"`
			Vars    map[string]string `json:"vars"`
		}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("%s: %v: %s", r.Name, err, out)
		}
		i := 0
		for _, n := range native {
			if n.Capture != nil {
				if got.Vars[n.Capture.Name] != n.Capture.Value {
					t.Errorf("%s: capture %s = %q in JS, %q natively", r.Name, n.Capture.Name, got.Vars[n.Capture.Name], n.Capture.Value)
				}
				continue
			}
			if i >= len(got.Results) {
				t.Fatalf("%s: JS ran fewer tests than native", r.Name)
			}
			if got.Results[i][1].(bool) != n.Passed {
				t.Errorf("%s: %q passed=%v natively but %v in Postman JS", r.Name, n.Source, n.Passed, got.Results[i][1])
			}
			i++
			checked++
		}
	})
	if checked < 15 {
		t.Fatalf("only %d assertions compared", checked)
	}
}

func TestExportSkipsWebSockets(t *testing.T) {
	ws := models.NewRequest("Socket")
	ws.Type, ws.URL = models.TypeWebSocket, "wss://x.test"
	get := models.NewRequest("Get")
	get.URL = "https://x.test"
	res, err := ExportCollection(&models.Collection{Name: "Mixed", Requests: []*models.Request{&ws, &get}})
	if err != nil || res.Requests != 1 || len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "Mixed / Socket: WebSocket") {
		t.Fatalf("%v %d %v", err, res.Requests, res.Notes)
	}
}
