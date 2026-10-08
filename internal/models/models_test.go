package models

import (
	"reflect"
	"testing"
)

func TestKVRoundTrip(t *testing.T) {
	text := "Accept: application/json\n\n# X-Off: 1\nReferer: http://a/b:c\nEmpty"
	kvs := ParseKV(text, ":")
	want := []KeyValue{
		{Key: "Accept", Value: "application/json"},
		{Key: "X-Off", Value: "1", Disabled: true},
		{Key: "Referer", Value: "http://a/b:c"},
		{Key: "Empty"},
	}
	if !reflect.DeepEqual(kvs, want) {
		t.Fatalf("got %+v", kvs)
	}
	if !reflect.DeepEqual(ParseKV(FormatKV(kvs, ": "), ":"), want) {
		t.Fatalf("round trip failed")
	}
}

func TestEqualAndClone(t *testing.T) {
	r := NewRequest("a")
	r.Headers = []KeyValue{{Key: "A", Value: "1"}}
	c := r.Clone()
	if !r.Equal(c) {
		t.Fatal("clone should be equal")
	}
	c.Headers[0].Value = "2"
	if r.Headers[0].Value != "1" || r.Equal(c) {
		t.Fatal("clone must be deep")
	}
}

func TestVariables(t *testing.T) {
	w := &Workspace{Variables: []KeyValue{{Key: "a", Value: "1"}, {Key: "b", Value: "2", Disabled: true}}}
	w.SetVariable("b", "3")
	w.SetVariable("c", "4")
	w.UnsetVariable("a")
	if got := w.VariableMap(); !reflect.DeepEqual(got, map[string]string{"b": "3", "c": "4"}) {
		t.Fatalf("got %v", got)
	}
}

func sampleTree() (*Workspace, map[string]*Request, map[string]*Collection) {
	r := map[string]*Request{}
	mk := func(n string) *Request { x := NewRequest(n); r[n] = &x; return &x }
	c := map[string]*Collection{}
	c["deep"] = &Collection{Name: "Deep", Requests: []*Request{mk("d1")}}
	c["sub"] = &Collection{Name: "Sub", Folders: []*Collection{c["deep"]}, Requests: []*Request{mk("s1"), mk("s2")}}
	c["a"] = &Collection{Name: "A", Folders: []*Collection{c["sub"]}, Requests: []*Request{mk("a1")}}
	c["b"] = &Collection{Name: "B"}
	w := &Workspace{Collections: []*Collection{c["a"], c["b"]}, Requests: []*Request{mk("root1")}}
	return w, r, c
}

func TestTreeHelpers(t *testing.T) {
	w, r, c := sampleTree()

	var order []string
	w.WalkRequests(func(path []*Collection, x *Request) {
		order = append(order, PathName(path)+":"+x.Name)
	})
	want := []string{"A / Sub / Deep:d1", "A / Sub:s1", "A / Sub:s2", "A:a1", ":root1"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("walk order %v", order)
	}

	if w.ParentOf(r["d1"]) != c["deep"] || w.ParentOf(r["root1"]) != nil || !w.Contains(r["root1"]) {
		t.Fatal("ParentOf/Contains")
	}
	stray := NewRequest("x")
	if w.Contains(&stray) {
		t.Fatal("stray request reported as contained")
	}
	if w.ParentOfCollection(c["deep"]) != c["sub"] || w.ParentOfCollection(c["a"]) != nil {
		t.Fatal("ParentOfCollection")
	}
	if !w.IsWithin(c["deep"], c["a"]) || w.IsWithin(c["a"], c["deep"]) || w.IsWithin(c["b"], c["a"]) {
		t.Fatal("IsWithin")
	}
	if c["a"].CountRequests() != 4 || !c["a"].Holds(r["d1"]) || c["b"].Holds(r["d1"]) {
		t.Fatal("CountRequests/Holds")
	}

	for name, x := range r {
		folders, idx, ok := w.Location(x)
		if !ok || w.AtLocation(folders, idx) != x {
			t.Fatalf("location round trip failed for %s: %v %d", name, folders, idx)
		}
	}
	if w.AtLocation([]int{9}, 0) != nil || w.AtLocation(nil, 5) != nil {
		t.Fatal("out of range location should be nil")
	}

	cp := c["a"].Clone()
	cp.Folders[0].Requests[0].Name = "changed"
	if r["s1"].Name != "s1" || cp.CountRequests() != 4 {
		t.Fatal("clone must be deep")
	}
}

