package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func TestGraphQLSchemaBrowserAndSend(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)

	r := models.NewRequest("Countries")
	r.URL = srv.URL + "/graphql"
	h.do(func() {
		h.a.loadIntoBuilder(r, nil)
		h.a.tv.SetFocus(h.a.urlInput)
	})

	// Ctrl+T (F6 elsewhere) fetches the schema and lists the root fields with their types.
	h.key(tcell.KeyCtrlT, 0, tcell.ModCtrl)
	h.eventually("schema dialog", func() bool { return len(h.a.dialogs) == 1 })
	s := h.screenText()
	for _, want := range []string{"GraphQL schema", "countries(continent: String): [Country!]!", "country(code: ID!): Country", "A country by its ISO"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	// → shows the fields of Country under "country".
	h.key(tcell.KeyDown, 0, 0)
	h.key(tcell.KeyRight, 0, 0)
	if s := h.screenText(); !strings.Contains(s, "capital: String") || !strings.Contains(s, "continent: Continent!") {
		t.Fatalf("expanded:\n%s", s)
	}
	// Enter writes a query for it.
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.dialogs) != 0 || h.a.req.BodyType != models.BodyGraphQL || h.a.req.Method != "POST" {
			t.Fatalf("dialogs %d, body %s, method %s", len(h.a.dialogs), h.a.req.BodyType, h.a.req.Method)
		}
		if !strings.HasPrefix(h.a.req.Body, "query Country($code: ID!) {\n  country(code: $code) {") || h.a.req.GraphQLVariables != "{\n  \"code\": \"\"\n}" {
			t.Fatalf("query %q vars %q", h.a.req.Body, h.a.req.GraphQLVariables)
		}
		if h.a.tv.GetFocus() != h.a.bodyArea || !strings.Contains(h.a.status, "Wrote a query for country") {
			t.Fatalf("focus or status: %q", h.a.status)
		}
		h.a.gqlVarsArea.SetText(`{"code":"{{country}}"}`, false)
		h.a.ws.SetVariable("country", "IN")
		h.a.tv.SetFocus(h.a.urlInput)
	})
	if s := h.screenText(); !strings.Contains(s, "Query · GraphQL") || !strings.Contains(s, "Variables · JSON") {
		t.Fatalf("editors:\n%s", s)
	}

	// Ctrl+P formats the variables.
	h.key(tcell.KeyCtrlP, 0, tcell.ModCtrl)
	h.do(func() {
		if h.a.req.GraphQLVariables != "{\n  \"code\": \"{{country}}\"\n}" {
			t.Fatalf("formatted %q", h.a.req.GraphQLVariables)
		}
	})

	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("response", func() bool { return !h.a.sending && h.a.result != nil && h.a.result.resp != nil })
	h.do(func() {
		body := string(h.a.result.resp.Body)
		if !strings.Contains(body, `"capital":"New Delhi"`) {
			t.Fatalf("body %s", body)
		}
	})

	// Switching away from GraphQL hides the variables editor.
	h.do(func() {
		h.a.bodyType.SetCurrentOption(indexOf(models.BodyTypes, models.BodyJSON))
		if h.a.bodyEditors.GetItemCount() != 1 {
			t.Fatal("variables editor should be hidden for JSON")
		}
	})
}

func TestGraphQLSchemaErrors(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)
	r := models.NewRequest("Not GraphQL")
	r.URL = srv.URL + "/json"
	h.do(func() { h.a.loadIntoBuilder(r, nil) })
	h.key(tcell.KeyF6, 0, 0)
	h.eventually("error", func() bool { return strings.Contains(h.a.status, "Cannot fetch the schema") })
	h.do(func() {
		if len(h.a.dialogs) != 0 || !strings.Contains(h.a.status, "no __schema") {
			t.Fatalf("status %q", h.a.status)
		}
	})
}

func TestGraphQLEditorsAreTypable(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)
	r := models.NewRequest("Typed")
	r.URL = srv.URL + "/graphql"
	h.do(func() {
		h.a.loadIntoBuilder(r, nil)
		h.a.switchTab(3)
		h.a.bodyType.SetCurrentOption(indexOf(models.BodyTypes, models.BodyGraphQL))
	})
	// The empty editors show GraphQL examples, not a JSON body.
	if s := h.screenText(); !strings.Contains(s, "Type a GraphQL query") || !strings.Contains(s, "Optional: a JSON") {
		t.Fatalf("examples:\n%s", s)
	}

	// Sending with an empty query explains what to do, with the environment.
	h.do(func() { h.a.tv.SetFocus(h.a.urlInput) })
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.do(func() {
		res := h.a.result
		if res == nil || res.err == nil || !strings.Contains(res.err.Error(), "press Ctrl+T") || res.env != "httpbin.org" {
			t.Fatalf("empty query: %+v", res)
		}
	})

	// Both editors take typing; Tab moves from the query to the variables.
	h.do(func() { h.a.tv.SetFocus(h.a.bodyArea) })
	h.typeText(`query($code: ID!) { country(code: $code) { capital } }`)
	h.key(tcell.KeyTab, 0, 0)
	h.do(func() {
		if h.a.tv.GetFocus() != h.a.gqlVarsArea {
			t.Fatalf("Tab should reach the variables editor, focus is %T", h.a.tv.GetFocus())
		}
	})
	h.typeText(`{"code":"JP"}`)
	h.do(func() {
		if !strings.HasPrefix(h.a.req.Body, "query($code: ID!)") || h.a.req.GraphQLVariables != `{"code":"JP"}` {
			t.Fatalf("typed: %q %q", h.a.req.Body, h.a.req.GraphQLVariables)
		}
		h.a.tv.SetFocus(h.a.urlInput)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("answer", func() bool {
		res := h.a.result
		return !h.a.sending && res != nil && res.resp != nil && strings.Contains(string(res.resp.Body), `"capital":"Tokyo"`)
	})
}
