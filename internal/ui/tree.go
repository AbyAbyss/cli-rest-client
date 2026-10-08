package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

func (a *App) buildTree() {
	a.tree = tview.NewTreeView()
	a.tree.SetBorder(true).SetTitle(" Collections ").SetBorderPadding(0, 0, 1, 1)
	a.tree.SetTopLevel(1) // hide the invisible root
	a.tree.SetSelectedFunc(a.treeSelected)
	a.tree.SetInputCapture(a.treeKeys)
	a.themed = append(a.themed, a.tree)
	a.bordered = append(a.bordered, a.tree)
}

// rebuildTree recreates the nodes from the workspace and selects the node
// whose reference is sel (a *models.Collection or *models.Request).
func (a *App) rebuildTree(sel any) {
	if sel == nil {
		if cur := a.tree.GetCurrentNode(); cur != nil {
			sel = cur.GetReference()
		}
	}
	root := tview.NewTreeNode("")
	var selected *tview.TreeNode
	t := a.theme
	for _, c := range a.ws.Collections {
		icon := "▾"
		if a.collapsed[c] {
			icon = "▸"
		}
		cn := tview.NewTreeNode(fmt.Sprintf("[%s]%s[-] [%s::b]%s[-:-:-] [%s](%d)",
			t.HexMuted, icon, t.HexText, tview.Escape(c.Name), t.HexMuted, len(c.Requests))).
			SetReference(c).
			SetExpanded(!a.collapsed[c])
		a.styleNode(cn)
		root.AddChild(cn)
		if sel == any(c) {
			selected = cn
		}
		for _, r := range c.Requests {
			marker := " "
			if r == a.linked {
				marker = fmt.Sprintf("[%s]●[-]", t.HexAccent)
			}
			rn := tview.NewTreeNode(fmt.Sprintf("%s[%s]%-6s[-] [%s]%s",
				marker, t.methodColor(r.Method), r.Method, t.HexText, tview.Escape(r.Name))).
				SetReference(r)
			a.styleNode(rn)
			cn.AddChild(rn)
			if sel == any(r) {
				selected = rn
			}
		}
	}
	a.tree.SetRoot(root)
	if selected == nil && len(root.GetChildren()) > 0 {
		selected = root.GetChildren()[0]
	}
	if selected != nil {
		a.tree.SetCurrentNode(selected)
	}
	a.renderTitles()
}

func (a *App) styleNode(n *tview.TreeNode) {
	n.SetTextStyle(tcell.StyleDefault.Foreground(a.theme.Text).Background(a.theme.Background))
	n.SetSelectedTextStyle(tcell.StyleDefault.Foreground(a.theme.Text).Background(a.theme.Selection))
}

func (a *App) treeSelected(node *tview.TreeNode) {
	switch ref := node.GetReference().(type) {
	case *models.Collection:
		a.collapsed[ref] = !a.collapsed[ref]
		a.rebuildTree(ref)
	case *models.Request:
		if ref == a.linked {
			a.tv.SetFocus(a.urlInput)
			return
		}
		a.guardUnsaved(func() {
			a.loadIntoBuilder(*ref, ref)
			a.rebuildTree(ref)
			a.setStatus(levelInfo, "Opened "+ref.Name)
			a.tv.SetFocus(a.urlInput)
		})
	}
}

// selection returns the collection and request (if any) under the cursor.
func (a *App) selection() (*models.Collection, *models.Request) {
	node := a.tree.GetCurrentNode()
	if node == nil {
		return nil, nil
	}
	switch ref := node.GetReference().(type) {
	case *models.Collection:
		return ref, nil
	case *models.Request:
		return a.ws.CollectionOf(ref), ref
	}
	return nil, nil
}

func (a *App) treeKeys(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 && ev.Key() == tcell.KeyRune {
		return ev
	}
	switch ev.Key() {
	case tcell.KeyDelete, tcell.KeyBackspace, tcell.KeyBackspace2:
		a.deleteSelected()
		return nil
	case tcell.KeyF2:
		a.renameSelected()
		return nil
	case tcell.KeyUp, tcell.KeyDown:
		if ev.Modifiers()&tcell.ModShift != 0 {
			if ev.Key() == tcell.KeyUp {
				a.moveSelected(-1)
			} else {
				a.moveSelected(1)
			}
			return nil
		}
	case tcell.KeyLeft, tcell.KeyRight:
		if c, r := a.selection(); c != nil && r == nil {
			a.collapsed[c] = ev.Key() == tcell.KeyLeft
			a.rebuildTree(c)
			return nil
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'n':
			a.newCollection()
		case 'a', 'r':
			a.newRequestInCollection()
		case 'e':
			a.renameSelected()
		case 'd':
			a.deleteSelected()
		case 'c':
			a.duplicateSelected()
		case 'K':
			a.moveSelected(-1)
		case 'J':
			a.moveSelected(1)
		case ' ':
			if node := a.tree.GetCurrentNode(); node != nil {
				a.treeSelected(node)
			}
		default:
			return ev
		}
		return nil
	}
	return ev
}

func (a *App) newCollection() {
	a.prompt("New Collection", "Name", "New Collection", func(name string) {
		c := &models.Collection{Name: name}
		a.ws.Collections = append(a.ws.Collections, c)
		a.persist()
		a.rebuildTree(c)
		a.tv.SetFocus(a.tree)
		a.setStatus(levelSuccess, "Created collection "+name)
	})
}

