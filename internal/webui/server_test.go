package webui

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

type harness struct {
	t    *testing.T
	s    *Server
	srv  *httptest.Server
	demo *httptest.Server
}

func start(t *testing.T) *harness {
	t.Helper()
	demo := testutil.NewHTTPBin()
	ws := storage.SampleWorkspace()
	testutil.UseLocal(ws, demo.URL)
	h := &harness{t: t, demo: demo}
	// Bind first so the server knows its own address for the Host check.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h.s, err = New(ws, nil, Info{Version: "test"}, "http://"+l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	h.srv = &httptest.Server{Listener: l, Config: &http.Server{Handler: h.s.Handler()}}
	h.srv.Start()
	t.Cleanup(func() { h.srv.Close(); demo.Close() })
	return h
}

func (h *harness) do(method, path, body string, hdr map[string]string) *http.Response {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	if _, ok := hdr["X-TRC-Token"]; !ok {
		req.Header.Set("X-TRC-Token", h.s.Token())
	}
	for k, v := range hdr {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

func (h *harness) json(method, path string, in any, out any) {
	h.t.Helper()
	b, _ := json.Marshal(in)
	if in == nil {
		b = nil
	}
	res := h.do(method, path, string(b), nil)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(res.Body)
		h.t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, msg)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			h.t.Fatal(err)
		}
	}
}

func TestGuards(t *testing.T) {
	h := start(t)
	// No token.
	if res := h.do("GET", "/api/state", "", map[string]string{"X-TRC-Token": ""}); res.StatusCode != 401 {
		t.Fatalf("no token: %d", res.StatusCode)
	}
	// Another site's page.
	if res := h.do("PUT", "/api/workspace", "{}", map[string]string{"Origin": "http://evil.test"}); res.StatusCode != 403 {
		t.Fatalf("cross origin: %d", res.StatusCode)
	}
	// DNS rebinding: a different Host name.
	req, _ := http.NewRequest("GET", h.srv.URL+"/api/state", nil)
	req.Host = "evil.test"
	req.Header.Set("X-TRC-Token", h.s.Token())
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("bad host: %d", res.StatusCode)
	}
	// The page without the key explains what to do.
	res := h.do("GET", "/", "", map[string]string{"X-TRC-Token": ""})
	if b, _ := io.ReadAll(res.Body); res.StatusCode != 401 || !strings.Contains(string(b), "one-time key") {
		t.Fatalf("index without key: %d %s", res.StatusCode, b)
	}
	// The printed link sets the cookie and redirects; the cookie opens the page and the API.
	res = h.do("GET", "/?t="+h.s.Token(), "", map[string]string{"X-TRC-Token": ""})
	if res.StatusCode != http.StatusSeeOther || len(res.Cookies()) != 1 {
		t.Fatalf("login: %d %v", res.StatusCode, res.Cookies())
	}
	c := res.Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags: %+v", c)
	}
	for _, path := range []string{"/", "/api/state", "/assets/app.js", "/assets/app.css"} {
		req, _ := http.NewRequest("GET", h.srv.URL+path, nil)
		req.AddCookie(c)
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("%s with cookie: %v %d", path, err, res.StatusCode)
		}
		if path == "/" && !strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src 'self'") {
			t.Fatal("index needs a CSP")
		}
	}
	if res := h.do("GET", "/?t=wrong", "", map[string]string{"X-TRC-Token": ""}); res.StatusCode != 401 {
		t.Fatalf("wrong key: %d", res.StatusCode)
	}
}

func TestStateAndWorkspace(t *testing.T) {
	h := start(t)
	var st struct {
		Workspace models.Workspace `json:"workspace"`
		Languages []langJSON       `json:"languages"`
		Info      Info             `json:"info"`
	}
	h.json("GET", "/api/state", nil, &st)
	if len(st.Workspace.Collections) == 0 || len(st.Languages) < 5 || st.Info.Version != "test" {
		t.Fatalf("state: %d collections, %d languages, %+v", len(st.Workspace.Collections), len(st.Languages), st.Info)
	}
	st.Workspace.Collections = append(st.Workspace.Collections, &models.Collection{Name: "From the page"})
	h.json("PUT", "/api/workspace", st.Workspace, nil)
	if last := h.s.ws.Collections[len(h.s.ws.Collections)-1]; last.Name != "From the page" {
		t.Fatalf("not saved: %q", last.Name)
	}
	if res := h.do("PUT", "/api/workspace", "{nope", nil); res.StatusCode != 400 {
		t.Fatalf("bad JSON: %d", res.StatusCode)
	}
}

