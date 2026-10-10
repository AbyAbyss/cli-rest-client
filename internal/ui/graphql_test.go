package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

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
	for _, want := range []string{"GraphQL schema", "countries(continent: String): [Country!]!", "country(code: ID!): Country",
		"Details", "All countries, optionally on one continent.", "i writes this query"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	// Moving to "country" shows its arguments, its type's fields and the query.
	h.key(tcell.KeyDown, 0, 0)
	s = h.screenText()
	for _, want := range []string{"A country by its ISO 3166 code.", "Arguments", "code: ID! (required)", "Country fields", "query Country($code: ID!) {"} {
		if !strings.Contains(s, want) {
			t.Fatalf("details missing %q:\n%s", want, s)
		}
	}
	// Enter opens it in the tree; the browser stays open.
	h.key(tcell.KeyEnter, 0, 0)
	if s := h.screenText(); len(h.a.dialogs) != 1 || !strings.Contains(s, "▾ country(") || !strings.Contains(s, "capital: String") {
		t.Fatalf("Enter should open the field:\n%s", s)
	}
	// A mouse click opens a field too, without closing anything.
	x, y := -1, -1
	for row, l := range strings.Split(h.screenText(), "\n") {
		if i := strings.Index(l, "▸ countries("); i >= 0 {
			x, y = len([]rune(l[:i]))+2, row
		}
	}
	if y < 0 {
		t.Fatal("countries row not found")
	}
	h.screen.InjectMouse(x, y, tcell.Button1, 0)
	h.screen.InjectMouse(x, y, tcell.ButtonNone, 0)
	opened := false
	for i := 0; i < 100 && !opened; i++ { // screenText pauses the UI itself, so not inside eventually
		opened = strings.Contains(h.screenText(), "▾ countries(")
		time.Sleep(20 * time.Millisecond)
	}
	if !opened {
		t.Fatalf("a click should open countries:\n%s", h.screenText())
	}
	if len(h.a.dialogs) != 1 {
		t.Fatal("a click must not close the browser")
	}
	// On a nested field, i writes the query for its top-level field.
	h.do(func() {
		for _, n := range h.a.tv.GetFocus().(*tview.TreeView).GetRoot().GetChildren()[0].GetChildren()[1].GetChildren() {
			if ref := n.GetReference().(*schemaRef); ref.field.Name == "capital" {
				h.a.tv.GetFocus().(*tview.TreeView).SetCurrentNode(n)
			}
		}
	})
	if s := h.screenText(); !strings.Contains(s, "Country.capital: String") || !strings.Contains(s, "writes the query for country") {
		t.Fatalf("nested details:\n%s", s)
	}
	h.key(tcell.KeyRune, 'i', 0)
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
