package postman

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func find(t *testing.T, ws *models.Workspace, path string) *models.Request {
	t.Helper()
	var found *models.Request
	ws.WalkRequests(func(p []*models.Collection, r *models.Request) {
		names := []string{}
		for _, c := range p {
			names = append(names, c.Name)
		}
		if strings.Join(append(names, r.Name), "/") == path {
			found = r
		}
	})
	if found == nil {
		t.Fatalf("no request %q", path)
	}
	return found
}

func TestImportCollection(t *testing.T) {
	ws := &models.Workspace{Variables: []models.KeyValue{{Key: "pageSize", Value: "50"}}}
	res, err := Import(load(t, "shop.postman_collection.json"), ws)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "collection" || res.Collection.Name != "Shop API" || res.Requests != 9 || res.Folders != 3 {
		t.Fatalf("summary: %+v", res)
	}
	shop := res.Collection
	if len(shop.Folders) != 1 || shop.Folders[0].Name != "Products" || len(shop.Folders[0].Folders) != 2 {
		t.Fatalf("folder structure: %+v", shop.Folders)
	}

	// Query params come out of the URL, keeping disabled ones; headers too.
	list := find(t, ws, "Shop API/Products/List products")
	if list.URL != "{{baseUrl}}/products" || len(list.Params) != 4 || !list.Params[3].Disabled || list.Params[1].Value != "{{pageSize}}" {
		t.Fatalf("list URL/params: %q %+v", list.URL, list.Params)
	}
	if len(list.Headers) != 2 || !list.Headers[1].Disabled {
		t.Fatalf("headers: %+v", list.Headers)
	}
	// Collection-level bearer auth is inherited.
	if list.Auth.Type != models.AuthBearer || list.Auth.Token != "{{token}}" {
		t.Fatalf("inherited auth: %+v", list.Auth)
	}
	// Common test patterns are converted; the original is kept as comments.
	for _, want := range []string{"status == 200", "time < 800", "set firstId = json.items[0].id", "header Content-Type exists", "#     pm.response.to.have.status(200);"} {
		if !strings.Contains(list.Tests, want) {
			t.Errorf("tests missing %q:\n%s", want, list.Tests)
		}
	}

	// Path variables: value substituted, or {{name}} when empty.
	get := find(t, ws, "Shop API/Products/Get product")
	if get.URL != "{{baseUrl}}/products/42/reviews/{{reviewId}}" {
		t.Fatalf("path vars: %q", get.URL)
	}

	// Folder auth overrides collection auth; noauth turns it off.
	create := find(t, ws, "Shop API/Products/Admin/Create product")
	if create.Auth.Type != models.AuthBasic || create.Auth.Username != "admin" || create.Auth.Password != "{{adminPassword}}" {
		t.Fatalf("folder auth: %+v", create.Auth)
	}
	if create.BodyType != models.BodyJSON || !strings.Contains(create.Body, `"sku": "{{sku}}"`) || create.Method != "POST" {
		t.Fatalf("raw json body: %s %q", create.BodyType, create.Body)
	}
	if create.Headers[0].Value != "{{$uuid}}" {
		t.Fatalf("$guid should become $uuid: %q", create.Headers[0].Value)
	}
	if !strings.Contains(create.PreRequest, "set sku = SKU-{{$randomInt}}") || !strings.Contains(create.PreRequest, "set requestId = {{$uuid}}") {
		t.Fatalf("pre-request: %s", create.PreRequest)
	}
	if del := find(t, ws, "Shop API/Products/Admin/Delete product"); del.Auth.Type != models.AuthNone {
		t.Fatalf("noauth: %+v", del.Auth)
	}

	// Bodies.
	login := find(t, ws, "Shop API/Login (form)")
	if login.BodyType != models.BodyForm || login.Body != "username=aby\npassword=secret\n# remember=1" || login.Auth.Type != models.AuthBearer {
		t.Fatalf("urlencoded: %q %+v", login.Body, login.Auth)
	}
	upload := find(t, ws, "Shop API/Upload avatar")
	if upload.BodyType != models.BodyForm || upload.Body != "caption=me" {
		t.Fatalf("formdata: %q", upload.Body)
	}
	gql := find(t, ws, "Shop API/GraphQL search")
	if gql.URL != "https://shop.example.com/graphql" || gql.BodyType != models.BodyGraphQL ||
		gql.Body != "query($q: String!) { search(q: $q) { id name } }" || gql.GraphQLVariables != `{"q": "lamp"}` {
		t.Fatalf("graphql: %q %s %q %q", gql.URL, gql.BodyType, gql.Body, gql.GraphQLVariables)
	}
	if gql.Auth.Type != models.AuthAPIKey || gql.Auth.Key != "api_key" || gql.Auth.In != "query" {
		t.Fatalf("apikey: %+v", gql.Auth)
	}
	if o := find(t, ws, "Shop API/OAuth thing"); o.Auth.Type != models.AuthNone {
		t.Fatal("unsupported auth should become none")
	}
	if p := find(t, ws, "Shop API/Plain string request"); p.URL != "https://shop.example.com/health" || p.Method != "GET" {
		t.Fatalf("string request: %+v", p)
	}

	// Collection variables: added to globals unless already there.
	vars := ws.VariableMap()
	if vars["baseUrl"] != "https://shop.example.com/api" || vars["pageSize"] != "50" || res.Variables != 1 {
		t.Fatalf("collection variables: %v (added %d)", vars, res.Variables)
	}

	// Warnings explain what didn't carry over.
	all := strings.Join(res.Warnings, "\n")
	for _, want := range []string{"Collection-level scripts", ":reviewId became {{reviewId}}", "file field \"file\" was skipped", "multipart form-data", "oauth2 auth is not supported", "pageSize"} {
		if !strings.Contains(all, want) {
			t.Errorf("warnings missing %q:\n%s", want, all)
		}
	}

	// Importing again doesn't clobber the first copy.
	res2, err := Import(load(t, "shop.postman_collection.json"), ws)
	if err != nil || res2.Collection.Name != "Shop API 2" {
		t.Fatalf("second import: %v %+v", err, res2)
	}
}