// sendLines posts to /api/send and returns the NDJSON events.
func (h *harness) sendLines(body any) []map[string]any {
	h.t.Helper()
	b, _ := json.Marshal(body)
	res := h.do("POST", "/api/send", string(b), nil)
	defer res.Body.Close()
	var out []map[string]any
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			h.t.Fatalf("bad line %q", sc.Text())
		}
		out = append(out, m)
	}
	return out
}

func TestSend(t *testing.T) {
	h := start(t)
	r := models.NewRequest("Echo")
	r.Method, r.URL = "POST", "{{baseUrl}}/post"
	r.Headers = []models.KeyValue{{Key: "X-Who", Value: "{{who}}"}}
	r.BodyType, r.Body = models.BodyJSON, `{"a":1}`
	r.PreRequest = "set who = page"
	r.Tests = "status == 200\njson.json.a == 1\nset echoed = json.headers.X-Who"
	lines := h.sendLines(sendRequest{Request: r, Source: "Tests / Echo"})
	if len(lines) != 2 || lines[0]["kind"] != "start" || lines[1]["kind"] != "done" {
		t.Fatalf("lines: %v", lines)
	}
	if !strings.HasSuffix(lines[0]["url"].(string), "/post") {
		t.Fatalf("start: %v", lines[0])
	}
	done := lines[1]
	resp := done["response"].(map[string]any)
	if resp["status"].(float64) != 200 || !strings.Contains(resp["body"].(string), `"X-Who":"page"`) {
		t.Fatalf("response: %v", resp)
	}
	tests := done["tests"].([]any)
	for _, x := range tests {
		if !x.(map[string]any)["passed"].(bool) {
			t.Fatalf("test failed: %v", x)
		}
	}
	if done["workspace"] == nil || h.s.ws.VariableMap()["echoed"] != "page" {
		t.Fatal("captured variables should come back with the workspace")
	}
	if h.s.hist.Entries[0].Source != "Tests / Echo" || h.s.hist.Entries[0].Status != 200 {
		t.Fatalf("history: %+v", h.s.hist.Entries[0])
	}

	// Nothing listening: an error event, recorded in history.
	r = models.NewRequest("Down")
	r.URL = "http://127.0.0.1:1/x"
	lines = h.sendLines(sendRequest{Request: r})
	if last := lines[len(lines)-1]; last["kind"] != "error" || !strings.Contains(last["error"].(string), "refused") {
		t.Fatalf("down: %v", lines)
	}
	if h.s.hist.Entries[0].Error == "" {
		t.Fatal("failed sends belong in history too")
	}
}

func TestSendStreamAndStop(t *testing.T) {
	h := start(t)
	r := models.NewRequest("SSE")
	r.URL = h.demo.URL + "/sse?count=200&interval=20ms"
	r.Tests = "events >= 2"
	b, _ := json.Marshal(sendRequest{Request: r, ID: "abc"})
	res := h.do("POST", "/api/send", string(b), nil)
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	var kinds []string
	var done map[string]any
	events := 0
	for sc.Scan() {
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		kinds = append(kinds, m["kind"].(string))
		if m["kind"] == "events" {
			events += len(m["events"].([]any))
			if events >= 3 && len(kinds) < 100 {
				var stopped map[string]bool
				h.json("POST", "/api/stop", map[string]string{"id": "abc"}, &stopped)
				if !stopped["stopped"] {
					t.Fatal("stop should find the send")
				}
				kinds = append(kinds, "STOP")
			}
		}
		if m["kind"] == "done" {
			done = m
		}
	}
	if kinds[0] != "start" || kinds[1] != "head" || done == nil {
		t.Fatalf("kinds %v", kinds)
	}
	resp := done["response"].(map[string]any)
	if resp["stopped"] != true || resp["stream"] != "sse" || len(resp["events"].([]any)) < 3 {
		t.Fatalf("stopped response: %v", resp)
	}
	if tests := done["tests"].([]any); len(tests) != 1 || !tests[0].(map[string]any)["passed"].(bool) {
		t.Fatalf("tests after stop: %v", done["tests"])
	}
	var stopped map[string]bool
	h.json("POST", "/api/stop", map[string]string{"id": "abc"}, &stopped)
	if stopped["stopped"] {
		t.Fatal("a finished send can't be stopped")
	}
}

