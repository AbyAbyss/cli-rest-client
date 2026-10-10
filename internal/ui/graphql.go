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
	op     string // "query", "mutation" or "subscription"; empty for the root type nodes
	parent string // the type the field belongs to
	field  graphql.Field
	root   *schemaRef // the top-level field this one is under (itself for top-level fields)
}

func (a *App) showSchema(url string, s *graphql.Schema) {
	t := a.theme
	root := tview.NewTreeNode("")
	tree := tview.NewTreeView().SetRoot(root).SetTopLevel(1)
	tree.SetBorder(true).SetTitle(" GraphQL schema · "+tview.Escape(url)+" ").
		SetTitleColor(t.Title).SetBorderColor(t.Focus).SetBorderPadding(0, 0, 1, 1)
	tree.SetBackgroundColor(t.Input)
	tree.SetGraphicsColor(t.Border)

	details := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetScrollable(true)
	details.SetBorder(true).SetTitle(" Details ").SetTitleColor(t.Title).SetBorderColor(t.Border).SetBorderPadding(0, 0, 1, 1)
	details.SetBackgroundColor(t.Input)

	expandable := func(f graphql.Field) bool {
		named := f.Type.Named()
		return named != nil && s.Types[named.Name] != nil && len(s.Types[named.Name].Fields) > 0
	}
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
		if expandable(f) {
			label = "▸ " + label
		} else {
			label = "  " + label
		}
		return label
	}
	// addFields lists a type's fields under node. Object fields open to
	// their own type's fields, so the whole schema can be browsed.
	addFields := func(node *tview.TreeNode, typeName, op string, under *schemaRef) {
		typ := s.Types[typeName]
		if typ == nil {
			return
		}
		for _, f := range typ.Fields {
			ref := &schemaRef{op: op, parent: typeName, field: f, root: under}
			if under == nil {
				ref.root = ref
			}
			n := tview.NewTreeNode(fieldLabel(f)).SetReference(ref).SetExpanded(false)
			a.styleNode(n)
			node.AddChild(n)
		}
	}
	for _, r := range s.Roots() {
		n := tview.NewTreeNode(fmt.Sprintf("[%s::b]%s[-:-:-] [%s]%s", t.HexAccent, r[0], t.HexMuted, r[1]))
		a.styleNode(n)
		addFields(n, r[1], r[0], nil)
		root.AddChild(n)
	}

	showDetails := func(n *tview.TreeNode) {
		details.SetText(a.schemaDetails(s, n))
		details.ScrollToBeginning()
	}
	toggle := func(n *tview.TreeNode) {
		ref, ok := n.GetReference().(*schemaRef)
		if !ok || !expandable(ref.field) {
			return
		}
		if len(n.GetChildren()) == 0 {
			addFields(n, ref.field.Type.Named().Name, ref.op, ref.root)
		}
		n.SetExpanded(!n.IsExpanded())
		text := strings.TrimPrefix(strings.TrimPrefix(n.GetText(), "▸ "), "▾ ")
		if n.IsExpanded() {
			n.SetText("▾ " + text)
		} else {
			n.SetText("▸ " + text)
		}
	}
	insert := func(n *tview.TreeNode) {
		ref, ok := n.GetReference().(*schemaRef)
		if !ok {
			a.setStatus(levelWarning, "Pick a field to write a query for")
			return
		}
		a.closeDialog()
		a.insertQuery(s, *ref.root)
	}

	if len(root.GetChildren()) == 0 {
		root.AddChild(tview.NewTreeNode(fmt.Sprintf("[%s]The schema has no query, mutation or subscription type.", t.HexMuted)))
	} else if first := root.GetChildren()[0].GetChildren(); len(first) > 0 {
		tree.SetCurrentNode(first[0])
		showDetails(first[0])
	}

	hint := tview.NewTextView().SetDynamicColors(true)
	hint.SetBackgroundColor(t.Input)
	key := func(k, what string) string {
		return fmt.Sprintf("[%s::b]%s[-:-:-] [%s]%s", t.HexAccent, k, t.HexMuted, what)
	}
	hint.SetText(" " + strings.Join([]string{
		key("Enter / click", "open or close a field"),
		key("i", "insert its query into the Body tab"),
		key("r", "reload"),
		key("Esc", "close"),
	}, "   "))

	tree.SetChangedFunc(showDetails)
	tree.SetSelectedFunc(toggle) // Enter and mouse clicks
	tree.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		n := tree.GetCurrentNode()
		switch {
		case ev.Key() == tcell.KeyEsc || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q'):
			a.closeDialog()
		case ev.Key() == tcell.KeyRune && ev.Rune() == 'i' && n != nil:
			insert(n)
		case ev.Key() == tcell.KeyRune && ev.Rune() == ' ' && n != nil:
			toggle(n)
		case ev.Key() == tcell.KeyRight && n != nil && !n.IsExpanded():
			toggle(n)
		case ev.Key() == tcell.KeyLeft && n != nil && n.IsExpanded() && len(n.GetChildren()) > 0:
			if _, ok := n.GetReference().(*schemaRef); ok {
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

	panes := tview.NewFlex().
		AddItem(tree, 0, 3, true).
		AddItem(details, 0, 2, false)
	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(panes, 0, 1, true).
		AddItem(hint, 1, 0, false)
	box.SetBackgroundColor(t.Input)
	a.openDialog(box, 132, 32)
	a.tv.SetFocus(tree)
}

// schemaDetails describes the selected node: a field's description,
// arguments and result type with its fields, and the query that i writes.
func (a *App) schemaDetails(s *graphql.Schema, n *tview.TreeNode) string {
	t := a.theme
	esc := tview.Escape
	var sb strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&sb, format+"\n", args...) }
	ref, ok := n.GetReference().(*schemaRef)
	if !ok {
		line("[%s]Open a field to see its arguments and type.", t.HexMuted)
		return sb.String()
	}
	f := ref.field
	line("[%s]%s.[%s::b]%s[-:-:-][%s]: [%s]%s", t.HexMuted, esc(ref.parent), t.HexText, esc(f.Name), t.HexMuted, t.HexAccent, esc(f.Type.String()))
	if f.Description != "" {
		line("[%s]%s", t.HexText, esc(f.Description))
	}
	if len(f.Args) > 0 {
		line("")
		line("[%s::b]Arguments", t.HexAccent)
		for _, arg := range f.Args {
			req := ""
			if arg.Type != nil && arg.Type.Kind == "NON_NULL" {
				req = fmt.Sprintf(" [%s](required)", t.HexWarning)
			}
			line("  [%s]%s: [%s]%s%s", t.HexText, esc(arg.Name), t.HexInfo, esc(arg.Type.String()), req)
			if arg.Description != "" {
				line("    [%s]%s", t.HexMuted, esc(arg.Description))
			}
		}
	}
	if named := f.Type.Named(); named != nil {
		if typ := s.Types[named.Name]; typ != nil && len(typ.Fields) > 0 {
			line("")
			line("[%s::b]%s[-:-:-] [%s]fields", t.HexAccent, esc(typ.Name), t.HexMuted)
			if typ.Description != "" {
				line("  [%s]%s", t.HexMuted, esc(typ.Description))
			}
			for _, sub := range typ.Fields {
				line("  [%s]%s[%s]: [%s]%s", t.HexText, esc(sub.Name), t.HexMuted, t.HexInfo, esc(sub.Type.String()))
			}
		}
	}
	query, variables := s.Skeleton(ref.root.op, ref.root.field)
	line("")
	if ref.root != ref {
		line("[%s::b]i[-:-:-] [%s]writes the query for [%s]%s[%s]:", t.HexAccent, t.HexMuted, t.HexText, esc(ref.root.field.Name), t.HexMuted)
	} else {
		line("[%s::b]i[-:-:-] [%s]writes this query:", t.HexAccent, t.HexMuted)
	}
	sb.WriteString("[" + t.HexText + "]" + esc(strings.TrimRight(query, "\n")) + "\n")
	if variables != "" {
		line("[%s]Variables:", t.HexMuted)
		sb.WriteString("[" + t.HexText + "]" + esc(variables) + "\n")
	}
	return sb.String()
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
