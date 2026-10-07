package engine

import (
	"encoding/base64"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

func newReq() models.Request {
	r := models.NewRequest("t")
	r.URL = "{{base}}/items?existing=1"
	return r
}

func TestPrepareQueryHeadersAndMissing(t *testing.T) {
	r := newReq()
	r.Params = []models.KeyValue{{Key: "q", Value: "a b&c"}, {Key: "off", Value: "x", Disabled: true}, {Key: "id", Value: "{{id}}"}}
	r.Headers = []models.KeyValue{{Key: "X-Trace", Value: "{{trace}}"}}
	p, err := Prepare(r, map[string]string{"base": "https://api.test", "id": "9"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Request.URL.String(); got != "https://api.test/items?existing=1&q=a+b%26c&id=9" {
		t.Fatalf("url = %s", got)
	}
	if got := p.Request.Header.Get("X-Trace"); got != "{{trace}}" {
		t.Fatalf("header = %q", got)
	}
	if !reflect.DeepEqual(p.Missing, []string{"trace"}) {
		t.Fatalf("missing = %v", p.Missing)
	}
}

func TestPrepareDefaultsScheme(t *testing.T) {
	r := models.NewRequest("t")
	r.URL = "localhost:8080/health"
	p, err := Prepare(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Request.URL.String() != "http://localhost:8080/health" {
		t.Fatalf("got %s", p.Request.URL)
	}
}

func TestPrepareErrors(t *testing.T) {
	for _, u := range []string{"", "   ", "ftp://x", "http://"} {
		r := models.NewRequest("t")
		r.URL = u
		if _, err := Prepare(r, nil); err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
	r := models.NewRequest("t")
	r.URL = "http://x"
	r.Headers = []models.KeyValue{{Key: "Bad Header", Value: "v"}}
	if _, err := Prepare(r, nil); err == nil {
		t.Errorf("expected header name error")
	}
}

func TestAuth(t *testing.T) {
	r := newReq()
	r.Auth = models.Auth{Type: models.AuthBasic, Username: "u", Password: "{{pw}}"}
	p, _ := Prepare(r, map[string]string{"base": "http://h", "pw": "p"})
	if got := p.Request.Header.Get("Authorization"); got != "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")) {
		t.Fatalf("basic = %q", got)
	}

	r.Auth = models.Auth{Type: models.AuthBearer, Token: "tok"}
	p, _ = Prepare(r, map[string]string{"base": "http://h"})
	if got := p.Request.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("bearer = %q", got)
	}

	r.Auth = models.Auth{Type: models.AuthAPIKey, Key: "X-Api-Key", Value: "k1", In: "header"}
	p, _ = Prepare(r, map[string]string{"base": "http://h"})
	if got := p.Request.Header.Get("X-Api-Key"); got != "k1" {
		t.Fatalf("apikey header = %q", got)
	}

	r.Auth = models.Auth{Type: models.AuthAPIKey, Key: "api_key", Value: "k 2", In: "query"}
	p, _ = Prepare(r, map[string]string{"base": "http://h"})
	if got := p.Request.URL.RawQuery; got != "existing=1&api_key=k+2" {
		t.Fatalf("apikey query = %q", got)
	}
}

func TestBodies(t *testing.T) {
	r := newReq()
	r.Method = "POST"
	r.BodyType = models.BodyJSON
	r.Body = `{"n": {{n}}}`
	p, err := Prepare(r, map[string]string{"base": "http://h", "n": "5"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(p.Request.Body)
	if string(body) != `{"n": 5}` || p.Request.Header.Get("Content-Type") != "application/json" || len(p.Warnings) != 0 {
		t.Fatalf("json body %q ct %q warnings %v", body, p.Request.Header.Get("Content-Type"), p.Warnings)
	}

	r.Body = `{"n": }`
	p, _ = Prepare(r, map[string]string{"base": "http://h"})
	if len(p.Warnings) != 1 {
		t.Fatalf("expected invalid JSON warning")
	}

	r.BodyType = models.BodyForm
	r.Body = "user=a b\n# skipped=1\npass=x&y"
	r.Headers = []models.KeyValue{{Key: "Content-Type", Value: "application/custom"}}
	p, _ = Prepare(r, map[string]string{"base": "http://h"})
	if string(p.Body) != "user=a+b&pass=x%26y" {
		t.Fatalf("form body %q", p.Body)
	}
	if p.Request.Header.Get("Content-Type") != "application/custom" {
		t.Fatalf("explicit Content-Type was overridden")
	}

	r.BodyType = models.BodyNone
	p, _ = Prepare(r, map[string]string{"base": "http://h"})
	if p.Request.ContentLength != 0 || len(p.Body) != 0 {
		t.Fatalf("none body should be empty")
	}
}

func TestCurl(t *testing.T) {
	r := models.NewRequest("t")
	r.Method = "POST"
	r.URL = "http://h/x"
	r.BodyType = models.BodyJSON
	r.Body = `{"it's": 1}`
	p, err := Prepare(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := p.Curl()
	for _, want := range []string{"curl -X POST 'http://h/x'", "-H 'Content-Type: application/json'", `--data-raw '{"it'\''s": 1}'`} {
		if !strings.Contains(c, want) {
			t.Errorf("curl missing %q:\n%s", want, c)
		}
	}
}
