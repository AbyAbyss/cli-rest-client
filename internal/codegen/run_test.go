package codegen

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
)

// captured is what a server saw.
type captured struct {
	Method string
	URI    string
	Header http.Header
	Body   []byte
}

func captureServer(t *testing.T) (*httptest.Server, func() []captured) {
	var mu sync.Mutex
	var got []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		h := r.Header.Clone()
		h.Set("Host", r.Host)
		mu.Lock()
		got = append(got, captured{r.Method, r.RequestURI, h, body})
		mu.Unlock()
		w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []captured {
		mu.Lock()
		defer mu.Unlock()
		return append([]captured(nil), got...)
	}
}

// fixedValues makes dynamic variables predictable, so the snippet and the
// reference request send the same thing.
func fixedValues(base string) map[string]string {
	return map[string]string{
		"baseUrl":    base,
		"graphqlUrl": base + "/graphql",
		"sseUrl":     base + "/sse",
		"country":    "IN",
		"token":      "tok en/1",
		"$uuid":      "3b1f8c2e-0000-4000-8000-000000000001",
		"$timestamp": "1700000000",
		"$randomInt": "42",
		"user":       "aby",
		"count":      "3",
		"flag":       "true",
	}
}

// testRequests is every sample request plus cases that exercise the
// awkward parts of each generator.
func testRequests() []models.Request {
	var out []models.Request
	storage.SampleWorkspace().WalkRequests(func(_ []*models.Collection, r *models.Request) {
		out = append(out, *r)
	})
	req := func(name, method, u, bodyType, body string) models.Request {
		r := models.NewRequest(name)
		r.Method, r.URL, r.BodyType, r.Body = method, u, bodyType, body
		return r
	}

	nested := req("Nested JSON", "POST", "{{baseUrl}}/orders?src=app&n={{count}}", models.BodyJSON,
		`{"user": {"name": "{{user}} \"quoted\" {braces} ${x} `+"`tick`"+` é", "tags": ["a", "b"], "n": {{count}}},
 "items": [{"sku": "A-1", "qty": 2.5}, {"sku": null}], "ok": {{flag}}, "empty": {}, "none": []}`)
	nested.Headers = []models.KeyValue{{Key: "X-Trace", Value: "{{$uuid}}"}, {Key: "Accept", Value: "application/json"}}
	out = append(out, nested)

	arr := req("Array body", "PUT", "{{baseUrl}}/bulk", models.BodyJSON, `[1, "two", {"three": 3}]`)
	out = append(out, arr)

	text := req("Text", "PATCH", "{{baseUrl}}/notes/1", models.BodyText, "line one\nline 'two' \"three\" $HOME `x`\n")
	out = append(out, text)

	xml := req("XML", "POST", "{{baseUrl}}/xml", models.BodyXML, `<order id="{{count}}"><who>{{user}}</who></order>`)
	xml.Headers = []models.KeyValue{{Key: "Content-Type", Value: "text/xml"}}
	out = append(out, xml)

	form := req("Form", "POST", "{{baseUrl}}/form", models.BodyForm, "tag=a\ntag=b c\nk:ey=v=al&u\nwho={{user}}")
	out = append(out, form)

	qkey := req("API key query", "GET", "{{baseUrl}}/search", models.BodyNone, "")
	qkey.Params = []models.KeyValue{{Key: "q", Value: "a b&c"}, {Key: "who", Value: "{{user}}"}, {Key: "off", Value: "x", Disabled: true}}
	qkey.Auth = models.Auth{Type: models.AuthAPIKey, Key: "api_key", Value: "{{token}}", In: "query"}
	out = append(out, qkey)

	hkey := req("API key header", "DELETE", "{{baseUrl}}/items/{{count}}", models.BodyNone, "")
	hkey.Auth = models.Auth{Type: models.AuthAPIKey, Key: "X-Api-Key", Value: "{{token}}", In: "header"}
	hkey.Headers = []models.KeyValue{{Key: "X-Empty", Value: ""}, {Key: "X-Multi", Value: "1"}, {Key: "X-Multi", Value: "2"}}
	out = append(out, hkey)

	head := req("Head", "HEAD", "{{baseUrl}}/ping", models.BodyNone, "")
	out = append(out, head)

	opts := req("Options", "OPTIONS", "{{baseUrl}}/ping", models.BodyNone, "")
	out = append(out, opts)

	gql := req("GraphQL", "POST", "{{baseUrl}}/graphql", models.BodyGraphQL,
		"query Users($first: Int!, $role: String) {\n  users(first: $first, role: $role, tag: \"{{user}}\") { id name }\n}")
	gql.GraphQLVariables = `{"first": {{count}}, "role": "admin-{{user}}", "nested": {"on": {{flag}}}}`
	out = append(out, gql)

	gqlGet := req("GraphQL GET", "GET", "{{baseUrl}}/graphql", models.BodyGraphQL, "{ me { id } }")
	gqlGet.GraphQLVariables = `{"who": "{{user}}"}`
	out = append(out, gqlGet)

	custom := req("Custom method", "PURGE", "{{baseUrl}}/cache", models.BodyNone, "")
	custom.Auth = models.Auth{Type: models.AuthBasic, Username: "{{user}}", Password: "p:a ss"}
	out = append(out, custom)
	return out
}

type runner struct {
	file string
	cmd  func(path string) *exec.Cmd
}

