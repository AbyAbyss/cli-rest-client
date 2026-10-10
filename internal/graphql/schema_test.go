package graphql

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func post(t *testing.T, url, query, variables string) []byte {
	t.Helper()
	body := map[string]any{"query": query}
	if variables != "" {
		body["variables"] = json.RawMessage(variables)
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return out
}

func TestIntrospectAndSkeleton(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	url := srv.URL + "/graphql"

	s, err := Parse(post(t, url, IntrospectionQuery, ""))
	if err != nil {
		t.Fatal(err)
	}
	if roots := s.Roots(); len(roots) != 1 || roots[0] != [2]string{"query", "Query"} {
		t.Fatalf("roots %v", roots)
	}
	if got := strings.Join(s.TypeNames(), " "); got != "Boolean Continent Country ID Query String" {
		t.Fatalf("types %q", got)
	}
	q := s.Types["Query"]
	if q.Fields[0].Type.String() != "[Country!]!" || q.Fields[1].Args[0].Type.String() != "ID!" {
		t.Fatalf("type strings: %s %s", q.Fields[0].Type, q.Fields[1].Args[0].Type)
	}

	query, vars := s.Skeleton("query", q.Fields[1])
	wantQuery := `query Country($code: ID!) {
  country(code: $code) {
    code
    name
    capital
    currency
    continent {
      code
      name
    }
  }
}
`
	if query != wantQuery || vars != "{\n  \"code\": \"\"\n}" {
		t.Fatalf("skeleton:\n%s\n%s", query, vars)
	}

	// The skeleton runs against the server once the variable is filled in.
	out := post(t, url, query, `{"code": "JP"}`)
	if !strings.Contains(string(out), `"capital":"Tokyo"`) || !strings.Contains(string(out), `"continent":{"code":"AS","name":"Asia"}`) {
		t.Fatalf("result: %s", out)
	}
	query, vars = s.Skeleton("query", q.Fields[0])
	if !strings.HasPrefix(query, "query Countries($continent: String) {\n  countries(continent: $continent) {") || vars != "{\n  \"continent\": \"\"\n}" {
		t.Fatalf("countries skeleton:\n%s\n%s", query, vars)
	}
}

func TestParseErrors(t *testing.T) {
	for body, want := range map[string]string{
		`not json`: "not a GraphQL result",
		`{"errors":[{"message":"introspection disabled"}]}`: "introspection disabled",
		`{"data":{}}`: "no __schema",
	} {
		if _, err := Parse([]byte(body)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", body, err)
		}
	}
}
