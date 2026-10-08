package curl

import (
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

func header(r models.Request, name string) string { return headerValue(r.Headers, name) }

func TestChromeCopyAsCurl(t *testing.T) {
	// Chrome DevTools "Copy as cURL (bash)", trimmed.
	cmd := `curl 'https://api.example.com/v1/orders?page=2&status=open%20now' \
  -H 'accept: application/json, text/plain, */*' \
  -H 'authorization: Bearer eyJhbGciOi.payload.sig' \
  -H 'content-type: application/json' \
  -b 'session=abc123; theme=dark' \
  -H 'user-agent: Mozilla/5.0 (Macintosh)' \
  --data-raw $'{"note":"it\'s here","qty":2,"tags":["a","b"]}' \
  --compressed`
	res, err := Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	r := res.Request
	if r.Method != "POST" || r.URL != "https://api.example.com/v1/orders" {
		t.Fatalf("method/url: %s %s", r.Method, r.URL)
	}
	if len(r.Params) != 2 || r.Params[1] != (models.KeyValue{Key: "status", Value: "open now"}) {
		t.Fatalf("params: %+v", r.Params)
	}
	if r.Auth.Type != models.AuthBearer || r.Auth.Token != "eyJhbGciOi.payload.sig" || header(r, "Authorization") != "" {
		t.Fatalf("bearer should move to Auth: %+v", r.Auth)
	}
	if header(r, "Cookie") != "session=abc123; theme=dark" || header(r, "user-agent") != "Mozilla/5.0 (Macintosh)" {
		t.Fatalf("headers: %+v", r.Headers)
	}
	if r.BodyType != models.BodyJSON || r.Body != `{"note":"it's here","qty":2,"tags":["a","b"]}` {
		t.Fatalf("body: %s %q", r.BodyType, r.Body)
	}
	if r.Name != "POST orders" || len(res.Notes) != 0 {
		t.Fatalf("name %q notes %v", r.Name, res.Notes)
	}
}

func TestFirefoxAndPostmanStyles(t *testing.T) {
	// Firefox: -X and double quotes.
	res, err := Parse(`curl "https://example.com/api/items/7" -X PATCH -H "Content-Type: application/json" --data-raw "{\"done\":true}"`)
	if err != nil || res.Request.Method != "PATCH" || res.Request.Body != `{"done":true}` || res.Request.BodyType != models.BodyJSON {
		t.Fatalf("firefox: %v %+v", err, res)
	}
	// Postman's code snippet.
	res, err = Parse(`curl --location --request POST 'https://api.example.com/login' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'username=aby' \
--data-urlencode 'password=p@ss word&1'`)
	if err != nil {
		t.Fatal(err)
	}
	r := res.Request
	if r.BodyType != models.BodyForm || r.Body != "username=aby\npassword=p@ss word&1" {
		t.Fatalf("postman form: %q", r.Body)
	}
}

func TestAPIDocExamples(t *testing.T) {
	cases := []struct {
		cmd   string
		check func(r models.Request) bool
		notes int
	}{
		{`curl https://api.github.com/user -u "octocat:token123"`, func(r models.Request) bool {
			return r.Method == "GET" && r.Auth.Type == models.AuthBasic && r.Auth.Username == "octocat" && r.Auth.Password == "token123"
		}, 0},
		{`curl -sSL -XDELETE https://x.test/a/1`, func(r models.Request) bool { return r.Method == "DELETE" && r.URL == "https://x.test/a/1" }, 0},
		{`curl -d "name=Aby" -d "role=admin" http://localhost:8080/users`, func(r models.Request) bool {
			return r.Method == "POST" && r.BodyType == models.BodyForm && r.Body == "name=Aby\nrole=admin"
		}, 0},
		{`curl -G https://x.test/search --data-urlencode "q=hello world" -d lang=en`, func(r models.Request) bool {
			return r.Method == "GET" && r.Body == "" && len(r.Params) == 2 && r.Params[0].Value == "hello world"
		}, 0},
		{`curl --json '{"a":1}' https://x.test/j`, func(r models.Request) bool {
			return r.Method == "POST" && r.BodyType == models.BodyJSON && header(r, "Content-Type") == "application/json" && header(r, "Accept") == "application/json"
		}, 0},
		{`curl -I https://x.test`, func(r models.Request) bool { return r.Method == "HEAD" && r.URL == "https://x.test" }, 0},
		{`curl -X POST https://x.test/upload -F "title=Hi" -F "file=@photo.jpg"`, func(r models.Request) bool {
			return r.BodyType == models.BodyForm && r.Body == "title=Hi"
		}, 2},
		{`curl -k -o out.json --max-time 5 https://x.test/data -H 'Authorization: Basic ` + base64.StdEncoding.EncodeToString([]byte("u:p")) + `'`, func(r models.Request) bool {
			return r.Auth.Type == models.AuthBasic && r.Auth.Username == "u" && r.Auth.Password == "p" && r.URL == "https://x.test/data"
		}, 1},
		{`curl -d '{"x":1}' https://x.test/no-ct`, func(r models.Request) bool { return r.BodyType == models.BodyJSON }, 1},
		{`curl --url https://x.test/a --header "X-Empty;" -A "agent/1"`, func(r models.Request) bool {
			return len(r.Headers) == 2 && r.Headers[0].Key == "X-Empty" && r.Headers[0].Value == "" && header(r, "User-Agent") == "agent/1"
		}, 0},
		{"$ curl https://x.test/prompt | jq .", func(r models.Request) bool { return r.URL == "https://x.test/prompt" }, 0},
		{`curl https://x.test/a?b={{token}} -H "X-Id: {{$uuid}}"`, func(r models.Request) bool {
			return r.Params[0].Value == "{{token}}" && header(r, "X-Id") == "{{$uuid}}"
		}, 0},
	}
	for _, c := range cases {
		res, err := Parse(c.cmd)
		if err != nil {
			t.Errorf("%s: %v", c.cmd, err)
			continue
		}
		if !c.check(res.Request) {
			t.Errorf("%s: unexpected %+v", c.cmd, res.Request)
		}
		if len(res.Notes) != c.notes {
			t.Errorf("%s: notes %v, want %d", c.cmd, res.Notes, c.notes)
		}
	}
}

func TestMultipleAndErrors(t *testing.T) {
	all, err := ParseAll("curl https://a.test/1\ncurl -X POST https://a.test/2 && curl https://a.test/3; echo done")
	if err != nil || len(all) != 3 || all[1].Request.Method != "POST" {
		t.Fatalf("multiple: %v %d", err, len(all))
	}
	if _, err := Parse("curl https://a.test/1\ncurl https://a.test/2"); err == nil {
		t.Error("Parse should insist on one command")
	}
	for _, bad := range []string{"echo hi", "curl -H 'x: y'", "curl 'unterminated", "curl -X"} {
		if _, err := ParseAll(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
	// A pasted command whose newlines were lost keeps "\ " separators.
	res, err := Parse(`curl 'https://a.test/x' \ -H 'A: 1' \ --data-raw 'k=v'`)
	if err != nil || header(res.Request, "A") != "1" || res.Request.Body != "k=v" {
		t.Fatalf("flattened paste: %v %+v", err, res)
	}
	if !LooksLikeCurl("  curl https://x") || LooksLikeCurl("curly") || LooksLikeCurl("https://x") {
		t.Error("LooksLikeCurl")
	}
}

// TestRoundTripWithCurlExport: the app's own "Copy as cURL" output parses
// back into a request that prepares to the same HTTP request.
func TestRoundTripWithCurlExport(t *testing.T) {
	orig := models.NewRequest("x")
	orig.Method = "PUT"
	orig.URL = "https://api.test/items/5"
	orig.Params = []models.KeyValue{{Key: "q", Value: "a b&c"}, {Key: "n", Value: "1"}}
	orig.Headers = []models.KeyValue{{Key: "X-Trace", Value: "it's \"quoted\""}}
	orig.Auth = models.Auth{Type: models.AuthBearer, Token: "t0k"}
	orig.BodyType = models.BodyJSON
	orig.Body = "{\n  \"name\": \"O'Brien\",\n  \"n\": 1\n}"

	p1, err := engine.Prepare(orig, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Parse(p1.Curl())
	if err != nil {
		t.Fatalf("%v\n%s", err, p1.Curl())
	}
	p2, err := engine.Prepare(res.Request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p1.Request.Method != p2.Request.Method || p1.Request.URL.String() != p2.Request.URL.String() {
		t.Fatalf("line: %s %s vs %s %s", p1.Request.Method, p1.Request.URL, p2.Request.Method, p2.Request.URL)
	}
	for k := range p1.Request.Header {
		if p1.Request.Header.Get(k) != p2.Request.Header.Get(k) {
			t.Errorf("header %s: %q vs %q", k, p1.Request.Header.Get(k), p2.Request.Header.Get(k))
		}
	}
	b1, _ := io.ReadAll(p1.Request.Body)
	b2, _ := io.ReadAll(p2.Request.Body)
	if string(b1) != string(b2) {
		t.Fatalf("body %q vs %q", b1, b2)
	}
	if !strings.Contains(p1.Curl(), "\\\n") {
		t.Fatal("export should be multi-line, so continuations are exercised")
	}
}