func runners(t *testing.T) map[string]runner {
	rs := map[string]runner{}
	if exec.Command("python3", "-c", "import requests").Run() == nil {
		rs["python"] = runner{"main.py", func(p string) *exec.Cmd { return exec.Command("python3", p) }}
	} else {
		t.Log("python3 with requests not found; skipping Python")
	}
	if _, err := exec.LookPath("node"); err == nil {
		rs["javascript"] = runner{"main.mjs", func(p string) *exec.Cmd { return exec.Command("node", p) }}
	} else {
		t.Log("node not found; skipping JavaScript")
	}
	if _, err := exec.LookPath("go"); err == nil {
		rs["go"] = runner{"main.go", func(p string) *exec.Cmd { return exec.Command("go", "run", p) }}
	}
	if _, err := exec.LookPath("http"); err == nil {
		rs["httpie"] = runner{"main.sh", func(p string) *exec.Cmd {
			// --ignore-stdin: HTTPie would otherwise read the test's stdin as the body.
			script, _ := os.ReadFile(p)
			s := strings.Replace(string(script), "http ", "http --ignore-stdin ", 1)
			os.WriteFile(p, []byte(s), 0o644)
			return exec.Command("sh", p)
		}}
	} else {
		t.Log("http (HTTPie) not found; skipping HTTPie")
	}
	return rs
}

// TestSnippetsSendTheSameRequest runs every generated snippet and checks
// that the server receives what the app itself would send.
func TestSnippetsSendTheSameRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("runs external interpreters")
	}
	rs := runners(t)
	env := []string{}
	for _, kv := range os.Environ() {
		if k := strings.ToUpper(strings.SplitN(kv, "=", 2)[0]); strings.HasSuffix(k, "_PROXY") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "NO_PROXY=*")

	for _, r := range testRequests() {
		for lang, run := range rs {
			for _, template := range []bool{false, true} {
				r, lang, run, template := r, lang, run, template
				mode := "resolved"
				if template {
					mode = "template"
				}
				t.Run(r.Name+"/"+lang+"/"+mode, func(t *testing.T) {
					t.Parallel()
					srv, got := captureServer(t)
					values := fixedValues(srv.URL)

					// What the app sends.
					v := map[string]string{}
					for k, x := range values {
						v[k] = x
					}
					script.RunPre(r.PreRequest, v)
					p, err := engine.Prepare(r, v)
					if err != nil {
						t.Fatal(err)
					}
					resp, err := http.DefaultClient.Do(p.Request)
					if err != nil {
						t.Fatal(err)
					}
					resp.Body.Close()

					snip, err := Generate(lang, r, values, template)
					if err != nil {
						t.Fatal(err)
					}
					dir := t.TempDir()
					path := filepath.Join(dir, run.file)
					if err := os.WriteFile(path, []byte(snip.Text), 0o644); err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					cmd := run.cmd(path)
					cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
					cmd.Dir, cmd.Env = dir, env
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("%v\n%s\n--- snippet ---\n%s", err, out, snip.Text)
					}

					reqs := got()
					if len(reqs) != 2 {
						t.Fatalf("server saw %d requests\n%s", len(reqs), snip.Text)
					}
					compare(t, reqs[0], reqs[1], snip.Text)
				})
			}
		}
	}
}

// compare checks the parts of a request that matter. Client-specific
// headers (User-Agent, Accept-Encoding, Connection, ...) are ignored.
func compare(t *testing.T, want, got captured, snippet string) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf(format+"\n--- snippet ---\n%s", append(args, snippet)...)
	}
	if want.Method != got.Method {
		fail("method %q, want %q", got.Method, want.Method)
	}
	wu, _ := url.Parse(want.URI)
	gu, _ := url.Parse(got.URI)
	if wu.Path != gu.Path || !reflect.DeepEqual(wu.Query(), gu.Query()) {
		fail("URI %q, want %q", got.URI, want.URI)
	}
	skip := map[string]bool{"User-Agent": true, "Accept-Encoding": true, "Connection": true, "Content-Length": true,
		"Accept": true, "Accept-Language": true, "Sec-Fetch-Mode": true}
	for k, vs := range want.Header {
		if skip[k] {
			continue
		}
		g := got.Header.Values(k)
		if k == "Content-Type" {
			wm, _, _ := mime.ParseMediaType(vs[0])
			gm, _, _ := mime.ParseMediaType(strings.Join(g, ""))
			if wm != gm {
				fail("Content-Type %q, want %q", g, vs)
			}
			continue
		}
		if strings.Join(g, ", ") != strings.Join(vs, ", ") {
			fail("header %s = %q, want %q", k, g, vs)
		}
	}
	if accept := want.Header.Get("Accept"); accept != "" && got.Header.Get("Accept") != accept {
		fail("Accept %q, want %q", got.Header.Get("Accept"), accept)
	}

	ct := want.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "json") && len(want.Body) > 0:
		var w, g any
		if json.Unmarshal(want.Body, &w) != nil || json.Unmarshal(got.Body, &g) != nil || !reflect.DeepEqual(w, g) {
			fail("body %s, want %s", got.Body, want.Body)
		}
	case strings.Contains(ct, "x-www-form-urlencoded"):
		w, _ := url.ParseQuery(string(want.Body))
		g, _ := url.ParseQuery(string(got.Body))
		if !reflect.DeepEqual(w, g) {
			fail("form %s, want %s", got.Body, want.Body)
		}
	default:
		if string(want.Body) != string(got.Body) {
			fail("body %q, want %q", got.Body, want.Body)
		}
	}
}