func TestMigrateV1Draft(t *testing.T) {
	w := &Workspace{Version: 1, Collections: []*Collection{{Name: "C", Requests: []*Request{{Name: "r"}}}},
		Draft: &Draft{Collection: 0, Index: 0}}
	w.Normalize()
	if w.Version != CurrentVersion || !reflect.DeepEqual(w.Draft.Folders, []int{0}) || w.AtLocation(w.Draft.Folders, w.Draft.Index) == nil {
		t.Fatalf("draft not migrated: %+v", w.Draft)
	}
	w = &Workspace{Version: 1, Draft: &Draft{Collection: -1, Index: -1}}
	w.Normalize()
	if w.Draft.Index != -1 || w.Draft.Folders != nil {
		t.Fatalf("unlinked draft migrated wrong: %+v", w.Draft)
	}
}

func TestHistoryAdd(t *testing.T) {
	h := &History{}
	for i := 0; i < HistoryLimit+5; i++ {
		h.Add(&HistoryEntry{URL: string(rune('a' + i%26))})
	}
	if len(h.Entries) != HistoryLimit {
		t.Fatalf("len %d", len(h.Entries))
	}
	big := &HistoryEntry{Body: make([]byte, HistoryBodyLimit+10)}
	h.Add(big)
	if h.Entries[0] != big || len(big.Body) != HistoryBodyLimit || !big.BodyTruncated || big.BodySize != HistoryBodyLimit+10 {
		t.Fatalf("newest first / body cap failed: %d %v %d", len(big.Body), big.BodyTruncated, big.BodySize)
	}
	h.Remove(big)
	if h.Entries[0] == big || len(h.Entries) != HistoryLimit-1 {
		t.Fatal("remove failed")
	}
}

func TestEnvironments(t *testing.T) {
	w := &Workspace{
		Variables: []KeyValue{{Key: "token", Value: "global-token"}, {Key: "baseUrl", Value: "http://global"}},
		Environments: []*Environment{
			{Name: "Local", Variables: []KeyValue{{Key: "baseUrl", Value: "http://localhost"}, {Key: "off", Value: "x", Disabled: true}}},
			{Name: "Prod", Variables: []KeyValue{{Key: "baseUrl", Value: "https://prod"}}},
		},
	}
	if got := w.VariableMap()["baseUrl"]; got != "http://global" {
		t.Fatalf("no env: %s", got)
	}
	w.SetActive("local")
	if w.ActiveEnvironment != "Local" {
		t.Fatalf("SetActive should be case-insensitive and store the real name: %q", w.ActiveEnvironment)
	}
	m := w.VariableMap()
	if m["baseUrl"] != "http://localhost" || m["token"] != "global-token" || m["off"] != "" {
		t.Fatalf("env should override globals: %v", m)
	}
	if w.VariableSource("baseUrl") != "Local" || w.VariableSource("token") != "Globals" || w.VariableSource("nope") != "" {
		t.Fatal("VariableSource")
	}

	// set updates where the variable lives; new ones go to the active env.
	w.SetVariable("token", "t2")
	w.SetVariable("baseUrl", "http://127.0.0.1")
	w.SetVariable("fresh", "1")
	if w.Variables[0].Value != "t2" || w.Environments[0].Variables[0].Value != "http://127.0.0.1" || !hasKey(w.Environments[0].Variables, "fresh") || hasKey(w.Variables, "fresh") {
		t.Fatalf("SetVariable placement wrong: globals %v env %v", w.Variables, w.Environments[0].Variables)
	}
	w.UnsetVariable("baseUrl") // removes the env value, the global shows through again
	if w.VariableMap()["baseUrl"] != "http://global" {
		t.Fatal("UnsetVariable should remove from the env first")
	}

	w.SetActive("missing")
	if w.Active() != nil || w.ActiveEnvironment != "" {
		t.Fatal("unknown env should mean none")
	}
	w.SetVariable("new", "g")
	if !hasKey(w.Variables, "new") {
		t.Fatal("with no env, new variables are global")
	}
	if w.UniqueEnvName("Local") != "Local 2" || w.UniqueEnvName("QA") != "QA" {
		t.Fatal("UniqueEnvName")
	}

	w.ActiveEnvironment = "Gone"
	w.Normalize()
	if w.ActiveEnvironment != "" {
		t.Fatal("Normalize should drop a dangling active environment")
	}
}