// TestImportedRequestsWork prepares imported requests and runs their
// converted scripts, so the conversion isn't just cosmetic.
func TestImportedRequestsWork(t *testing.T) {
	ws := &models.Workspace{}
	if _, err := Import(load(t, "shop.postman_collection.json"), ws); err != nil {
		t.Fatal(err)
	}
	ws.SetGlobal("token", "tok")
	ws.SetGlobal("adminPassword", "pw")
	ws.SetGlobal("reviewId", "7")

	vars := ws.VariableMap()
	create := find(t, ws, "Shop API/Products/Admin/Create product")
	if _, errs := script.RunPre(create.PreRequest, vars); len(errs) > 0 {
		t.Fatalf("converted pre-request script should run: %v", errs)
	}
	p, err := engine.Prepare(*create, vars)
	if err != nil {
		t.Fatal(err)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:pw"))
	if p.Request.Header.Get("Authorization") != wantAuth || !strings.Contains(string(p.Body), `"sku": "SKU-`) || len(p.Missing) != 0 {
		t.Fatalf("prepared: %v %s missing=%v", p.Request.Header, p.Body, p.Missing)
	}

	list := find(t, ws, "Shop API/Products/List products")
	p, _ = engine.Prepare(*list, vars)
	if p.Request.URL.String() != "https://shop.example.com/api/products?page=1&size=20&sort=name" || p.Request.Header.Get("Authorization") != "Bearer tok" {
		t.Fatalf("list prepared: %s %v", p.Request.URL, p.Request.Header)
	}
	results := script.RunTests(list.Tests, script.Response{
		Status: 200, Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body: []byte(`{"items":[{"id":"p1"}]}`),
	}, vars)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("converted test failed: %s (%s)", r.Source, r.Message)
		}
	}
	if vars["firstId"] != "p1" {
		t.Fatal("converted capture didn't run")
	}
}

func TestImportEnvironmentAndGlobals(t *testing.T) {
	ws := &models.Workspace{Variables: []models.KeyValue{{Key: "token", Value: "mine"}}}
	res, err := Import(load(t, "staging.postman_environment.json"), ws)
	if err != nil {
		t.Fatal(err)
	}
	e := ws.Environment("Staging")
	if res.Kind != "environment" || e == nil || len(e.Variables) != 3 || !e.Variables[2].Disabled || e.Variables[1].Value != "staging-token" {
		t.Fatalf("environment: %+v %+v", res, e)
	}
	if ws.ActiveEnvironment != "" {
		t.Fatal("importing must not switch the active environment")
	}
	res, _ = Import(load(t, "staging.postman_environment.json"), ws)
	if res.Environment.Name != "Staging 2" {
		t.Fatalf("duplicate env name: %q", res.Environment.Name)
	}

	res, err = Import(load(t, "workspace.postman_globals.json"), ws)
	if err != nil || res.Kind != "globals" || res.Variables != 1 {
		t.Fatalf("globals: %v %+v", err, res)
	}
	if ws.VariableMap()["token"] != "mine" || ws.VariableMap()["adminPassword"] != "hunter2" {
		t.Fatalf("globals merge: %v", ws.VariableMap())
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), `Global "token" kept its current value`) {
		t.Fatalf("warnings: %v", res.Warnings)
	}
}

func TestImportRejects(t *testing.T) {
	cases := map[string]string{
		"{bad json":    "not a JSON file",
		`{"hello": 1}`: "not a Postman collection or environment",
		`{"id":"x","name":"old","requests":[],"order":[]}`:                                                                "Collection v1",
		`{"info":{"name":"x","schema":"https://schema.getpostman.com/json/collection/v1.0.0/collection.json"},"item":[]}`: "unsupported collection schema",
	}
	for in, want := range cases {
		_, err := Import([]byte(in), &models.Workspace{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", in, err, want)
		}
	}
}

func TestImportV20AuthObjects(t *testing.T) {
	// Collection v2.0 stores auth parameters as objects, not key/value lists.
	data := `{"info":{"name":"Old","schema":"https://schema.getpostman.com/json/collection/v2.0.0/collection.json"},
	"item":[{"name":"r","request":{"method":"GET","url":"http://x/a?b=1",
	"auth":{"type":"bearer","bearer":{"token":"abc"}}}}]}`
	ws := &models.Workspace{}
	if _, err := Import([]byte(data), ws); err != nil {
		t.Fatal(err)
	}
	r := find(t, ws, "Old/r")
	if r.Auth.Token != "abc" || r.URL != "http://x/a" || len(r.Params) != 1 || r.Params[0].Value != "1" {
		t.Fatalf("v2.0: %+v", r)
	}
}
