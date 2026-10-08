package models

import "strings"

// A container is where requests and folders live: a *Collection, or nil for
// the top level of the workspace.

// RequestsIn returns a pointer to the request list of parent (nil = top level).
func (w *Workspace) RequestsIn(parent *Collection) *[]*Request {
	if parent == nil {
		return &w.Requests
	}
	return &parent.Requests
}

// FoldersIn returns a pointer to the folder list of parent (nil = top level,
// i.e. the collections).
func (w *Workspace) FoldersIn(parent *Collection) *[]*Collection {
	if parent == nil {
		return &w.Collections
	}
	return &parent.Folders
}

// WalkCollections visits every collection and folder depth-first.
// ancestors lists the containers above c, outermost first.
func (w *Workspace) WalkCollections(fn func(ancestors []*Collection, c *Collection)) {
	var walk func(anc []*Collection, cols []*Collection)
	walk = func(anc []*Collection, cols []*Collection) {
		for _, c := range cols {
			fn(anc, c)
			walk(append(append([]*Collection(nil), anc...), c), c.Folders)
		}
	}
	walk(nil, w.Collections)
}

// WalkRequests visits every saved request: those in collections (depth-first,
// folders before requests) and then the top-level ones. path lists the
// containing collections, outermost first (empty for top-level requests).
func (w *Workspace) WalkRequests(fn func(path []*Collection, r *Request)) {
	var walk func(path []*Collection, c *Collection)
	walk = func(path []*Collection, c *Collection) {
		path = append(append([]*Collection(nil), path...), c)
		for _, f := range c.Folders {
			walk(path, f)
		}
		for _, r := range c.Requests {
			fn(path, r)
		}
	}
	for _, c := range w.Collections {
		walk(nil, c)
	}
	for _, r := range w.Requests {
		fn(nil, r)
	}
}

// PathOf returns the containers holding r (outermost first) and whether r
// is in the workspace at all.
func (w *Workspace) PathOf(r *Request) ([]*Collection, bool) {
	var found []*Collection
	ok := false
	w.WalkRequests(func(path []*Collection, x *Request) {
		if x == r && !ok {
			found, ok = path, true
		}
	})
	return found, ok
}

// Contains reports whether r is saved somewhere in the workspace.
func (w *Workspace) Contains(r *Request) bool {
	_, ok := w.PathOf(r)
	return ok
}

// ParentOf returns the container directly holding r (nil = top level).
func (w *Workspace) ParentOf(r *Request) *Collection {
	path, _ := w.PathOf(r)
	if len(path) == 0 {
		return nil
	}
	return path[len(path)-1]
}

// AncestorsOf returns the containers above collection c, outermost first,
// and whether c is in the workspace.
func (w *Workspace) AncestorsOf(c *Collection) ([]*Collection, bool) {
	var found []*Collection
	ok := false
	w.WalkCollections(func(anc []*Collection, x *Collection) {
		if x == c && !ok {
			found, ok = anc, true
		}
	})
	return found, ok
}

// ParentOfCollection returns the container holding c (nil = top level).
func (w *Workspace) ParentOfCollection(c *Collection) *Collection {
	anc, _ := w.AncestorsOf(c)
	if len(anc) == 0 {
		return nil
	}
	return anc[len(anc)-1]
}

// IsWithin reports whether c is inside (or is) container root.
func (w *Workspace) IsWithin(c, root *Collection) bool {
	if c == root {
		return true
	}
	anc, _ := w.AncestorsOf(c)
	for _, a := range anc {
		if a == root {
			return true
		}
	}
	return false
}

// PathName joins container names with " / ".
func PathName(path []*Collection) string {
	names := make([]string, len(path))
	for i, c := range path {
		names[i] = c.Name
	}
	return strings.Join(names, " / ")
}

// Location returns the folder indices (from w.Collections down) and the
// request index that address r, so it can be found again after a reload.
func (w *Workspace) Location(r *Request) (folders []int, index int, ok bool) {
	path, ok := w.PathOf(r)
	if !ok {
		return nil, -1, false
	}
	list := w.Collections
	for _, c := range path {
		for i, x := range list {
			if x == c {
				folders = append(folders, i)
				break
			}
		}
		list = c.Folders
	}
	for i, x := range *w.RequestsIn(w.ParentOf(r)) {
		if x == r {
			return folders, i, true
		}
	}
	return nil, -1, false
}

// AtLocation is the inverse of Location.
func (w *Workspace) AtLocation(folders []int, index int) *Request {
	var parent *Collection
	list := w.Collections
	for _, i := range folders {
		if i < 0 || i >= len(list) {
			return nil
		}
		parent = list[i]
		list = parent.Folders
	}
	reqs := *w.RequestsIn(parent)
	if index < 0 || index >= len(reqs) {
		return nil
	}
	return reqs[index]
}

// CountRequests returns how many requests c holds, including sub-folders.
func (c *Collection) CountRequests() int {
	n := len(c.Requests)
	for _, f := range c.Folders {
		n += f.CountRequests()
	}
	return n
}

// Clone returns a deep copy of c.
func (c *Collection) Clone() *Collection {
	out := &Collection{Name: c.Name}
	for _, f := range c.Folders {
		out.Folders = append(out.Folders, f.Clone())
	}
	for _, r := range c.Requests {
		cp := r.Clone()
		out.Requests = append(out.Requests, &cp)
	}
	return out
}

// Holds reports whether r is anywhere inside c.
func (c *Collection) Holds(r *Request) bool {
	for _, x := range c.Requests {
		if x == r {
			return true
		}
	}
	for _, f := range c.Folders {
		if f.Holds(r) {
			return true
		}
	}
	return false
}
