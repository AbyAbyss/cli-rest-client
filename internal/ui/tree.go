package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// The tree shows top-level collections, their folders (nested to any depth)
// and requests, followed by requests that are not in any collection. Within
// a container, folders come before requests. A container is a
// *models.Collection, or nil for the top level.

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
// whose reference is sel (a *models.Collection or *models.Request). With sel
// nil the current selection is kept.
func (a *App) rebuildTree(sel any) {
	if sel == nil {
		if cur := a.tree.GetCurrentNode(); cur != nil {
			sel = cur.GetReference()
		}
	}
	root := tview.NewTreeNode("")
	var selected *tview.TreeNode
	t := a.theme

	addRequest := func(parent *tview.TreeNode, r *models.Request) {
		marker := " "
		if r == a.linked {
			marker = fmt.Sprintf("[%s]●[-]", t.HexAccent)
		}
		rn := tview.NewTreeNode(fmt.Sprintf("%s[%s]%-6s[-] [%s]%s",
			marker, t.methodColor(r.Method), r.Method, t.HexText, tview.Escape(r.Name))).
			SetReference(r)
		a.styleNode(rn)
		parent.AddChild(rn)
		if sel == any(r) {
			selected = rn
		}
	}
	var addCollection func(parent *tview.TreeNode, c *models.Collection, topLevel bool)
	addCollection = func(parent *tview.TreeNode, c *models.Collection, topLevel bool) {
		icon := "▾"
		if a.collapsed[c] {
			icon = "▸"
		}
		style := ""
		if topLevel {
			style = "::b"
		}
		cn := tview.NewTreeNode(fmt.Sprintf("[%s]%s[-] [%s%s]%s[-:-:-] [%s](%d)",
			t.HexMuted, icon, t.HexText, style, tview.Escape(c.Name), t.HexMuted, c.CountRequests())).
			SetReference(c).
			SetExpanded(!a.collapsed[c])
		a.styleNode(cn)
		parent.AddChild(cn)
		if sel == any(c) {
			selected = cn
		}
		for _, f := range c.Folders {
			addCollection(cn, f, false)
		}
		for _, r := range c.Requests {
			addRequest(cn, r)
		}
	}
	for _, c := range a.ws.Collections {
		addCollection(root, c, true)
	}
	for _, r := range a.ws.Requests {
		addRequest(root, r)
	}

	a.tree.SetRoot(root)
	if selected != nil {
		// Make sure the selection is visible.
		for _, c := range a.ancestorsOfRef(selected.GetReference()) {
			if a.collapsed[c] {
				delete(a.collapsed, c)
				a.rebuildTree(sel)
				return
			}
		}
	}
	if selected == nil && len(root.GetChildren()) > 0 {
		selected = root.GetChildren()[0]
	}
	if selected != nil {
		a.tree.SetCurrentNode(selected)
	}
	a.renderTitles()
}