func TestSocketBridge(t *testing.T) {
	h := start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.srv.URL, "http")+"/api/socket", &websocket.DialOptions{
		HTTPHeader: http.Header{"X-TRC-Token": {h.s.Token()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	r := models.NewRequest("WS")
	r.Type, r.URL = models.TypeWebSocket, "{{wsUrl}}"
	r.Tests = "events >= 2\nevent[-1].json.n == 7"
	read := func(want string) map[string]any {
		t.Helper()
		for {
			var m map[string]any
			if err := wsjson.Read(ctx, c, &m); err != nil {
				t.Fatalf("waiting for %s: %v", want, err)
			}
			if m["type"] == "error" && want != "error" {
				t.Fatalf("error: %v", m)
			}
			if m["type"] == want {
				return m
			}
		}
	}
	_ = wsjson.Write(ctx, c, map[string]any{"type": "connect", "request": r})
	if m := read("connected"); m["status"].(float64) != 101 {
		t.Fatalf("connected: %v", m)
	}
	read("message") // the demo server's welcome
	h.s.ws.SetVariable("n", "7")
	_ = wsjson.Write(ctx, c, map[string]any{"type": "send", "message": `{"n":{{n}}}`})
	if m := read("message"); m["dir"] != "out" || m["data"] != `{"n":7}` {
		t.Fatalf("sent: %v", m)
	}
	if m := read("message"); m["dir"] != "in" || m["data"] != `{"n":7}` {
		t.Fatalf("echo: %v", m)
	}
	_ = wsjson.Write(ctx, c, map[string]any{"type": "close"})
	m := read("closed")
	tests := m["tests"].([]any)
	if len(tests) != 2 || !tests[0].(map[string]any)["passed"].(bool) || !tests[1].(map[string]any)["passed"].(bool) {
		t.Fatalf("socket tests: %v", m)
	}
	// Without the key the bridge refuses.
	_, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.srv.URL, "http")+"/api/socket", nil)
	if err == nil || res == nil || res.StatusCode != 401 {
		t.Fatalf("socket without key: %v", err)
	}
}

func TestCodeSchemaImportExportHistory(t *testing.T) {
	h := start(t)
	r := models.NewRequest("Get")
	r.URL = "{{baseUrl}}/get?q={{q}}"
	var code struct {
		Text    string   `json:"text"`
		Missing []string `json:"missing"`
	}
	h.json("POST", "/api/code", map[string]any{"request": r, "lang": "python"}, &code)
	if !strings.Contains(code.Text, "import requests") || len(code.Missing) != 1 || code.Missing[0] != "q" {
		t.Fatalf("code: %+v", code)
	}

	g := models.NewRequest("GQL")
	g.Method, g.URL, g.BodyType = "POST", "{{graphqlUrl}}", models.BodyGraphQL
	var sc struct {
		Roots map[string]string     `json:"roots"`
		Types map[string]schemaType `json:"types"`
	}
	h.json("POST", "/api/schema", map[string]any{"request": g}, &sc)
	q := sc.Types[sc.Roots["query"]]
	found := false
	for _, f := range q.Fields {
		if f.Name == "country" && strings.HasPrefix(f.Query, "query Country($code: ID!)") && f.TypeString == "Country" {
			found = true
		}
	}
	if !found {
		t.Fatalf("schema query fields: %+v", q.Fields)
	}

	var one struct {
		Request models.Request `json:"request"`
	}
	h.json("POST", "/api/import", map[string]string{"text": "curl -X PUT https://x.test/a -H 'A: b' -d '{\"k\":1}'"}, &one)
	if one.Request.Method != "PUT" || one.Request.URL != "https://x.test/a" {
		t.Fatalf("curl import: %+v", one.Request)
	}
	before := len(h.s.ws.Collections)
	var many struct {
		Summary string `json:"summary"`
	}
	h.json("POST", "/api/import", map[string]string{"text": "curl https://x.test/1\ncurl https://x.test/2"}, &many)
	if len(h.s.ws.Collections) != before+1 || !strings.Contains(many.Summary, "2 curl commands") {
		t.Fatalf("multi curl: %q", many.Summary)
	}

	var exp struct {
		Filename string `json:"filename"`
		Data     string `json:"data"`
	}
	h.json("POST", "/api/export", map[string]any{"folders": []int{0}}, &exp)
	if !strings.HasSuffix(exp.Filename, ".postman_collection.json") || !strings.Contains(exp.Data, "schema.getpostman.com") {
		t.Fatalf("export: %s", exp.Filename)
	}

	for i := 0; i < 3; i++ {
		h.sendLines(sendRequest{Request: r})
	}
	var hist struct {
		History []json.RawMessage `json:"history"`
	}
	h.json("DELETE", "/api/history/1", nil, &hist)
	if len(hist.History) != 2 || len(h.s.hist.Entries) != 2 {
		t.Fatalf("delete one: %d", len(hist.History))
	}
	if !strings.Contains(string(hist.History[0]), `"bodyText"`) || strings.Contains(string(hist.History[0]), `"body":"`) {
		t.Fatalf("history bodies should be text: %s", hist.History[0])
	}
	h.json("DELETE", "/api/history", nil, &hist)
	if len(h.s.hist.Entries) != 0 {
		t.Fatal("clear")
	}
}

