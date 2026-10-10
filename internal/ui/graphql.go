package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/graphql"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/pkg/httpclient"
)

// browseSchema fetches the GraphQL schema of the request's URL (with its
// headers and auth) and opens the schema browser. Schemas are kept per URL
// for the session; reload refetches.
func (a *App) browseSchema(reload bool) {
	req := a.req.Clone()
	req.Method, req.BodyType = "POST", models.BodyGraphQL
	req.Body, req.GraphQLVariables = graphql.IntrospectionQuery, ""
	p, err := engine.Prepare(req, a.ws.VariableMap())
	if err != nil {
		a.setStatus(levelError, "Cannot fetch the schema: "+err.Error())
		return
	}
	url := p.Request.URL.String()
	if s := a.schemas[url]; s != nil && !reload {
		a.showSchema(url, s)
		return
	}
	a.setStatus(levelInfo, "Fetching the GraphQL schema from "+url)
	client := httpclient.NewClient(a.clientOptions())
	go func() {
		resp, err := client.Do(context.Background(), p.Request)
		var schema *graphql.Schema
		if err == nil {
			if resp.StatusCode >= 400 {
				err = fmt.Errorf("the server answered %s", resp.Status)
			} else {
				schema, err = graphql.Parse(resp.Body)
			}
		}
		a.tv.QueueUpdateDraw(func() {
			if err != nil {
				a.setStatus(levelError, "Cannot fetch the schema: "+err.Error())
				return
			}
			if a.schemas == nil {
				a.schemas = map[string]*graphql.Schema{}
			}
			a.schemas[url] = schema
			a.setStatus(levelSuccess, fmt.Sprintf("Schema loaded: %d types", len(schema.Types)))
			a.showSchema(url, schema)
		})
	}()
}

// schemaRef is what a node in the schema browser stands for.
type schemaRef struct {
	op    string // "query", "mutation" or "subscription" for root fields
	field graphql.Field
}