func (a *App) ancestorsOfRef(ref any) []*models.Collection {
	switch x := ref.(type) {
	case *models.Request:
		path, _ := a.ws.PathOf(x)
		return path
	case *models.Collection:
		anc, _ := a.ws.AncestorsOf(x)
		return anc
	}
	return nil
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

// selection returns the container or request under the cursor.
func (a *App) selection() (*models.Collection, *models.Request) {
	node := a.tree.GetCurrentNode()
	if node == nil {
		return nil, nil
	}
	switch ref := node.GetReference().(type) {
	case *models.Collection:
		return ref, nil
	case *models.Request:
		return nil, ref
	}
	return nil, nil
}

// targetContainer is where new items go: the selected collection or folder,
// or the container of the selected request (nil = top level).
func (a *App) targetContainer() *models.Collection {
	c, r := a.selection()
	if r != nil {
		return a.ws.ParentOf(r)
	}
	return c
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
	case tcell.KeyLeft:
		c, r := a.selection()
		switch {
		case c != nil && !a.collapsed[c]:
			a.collapsed[c] = true
			a.rebuildTree(c)
		case r != nil && a.ws.ParentOf(r) != nil:
			a.rebuildTree(a.ws.ParentOf(r))
		case c != nil && a.ws.ParentOfCollection(c) != nil:
			a.rebuildTree(a.ws.ParentOfCollection(c))
		}
		return nil
	case tcell.KeyRight:
		if c, _ := a.selection(); c != nil && a.collapsed[c] {
			delete(a.collapsed, c)
			a.rebuildTree(c)
			return nil
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'n':
			a.newContainer(nil)
		case 'f':
			a.newContainer(a.targetContainer())
		case 'a', 'r':
			a.newRequestInContainer()
		case 'e':
			a.renameSelected()
		case 'd':
			a.deleteSelected()
		case 'c':
			a.duplicateSelected()
		case 'm':
			a.moveSelectedTo()
		case 'i':
			a.importPostman()
		case 'x':
			a.exportSelected()
		case 'y':
			if _, r := a.selection(); r != nil {
				a.copyCurl(*r)
			} else {
				a.setStatus(levelWarning, "Select a request to copy it as curl")
			}
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

// newContainer creates a collection (parent nil) or a folder inside parent.
func (a *App) newContainer(parent *models.Collection) {
	title, initial := "New Collection", "New Collection"
	if parent != nil {
		title, initial = "New Folder in "+parent.Name, "New Folder"
	}
	a.prompt(title, "Name", initial, func(name string) {
		c := &models.Collection{Name: name}
		list := a.ws.FoldersIn(parent)
		*list = append(*list, c)
		if parent != nil {
			delete(a.collapsed, parent)
		}
		a.persist()
		a.rebuildTree(c)
		a.tv.SetFocus(a.tree)
		a.setStatus(levelSuccess, "Created "+name)
	})
}

func (a *App) newRequestInContainer() {
	parent := a.targetContainer()
	a.guardUnsaved(func() {
		r := models.NewRequest("New Request")
		ptr := &r
		list := a.ws.RequestsIn(parent)
		*list = append(*list, ptr)
		if parent != nil {
			delete(a.collapsed, parent)
		}
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
		a.prompt("Rename "+a.containerKind(c), "Name", c.Name, func(name string) {
			c.Name = name
			a.persist()
			a.rebuildTree(c)
			a.tv.SetFocus(a.tree)
		})
	}
}

func (a *App) containerKind(c *models.Collection) string {
	if a.ws.ParentOfCollection(c) == nil {
		return "Collection"
	}
	return "Folder"
}

func (a *App) deleteSelected() {
	c, r := a.selection()
	switch {
	case r != nil:
		parent := a.ws.ParentOf(r)
		a.confirm(fmt.Sprintf("Delete request %q?", r.Name), []string{"Delete", "Cancel"}, func(label string) {
			if label != "Delete" {
				return
			}
			list := a.ws.RequestsIn(parent)
			i := indexPtr(*list, r)
			*list = removeAt(*list, i)
			if a.linked == r {
				a.linked = nil
			}
			a.persist()
			var next any = parent
			if len(*list) > 0 {
				next = (*list)[min(i, len(*list)-1)]
			}
			a.rebuildTree(next)
			a.setStatus(levelInfo, "Deleted "+r.Name)
		})
	case c != nil:
		kind := strings.ToLower(a.containerKind(c))
		msg := fmt.Sprintf("Delete %s %q", kind, c.Name)
		var parts []string
		if n := len(c.Folders); n > 0 {
			parts = append(parts, plural(n, "folder"))
		}
		if n := c.CountRequests(); n > 0 {
			parts = append(parts, plural(n, "request"))
		}
		if len(parts) > 0 {
			msg += " with " + strings.Join(parts, " and ")
		}
		parent := a.ws.ParentOfCollection(c)
		a.confirm(msg+"?", []string{"Delete", "Cancel"}, func(label string) {
			if label != "Delete" {
				return
			}
			if a.linked != nil && c.Holds(a.linked) {
				a.linked = nil
			}
			list := a.ws.FoldersIn(parent)
			i := indexPtr(*list, c)
			*list = removeAt(*list, i)
			delete(a.collapsed, c)
			a.persist()
			var next any = parent
			if len(*list) > 0 {
				next = (*list)[min(i, len(*list)-1)]
			}
			a.rebuildTree(next)
			a.setStatus(levelInfo, "Deleted "+c.Name)
		})
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (a *App) duplicateSelected() {
	c, r := a.selection()
	switch {
	case r != nil:
		cp := r.Clone()
		cp.Name = "Copy of " + r.Name
		ptr := &cp
		list := a.ws.RequestsIn(a.ws.ParentOf(r))
		*list = insertAt(*list, indexPtr(*list, r)+1, ptr)
		a.persist()
		a.rebuildTree(ptr)
		a.setStatus(levelSuccess, "Duplicated "+r.Name)
	case c != nil:
		nc := c.Clone()
		nc.Name = "Copy of " + c.Name
		list := a.ws.FoldersIn(a.ws.ParentOfCollection(c))
		*list = insertAt(*list, indexPtr(*list, c)+1, nc)
		a.persist()
		a.rebuildTree(nc)
		a.setStatus(levelSuccess, "Duplicated "+c.Name)
	}
}

// moveSelected moves the selection up or down within its own list.
func (a *App) moveSelected(delta int) {
	c, r := a.selection()
	switch {
	case r != nil:
		if swapWith(*a.ws.RequestsIn(a.ws.ParentOf(r)), r, delta) {
			a.persist()
			a.rebuildTree(r)
		}
	case c != nil:
		if swapWith(*a.ws.FoldersIn(a.ws.ParentOfCollection(c)), c, delta) {
			a.persist()
			a.rebuildTree(c)
		}
	}
}

// moveSelectedTo asks for a destination and moves the selected request or
// folder there. A folder moved to the top level becomes a collection.
func (a *App) moveSelectedTo() {
	c, r := a.selection()
	if c == nil && r == nil {
		return
	}
	var exclude *models.Collection
	name := ""
	current := a.targetContainer()
	if c != nil {
		exclude, name = c, c.Name
		current = a.ws.ParentOfCollection(c)
	} else {
		name = r.Name
	}
	labels, targets := a.containerChoices(exclude)
	sel := 0
	for i, t := range targets {
		if t == current {
			sel = i
		}
	}
	dest := tview.NewDropDown().SetLabel("Move to  ").SetOptions(labels, nil).SetCurrentOption(sel)
	dest.SetTextOptions(" ", " ", "", "", "")
	submit := func() {
		i, _ := dest.GetCurrentOption()
		if i < 0 {
			return
		}
		to := targets[i]
		a.closeDialog()
		if c != nil {
			from := a.ws.FoldersIn(a.ws.ParentOfCollection(c))
			*from = removeAt(*from, indexPtr(*from, c))
			list := a.ws.FoldersIn(to)
			*list = append(*list, c)
		} else {
			from := a.ws.RequestsIn(a.ws.ParentOf(r))
			*from = removeAt(*from, indexPtr(*from, r))
			list := a.ws.RequestsIn(to)
			*list = append(*list, r)
		}
		if to != nil {
			delete(a.collapsed, to)
		}
		a.persist()
		if c != nil {
			a.rebuildTree(c)
		} else {
			a.rebuildTree(r)
		}
		a.setStatus(levelSuccess, fmt.Sprintf("Moved %s to %s", name, labels[i]))
	}
	dest.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEnter && !dest.IsOpen() {
			submit()
			return nil
		}
		return ev
	})
	a.formDialog("Move "+name, "Enter to move · Esc to cancel", []tview.Primitive{dest}, submit, 70)
}

// containerChoices lists every place requests or folders can go: the top
// level and each collection/folder path. Containers inside exclude (and
// exclude itself) are left out, so a folder can't be moved into itself.
func (a *App) containerChoices(exclude *models.Collection) ([]string, []*models.Collection) {
	labels := []string{topLevelLabel}
	targets := []*models.Collection{nil}
	a.ws.WalkCollections(func(anc []*models.Collection, c *models.Collection) {
		if exclude != nil && a.ws.IsWithin(c, exclude) {
			return
		}
		labels = append(labels, models.PathName(append(append([]*models.Collection(nil), anc...), c)))
		targets = append(targets, c)
	})
	return labels, targets
}

const topLevelLabel = "(top level, no collection)"

func indexPtr[T comparable](list []T, x T) int {
	for i, y := range list {
		if y == x {
			return i
		}
	}
	return -1
}

func removeAt[T any](list []T, i int) []T {
	if i < 0 || i >= len(list) {
		return list
	}
	return append(list[:i:i], list[i+1:]...)
}

func insertAt[T any](list []T, i int, x T) []T {
	if i < 0 || i > len(list) {
		i = len(list)
	}
	out := make([]T, 0, len(list)+1)
	out = append(out, list[:i]...)
	out = append(out, x)
	return append(out, list[i:]...)
}

func swapWith[T comparable](list []T, x T, delta int) bool {
	i := indexPtr(list, x)
	j := i + delta
	if i < 0 || j < 0 || j >= len(list) {
		return false
	}
	list[i], list[j] = list[j], list[i]
	return true
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