func TestRunPortFallbackAndAddresses(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan Addresses, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, storage.SampleWorkspace(), nil, Options{Port: port, Ready: func(a Addresses) { got <- a }})
	}()
	var a Addresses
	select {
	case a = <-got:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("not ready")
	}
	if a.Port == port || !strings.Contains(a.Fallback, strconv.Itoa(port)) || len(a.Network) != 0 {
		t.Fatalf("fallback: %+v", a)
	}
	if !strings.HasPrefix(a.Local[0], "http://localhost:"+strconv.Itoa(a.Port)+"/?t=") || !strings.HasPrefix(a.Open(), "http://localhost:") {
		t.Fatalf("local: %v", a.Local)
	}
	// Both local names work.
	for _, u := range a.Local {
		c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, err := c.Get(u)
		if err != nil || res.StatusCode != http.StatusSeeOther {
			t.Fatalf("%s: %v", u, err)
		}
	}
	cancel()
	if err := <-done; err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
	if err := Run(context.Background(), nil, nil, Options{Port: 70000}); err == nil {
		t.Fatal("bad port should fail")
	}
}

func TestLoadAPI(t *testing.T) {
	h := start(t)
	step := func(url string) []map[string]any {
		r := models.NewRequest("Get")
		r.URL, r.Tests = url, "status == 200"
		return []map[string]any{{"name": "API/Get", "request": r}}
	}
	lines := func(body any) []map[string]any {
		b, _ := json.Marshal(body)
		res := h.do("POST", "/api/load", string(b), nil)
		defer res.Body.Close()
		var out []map[string]any
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(nil, 1<<24)
		for sc.Scan() {
			var m map[string]any
			_ = json.Unmarshal(sc.Bytes(), &m)
			out = append(out, m)
		}
		return out
	}

	got := lines(map[string]any{"form": map[string]string{"load": "2", "duration": "1500ms", "thresholds": "p95<5s"}, "steps": step("{{baseUrl}}/get")})
	kinds := []string{}
	for _, l := range got {
		kinds = append(kinds, l["kind"].(string))
	}
	if len(got) < 3 || kinds[0] != "start" || kinds[1] != "tick" || kinds[len(kinds)-1] != "done" {
		t.Fatalf("kinds %v", kinds)
	}
	if got[0]["plan"] != "2 virtual users for 1.5s" {
		t.Fatalf("start %v", got[0])
	}
	done := got[len(got)-1]
	sum := done["summary"].(map[string]any)
	if sum["requests"].(float64) == 0 || sum["latency"].(map[string]any)["p95Ms"].(float64) <= 0 || !strings.Contains(done["text"].(string), "✓ p95<5s") {
		t.Fatalf("done %v", done)
	}

	// Bad settings and WebSocket-only selections are refused.
	if got := lines(map[string]any{"form": map[string]string{"load": "lots", "duration": "1s"}, "steps": step("{{baseUrl}}/get")}); got[0]["kind"] != "error" {
		t.Fatalf("bad form %v", got)
	}
	ws := models.NewRequest("Socket")
	ws.Type, ws.URL = models.TypeWebSocket, "ws://x"
	if got := lines(map[string]any{"form": map[string]string{"load": "1", "duration": "1s"}, "steps": []map[string]any{{"name": "S", "request": ws}}}); got[0]["kind"] != "error" {
		t.Fatalf("websocket %v", got)
	}

	// Another host: the page must ask first.
	got = lines(map[string]any{"form": map[string]string{"load": "1", "duration": "1s"}, "steps": step("https://api.example.com/x")})
	if len(got) != 1 || got[0]["kind"] != "confirm" || got[0]["hosts"].([]any)[0] != "api.example.com" {
		t.Fatalf("confirm %v", got)
	}

	// Stop ends it early with a summary.
	go func() {
		time.Sleep(400 * time.Millisecond)
		h.json("POST", "/api/stop", map[string]string{"id": "lt1"}, nil)
	}()
	start := time.Now()
	got = lines(map[string]any{"id": "lt1", "form": map[string]string{"load": "2", "duration": "1m"}, "steps": step("{{baseUrl}}/get")})
	last := got[len(got)-1]
	if last["kind"] != "done" || last["summary"].(map[string]any)["stopped"] != true || time.Since(start) > 5*time.Second {
		t.Fatalf("stop: %v after %v", last["kind"], time.Since(start))
	}
}
