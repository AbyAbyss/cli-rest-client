package codegen

import (
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

func TestLookup(t *testing.T) {
	for in, want := range map[string]string{
		"python": "python", "Py": "python", "js": "javascript", "JavaScript": "javascript",
		"fetch": "javascript", "golang": "go", "Go": "go", "http": "httpie", "HTTPie": "httpie", "curl": "curl",
	} {
		if l, ok := Lookup(in); !ok || l.ID != want {
			t.Errorf("Lookup(%q) = %v %v, want %s", in, l, ok, want)
		}
	}
	if _, ok := Lookup("cobol"); ok {
		t.Error("cobol should not be found")
	}
	if _, err := Generate("cobol", models.NewRequest("x"), nil, false); err == nil || !strings.Contains(err.Error(), "python") {
		t.Errorf("error should list the languages: %v", err)
	}
}

func TestVariableNames(t *testing.T) {
	vs := []Variable{{Name: "baseUrl"}, {Name: "api_key"}, {Name: "$uuid"}, {Name: "url"}, {Name: "base-url"}, {Name: "2fa"}, {Name: "HTTPToken"}}
	cases := map[string]naming{
		"base_url api_key uuid url_var base_url2 v_2fa httptoken": pyNames,
		"baseUrl apiKey uuid urlVar baseUrl2 v2fa httptoken":      jsNames,
		"BASE_URL API_KEY UUID URL BASE_URL2 V_2FA HTTPTOKEN":     shNames,
	}
	for want, n := range cases {
		ids := n.names(vs)
		var got []string
		for _, v := range vs {
			got = append(got, ids[v.Name])
		}
		if strings.Join(got, " ") != want {
			t.Errorf("got %q, want %q", strings.Join(got, " "), want)
		}
	}
}

func TestPythonSnippet(t *testing.T) {
	r := models.NewRequest("Charge")
	r.Method, r.URL, r.BodyType = "POST", "{{baseUrl}}/post", models.BodyJSON
	r.Body = `{"amount": 200, "orderId": "{{orderId}}", "live": {{live}}}`
	r.Params = []models.KeyValue{{Key: "v", Value: "2"}}
	r.Auth = models.Auth{Type: models.AuthBearer, Token: "{{token}}"}
	values := map[string]string{"baseUrl": "https://api.test", "orderId": "o-1", "token": "t", "live": "false"}

	s, err := Generate("python", r, values, false)
	if err != nil {
		t.Fatal(err)
	}
	want := `import requests

url = "https://api.test/post"
params = {
    "v": "2",
}
headers = {
    "Content-Type": "application/json",
    "Authorization": "Bearer t",
}
payload = {
    "amount": 200,
    "orderId": "o-1",
    "live": False,
}

response = requests.post(url, params=params, headers=headers, json=payload)
print(response.status_code)
print(response.text)
`
	if s.Text != want {
		t.Errorf("resolved:\n%s\nwant:\n%s", s.Text, want)
	}

	s, err = Generate("python", r, values, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{
		"base_url = \"https://api.test\"\ntoken = \"t\"\norder_id = \"o-1\"\nlive = False\n",
		`url = f"{base_url}/post"`,
		`"Authorization": f"Bearer {token}",`,
		`"orderId": order_id,`,
		`"live": live,`,
	} {
		if !strings.Contains(s.Text, part) {
			t.Errorf("template is missing %q:\n%s", part, s.Text)
		}
	}
}

func TestVariableUsedBareAndInText(t *testing.T) {
	r := models.NewRequest("x")
	r.Method, r.URL, r.BodyType = "POST", "https://api.test/{{live}}", models.BodyJSON
	r.Body = `{"live": {{live}}}`
	s, err := Generate("python", r, map[string]string{"live": "false"}, true)
	if err != nil {
		t.Fatal(err)
	}
	// Declared as a string, so the URL reads "false", not "False".
	if !strings.Contains(s.Text, `live = "false"`) || !strings.Contains(s.Text, `"live": json.loads(live),`) || !strings.HasPrefix(s.Text, "import json\n") {
		t.Errorf("got:\n%s", s.Text)
	}
}

func TestMissingVariables(t *testing.T) {
	r := models.NewRequest("x")
	r.URL = "https://api.test/{{tenant}}/items"
	for _, template := range []bool{false, true} {
		s, err := Generate("javascript", r, nil, template)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Missing) != 1 || s.Missing[0] != "tenant" {
			t.Errorf("template=%v: Missing = %v", template, s.Missing)
		}
		if template && !strings.Contains(s.Text, `const tenant = "";`) {
			t.Errorf("a missing variable should be declared empty:\n%s", s.Text)
		}
		if !template && !strings.Contains(s.Text, "{{tenant}}") {
			t.Errorf("a missing variable stays as {{name}}:\n%s", s.Text)
		}
	}
}
