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