func (a *App) showSchema(url string, s *graphql.Schema) {
	t := a.theme
	root := tview.NewTreeNode("")
	tree := tview.NewTreeView().SetRoot(root).SetTopLevel(1)
	tree.SetBorder(true).SetTitle(" GraphQL schema · "+tview.Escape(url)+" ").
		SetTitleColor(t.Title).SetBorderColor(t.Focus).SetBorderPadding(0, 0, 1, 1)
	tree.SetBackgroundColor(t.Input)
	tree.SetGraphicsColor(t.Border)

	fieldLabel := func(f graphql.Field) string {
		label := fmt.Sprintf("[%s]%s", t.HexText, tview.Escape(f.Name))
		if len(f.Args) > 0 {
			args := make([]string, len(f.Args))
			for i, arg := range f.Args {
				args[i] = fmt.Sprintf("[%s]%s: [%s]%s", t.HexText, tview.Escape(arg.Name), t.HexInfo, tview.Escape(arg.Type.String()))
			}
			label += fmt.Sprintf("[%s]([-]%s[%s])", t.HexMuted, strings.Join(args, fmt.Sprintf("[%s], ", t.HexMuted)), t.HexMuted)
		}
		label += fmt.Sprintf("[%s]: [%s]%s", t.HexMuted, t.HexAccent, tview.Escape(f.Type.String()))
		if f.Description != "" {
			label += fmt.Sprintf("  [%s]%s", t.HexMuted, tview.Escape(f.Description))
		}
		return label
	}
	// addFields lists a type's fields under node; object fields expand to
	// their own type's fields when opened.
	var addFields func(node *tview.TreeNode, typeName, op string)
	addFields = func(node *tview.TreeNode, typeName, op string) {
		typ := s.Types[typeName]
		if typ == nil {
			return
		}
		for _, f := range typ.Fields {
			n := tview.NewTreeNode(fieldLabel(f)).SetReference(schemaRef{op: op, field: f})
			a.styleNode(n)
			if named := f.Type.Named(); named != nil && s.Types[named.Name] != nil && len(s.Types[named.Name].Fields) > 0 {
				n.SetExpanded(false)
				n.SetText("▸ " + n.GetText())
			}
			node.AddChild(n)
		}
	}
	for _, r := range s.Roots() {
		n := tview.NewTreeNode(fmt.Sprintf("[%s::b]%s[-:-:-] [%s]%s", t.HexAccent, r[0], t.HexMuted, r[1]))
		a.styleNode(n)
		addFields(n, r[1], r[0])
		root.AddChild(n)
	}
	if len(root.GetChildren()) == 0 {
		root.AddChild(tview.NewTreeNode(fmt.Sprintf("[%s]The schema has no query, mutation or subscription type.", t.HexMuted)))
	} else if first := root.GetChildren()[0].GetChildren(); len(first) > 0 {
		tree.SetCurrentNode(first[0])
	}

	toggle := func(n *tview.TreeNode) {
		ref, ok := n.GetReference().(schemaRef)
		if !ok {
			return
		}
		named := ref.field.Type.Named()
		if named == nil || s.Types[named.Name] == nil || len(s.Types[named.Name].Fields) == 0 {
			return
		}
		if len(n.GetChildren()) == 0 {
			addFields(n, named.Name, "") // nested fields are browsed, not inserted
		}
		n.SetExpanded(!n.IsExpanded())
		text := strings.TrimPrefix(strings.TrimPrefix(n.GetText(), "▸ "), "▾ ")
		if n.IsExpanded() {
			n.SetText("▾ " + text)
		} else {
			n.SetText("▸ " + text)
		}
	}

	hint := tview.NewTextView().SetDynamicColors(true)
	hint.SetBackgroundColor(t.Input)
	hint.SetText(fmt.Sprintf(" [%s::b]Enter[-:-:-] [%s]write a query for a top-level field   [%s::b]→ ←[-:-:-] [%s]show / hide a type's fields   [%s::b]r[-:-:-] [%s]reload   [%s::b]Esc[-:-:-] [%s]close",
		t.HexAccent, t.HexMuted, t.HexAccent, t.HexMuted, t.HexAccent, t.HexMuted, t.HexAccent, t.HexMuted))

	tree.SetSelectedFunc(func(n *tview.TreeNode) {
		ref, ok := n.GetReference().(schemaRef)
		if !ok {
			return
		}
		if ref.op == "" {
			toggle(n)
			return
		}
		a.closeDialog()
		a.insertQuery(s, ref)
	})
	tree.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		n := tree.GetCurrentNode()
		switch {
		case ev.Key() == tcell.KeyEsc || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q'):
			a.closeDialog()
		case ev.Key() == tcell.KeyRight && n != nil && !n.IsExpanded():
			toggle(n)
		case ev.Key() == tcell.KeyLeft && n != nil && n.IsExpanded() && len(n.GetChildren()) > 0:
			if _, ok := n.GetReference().(schemaRef); ok {
				toggle(n)
			}
		case ev.Key() == tcell.KeyRune && ev.Rune() == 'r':
			a.closeDialog()
			a.browseSchema(true)
		default:
			return ev
		}
		return nil
	})

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tree, 0, 1, true).
		AddItem(hint, 1, 0, false)
	box.SetBackgroundColor(t.Input)
	a.openDialog(box, 110, 28)
	a.tv.SetFocus(tree)
}

// insertQuery replaces the query with a skeleton for a root field and
// fills in its variables, switching the body to GraphQL.
func (a *App) insertQuery(s *graphql.Schema, ref schemaRef) {
	query, variables := s.Skeleton(ref.op, ref.field)
	a.req.BodyType = models.BodyGraphQL
	if a.req.Method == "GET" || a.req.Method == "HEAD" {
		a.req.Method = "POST"
		a.methodDrop.SetCurrentOption(indexOf(models.Methods, "POST"))
	}
	a.loading = true
	a.bodyType.SetCurrentOption(indexOf(models.BodyTypes, models.BodyGraphQL))
	a.bodyArea.SetText(query, false)
	a.gqlVarsArea.SetText(variables, false)
	a.loading = false
	a.req.Body, a.req.GraphQLVariables = query, variables
	a.layoutBody()
	a.switchTab(indexOfTab(a.tabs, "Body"))
	a.tv.SetFocus(a.bodyArea)
	a.requestChanged()
	msg := fmt.Sprintf("Wrote a %s for %s", ref.op, ref.field.Name)
	if variables != "" {
		msg += ". Fill in the variables, then send"
	}
	a.setStatus(levelSuccess, msg)
}

func indexOfTab(tabs []*tab, name string) int {
	for i, t := range tabs {
		if t.name == name {
			return i
		}
	}
	return 0
}
