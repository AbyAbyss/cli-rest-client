package curl

import (
	"io"
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
)

// TestTemplateRoundTrip: copying a request as a template curl and pasting
// it back gives a request that sends exactly the same thing.
func TestTemplateRoundTrip(t *testing.T) {
	ws := storage.SampleWorkspace()
	vars := ws.VariableMap()
	vars["orderId"] = "o-1"
	checked := 0
	ws.WalkRequests(func(_ []*models.Collection, r *models.Request) {
		cmd := Template(*r)
		if strings.Contains(cmd.Text, "{{$") {
			return // dynamic values differ on every run
		}
		if !strings.Contains(cmd.Text, "{{") {
			t.Errorf("%s: template should keep variables:\n%s", r.Name, cmd.Text)
		}
		back, err := Parse(cmd.Text)
		if err != nil {
			t.Fatalf("%s: %v\n%s", r.Name, err, cmd.Text)
		}
		p1, err1 := engine.Prepare(*r, vars)
		p2, err2 := engine.Prepare(back.Request, vars)
		if err1 != nil || err2 != nil {
			t.Fatalf("%s: %v %v", r.Name, err1, err2)
		}
		if p1.Request.Method != p2.Request.Method || p1.Request.URL.String() != p2.Request.URL.String() {
			t.Errorf("%s: %s %s vs %s %s", r.Name, p1.Request.Method, p1.Request.URL, p2.Request.Method, p2.Request.URL)
		}
		for k := range p1.Request.Header {
			if p1.Request.Header.Get(k) != p2.Request.Header.Get(k) {
				t.Errorf("%s: header %s %q vs %q", r.Name, k, p1.Request.Header.Get(k), p2.Request.Header.Get(k))
			}
		}
		b1, _ := io.ReadAll(p1.Request.Body)
		b2, _ := io.ReadAll(p2.Request.Body)
		if string(b1) != string(b2) {
			t.Errorf("%s: body %q vs %q", r.Name, b1, b2)
		}
		checked++
	})
	if checked < 6 {
		t.Fatalf("only %d requests checked", checked)
	}
}

func TestResolvedRunsPreRequest(t *testing.T) {
	ws := storage.SampleWorkspace()
	var charge *models.Request
	ws.WalkRequests(func(_ []*models.Collection, r *models.Request) {
		if r.Name == "Charge" {
			charge = r
		}
	})
	vars := ws.VariableMap()
	cmd, err := Resolved(*charge, vars)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd.Text, `"orderId": "order-`) || len(cmd.Missing) != 0 {
		t.Fatalf("pre-request not applied: %v\n%s", cmd.Missing, cmd.Text)
	}
	if _, ok := vars["orderId"]; ok {
		t.Fatal("Resolved must not change the caller's variables")
	}
	if !strings.HasPrefix(cmd.Text, "curl -X POST 'https://httpbin.org/post'") {
		t.Fatalf("command: %s", cmd.Text)
	}
}

func TestHeadUsesDashI(t *testing.T) {
	r := models.NewRequest("h")
	r.Method = "HEAD"
	r.URL = "https://x.test/"
	cmd, err := Resolved(r, nil)
	if err != nil || !strings.HasPrefix(cmd.Text, "curl -I ") || strings.Contains(cmd.Text, "-X HEAD") {
		t.Fatalf("resolved HEAD: %v %s", err, cmd.Text)
	}
	if !strings.HasPrefix(Template(r).Text, "curl -I ") {
		t.Fatal("template HEAD")
	}
}

func TestTemplateDetails(t *testing.T) {
	r := models.NewRequest("x")
	r.Method = "POST"
	r.URL = "{{baseUrl}}/items?fixed=1"
	r.Params = []models.KeyValue{{Key: "q", Value: "a b {{term}}"}, {Key: "off", Value: "1", Disabled: true}}
	r.Auth = models.Auth{Type: models.AuthBasic, Username: "{{user}}", Password: "it's"}
	r.BodyType = models.BodyForm
	r.Body = "name=Aby Abyss\n# skip=1"
	got := Template(r).Text
	for _, want := range []string{
		`curl -X POST '{{baseUrl}}/items?fixed=1&q=a+b+{{term}}'`,
		`-u '{{user}}:it'\''s'`,
		`-H 'Content-Type: application/x-www-form-urlencoded'`,
		`--data-raw 'name=Aby+Abyss'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "off=") || strings.Contains(got, "skip") {
		t.Errorf("disabled entries leaked:\n%s", got)
	}
}