func (a *App) newRequestInCollection() {
	c, _ := a.selection()
	a.guardUnsaved(func() {
		if c == nil {
			if len(a.ws.Collections) == 0 {
				a.ws.Collections = append(a.ws.Collections, &models.Collection{Name: "My Collection"})
			}
			c = a.ws.Collections[0]
		}
		r := models.NewRequest("New Request")
		ptr := &r
		c.Requests = append(c.Requests, ptr)
		a.collapsed[c] = false
		a.persist()
		a.loadIntoBuilder(r, ptr)
		a.rebuildTree(ptr)
		a.prompt("Name Request", "Name", r.Name, func(name string) {
			a.renameRequest(ptr, name)
			a.tv.SetFocus(a.urlInput)
		})
	})
}

func (a *App) renameRequest(r *models.Request, name string) {
	r.Name = name
	if r == a.linked {
		a.req.Name = name
	}
	a.persist()
	a.rebuildTree(r)
}

func (a *App) renameSelected() {
	c, r := a.selection()
	switch {
	case r != nil:
		a.prompt("Rename Request", "Name", r.Name, func(name string) {
			a.renameRequest(r, name)
			a.tv.SetFocus(a.tree)
		})
	case c != nil:
		a.prompt("Rename Collection", "Name", c.Name, func(name string) {
			c.Name = name
			a.persist()
			a.rebuildTree(c)
			a.tv.SetFocus(a.tree)
		})
	}
}

func (a *App) deleteSelected() {
	c, r := a.selection()
	switch {
	case r != nil:
		a.confirm(fmt.Sprintf("Delete request %q?", r.Name), []string{"Delete", "Cancel"}, func(label string) {
			if label != "Delete" {
				return
			}
			ri := indexOfRequest(c.Requests, r)
			c.Requests = append(c.Requests[:ri], c.Requests[ri+1:]...)
			if a.linked == r {
				a.linked = nil
			}
			a.persist()
			var next any = c
			if len(c.Requests) > 0 {
				next = c.Requests[min(ri, len(c.Requests)-1)]
			}
			a.rebuildTree(next)
			a.setStatus(levelInfo, "Deleted "+r.Name)
		})
	case c != nil:
		msg := fmt.Sprintf("Delete collection %q", c.Name)
		if n := len(c.Requests); n > 0 {
			msg += fmt.Sprintf(" and its %d request(s)", n)
		}
		a.confirm(msg+"?", []string{"Delete", "Cancel"}, func(label string) {
			if label != "Delete" {
				return
			}
			ci := 0
			for i, x := range a.ws.Collections {
				if x == c {
					ci = i
				}
			}
			if a.linked != nil && indexOfRequest(c.Requests, a.linked) >= 0 {
				a.linked = nil
			}
			a.ws.Collections = append(a.ws.Collections[:ci], a.ws.Collections[ci+1:]...)
			delete(a.collapsed, c)
			a.persist()
			var next any
			if len(a.ws.Collections) > 0 {
				next = a.ws.Collections[min(ci, len(a.ws.Collections)-1)]
			}
			a.rebuildTree(next)
			a.setStatus(levelInfo, "Deleted collection "+c.Name)
		})
	}
}

func (a *App) duplicateSelected() {
	c, r := a.selection()
	switch {
	case r != nil:
		cp := r.Clone()
		cp.Name = "Copy of " + r.Name
		ptr := &cp
		ri := indexOfRequest(c.Requests, r)
		c.Requests = append(c.Requests[:ri+1], append([]*models.Request{ptr}, c.Requests[ri+1:]...)...)
		a.persist()
		a.rebuildTree(ptr)
		a.setStatus(levelSuccess, "Duplicated "+r.Name)
	case c != nil:
		nc := &models.Collection{Name: "Copy of " + c.Name}
		for _, x := range c.Requests {
			cp := x.Clone()
			nc.Requests = append(nc.Requests, &cp)
		}
		a.ws.Collections = append(a.ws.Collections, nc)
		a.persist()
		a.rebuildTree(nc)
		a.setStatus(levelSuccess, "Duplicated collection "+c.Name)
	}
}

func (a *App) moveSelected(delta int) {
	c, r := a.selection()
	switch {
	case r != nil:
		i := indexOfRequest(c.Requests, r)
		j := i + delta
		if j < 0 || j >= len(c.Requests) {
			return
		}
		c.Requests[i], c.Requests[j] = c.Requests[j], c.Requests[i]
		a.persist()
		a.rebuildTree(r)
	case c != nil:
		cols := a.ws.Collections
		for i := range cols {
			if cols[i] == c {
				j := i + delta
				if j < 0 || j >= len(cols) {
					return
				}
				cols[i], cols[j] = cols[j], cols[i]
				break
			}
		}
		a.persist()
		a.rebuildTree(c)
	}
}

func indexOfRequest(list []*models.Request, r *models.Request) int {
	for i, x := range list {
		if x == r {
			return i
		}
	}
	return -1
}

// suggestName derives a request name from the URL path.
func suggestName(r models.Request) string {
	u := r.URL
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	parts := strings.Split(strings.Trim(u, "/"), "/")
	for i := len(parts) - 1; i >= 1; i-- {
		if p := strings.TrimSpace(parts[i]); p != "" && !strings.HasPrefix(p, "{{") {
			return r.Method + " " + p
		}
	}
	return "New Request"
}
