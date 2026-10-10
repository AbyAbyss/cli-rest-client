package codegen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// wsCapture is what a WebSocket server saw from one client.
type wsCapture struct {
	Header   http.Header
	Query    string
	Protocol string
	Message  string
}

func wsCaptureServer(t *testing.T) (string, func() []wsCapture) {
	var mu sync.Mutex
	var got []wsCapture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"chat", "echo"}})
		if err != nil {
			return
		}
		defer c.CloseNow()
		cap := wsCapture{Header: r.Header.Clone(), Query: r.URL.RawQuery, Protocol: c.Subprotocol()}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, data, err := c.Read(ctx); err == nil {
			cap.Message = string(data)
			c.Write(ctx, websocket.MessageText, data)
		}
		mu.Lock()
		got = append(got, cap)
		mu.Unlock()
		c.Close(websocket.StatusNormalClosure, "done") // lets the snippet exit
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), func() []wsCapture {
		mu.Lock()
		defer mu.Unlock()
		return append([]wsCapture(nil), got...)
	}
}

// moduleRoot is where Go snippets run, so they can import coder/websocket
// from this module's dependencies.
func moduleRoot(t *testing.T) string {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

func TestWebSocketSnippets(t *testing.T) {
	if testing.Short() {
		t.Skip("runs external interpreters")
	}
	type runner struct {
		file string
		cmd  func(path string) *exec.Cmd
	}
	rs := map[string]runner{}
	if exec.Command("python3", "-c", "from websockets.sync.client import connect").Run() == nil {
		rs["python"] = runner{"main.py", func(p string) *exec.Cmd { return exec.Command("python3", p) }}
	} else {
		t.Log("python3 with websockets not found; skipping Python")
	}
	if exec.Command("node", "-e", "if (typeof WebSocket !== 'function') process.exit(1)").Run() == nil {
		rs["javascript"] = runner{"main.mjs", func(p string) *exec.Cmd { return exec.Command("node", p) }}
	} else {
		t.Log("node with a global WebSocket (Node 22+) not found; skipping JavaScript")
	}
	rs["go"] = runner{"main.go", func(p string) *exec.Cmd { return exec.Command("go", "run", p) }}
	if _, err := exec.LookPath("websocat"); err == nil {
		rs["curl"] = runner{"main.sh", func(p string) *exec.Cmd { return exec.Command("sh", p) }}
	} else {
		t.Log("websocat not found; skipping the cURL tab's websocat command")
	}

	url, got := wsCaptureServer(t)
	values := map[string]string{"wsUrl": url, "who": "aby", "token": "t 1", "room": "a b&c"}
	reqs := func() []models.Request {
		base := models.NewRequest("")
		base.Type = models.TypeWebSocket
		bearer := base
		bearer.Name, bearer.URL, bearer.Body = "bearer", "{{wsUrl}}/chat", `{"hello":"{{who}}","n":1}`
		bearer.Params = []models.KeyValue{{Key: "room", Value: "{{room}}"}}
		bearer.Auth = models.Auth{Type: models.AuthBearer, Token: "{{token}}"}
		bearer.Headers = []models.KeyValue{{Key: "X-Client", Value: "snippet"}, {Key: "Sec-WebSocket-Protocol", Value: "chat"}}
		basic := base
		basic.Name, basic.URL, basic.Body = "basic", "{{wsUrl}}", "plain text 'quoted' \"double\" `tick` ${x}"
		basic.Auth = models.Auth{Type: models.AuthBasic, Username: "{{who}}", Password: "p:w"}
		key := base
		key.Name, key.URL, key.Body = "apikey", "{{wsUrl}}/k?fixed=1", "{{who}}"
		key.Auth = models.Auth{Type: models.AuthAPIKey, Key: "api_key", Value: "{{token}}", In: "query"}
		return []models.Request{bearer, basic, key}
	}()

	root := moduleRoot(t)
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasSuffix(strings.ToUpper(strings.SplitN(kv, "=", 2)[0]), "_PROXY") {
			env = append(env, kv)
		}
	}
	env = append(env, "NO_PROXY=*")

	for _, r := range reqs {
		p, err := engine.Prepare(r, values)
		if err != nil {
			t.Fatal(err)
		}
		for lang, run := range rs {
			for _, template := range []bool{false, true} {
				name := r.Name + "/" + lang
				if template {
					name += "/template"
				}
				t.Run(name, func(t *testing.T) {
					snip, err := Generate(lang, r, values, template)
					if err != nil {
						t.Fatal(err)
					}
					// Go snippets run inside the module for its dependencies.
					dir, err := os.MkdirTemp(root, ".snippet-")
					if err != nil {
						t.Fatal(err)
					}
					defer os.RemoveAll(dir)
					path := filepath.Join(dir, run.file)
					os.WriteFile(path, []byte(snip.Text), 0o644)
					before := len(got())
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					c := run.cmd(path)
					cmd := exec.CommandContext(ctx, c.Path, c.Args[1:]...)
					cmd.Dir, cmd.Env = root, env
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("%v\n%s\n--- snippet ---\n%s", err, out, snip.Text)
					}
					all := got()
					if len(all) != before+1 {
						t.Fatalf("server saw %d connections\n%s", len(all)-before, snip.Text)
					}
					c2 := all[len(all)-1]
					if c2.Message != p.Message {
						t.Errorf("message %q, want %q\n%s", c2.Message, p.Message, snip.Text)
					}
					// websocat (cURL tab) can't URL-encode variables in a shell.
					skipQuery := lang == "curl" && template && strings.Contains(snip.Text, "URL-encode their values")
					if !skipQuery && c2.Query != p.Request.URL.RawQuery && !sameQuery(c2.Query, p.Request.URL.RawQuery) {
						t.Errorf("query %q, want %q\n%s", c2.Query, p.Request.URL.RawQuery, snip.Text)
					}
					for _, h := range []string{"Authorization", "X-Client"} {
						if c2.Header.Get(h) != p.Request.Header.Get(h) {
							t.Errorf("%s %q, want %q\n%s", h, c2.Header.Get(h), p.Request.Header.Get(h), snip.Text)
						}
					}
					if want := p.Request.Header.Get("Sec-WebSocket-Protocol"); c2.Protocol != want {
						t.Errorf("subprotocol %q, want %q", c2.Protocol, want)
					}
					if !strings.Contains(string(out), p.Message) {
						t.Errorf("the snippet should print the echo; output:\n%s", out)
					}
				})
			}
		}
	}
}

func sameQuery(a, b string) bool {
	pa, _ := urlParseQuery(a)
	pb, _ := urlParseQuery(b)
	return pa == pb
}

func urlParseQuery(q string) (string, error) {
	v, err := url.ParseQuery(q)
	return v.Encode(), err
}
