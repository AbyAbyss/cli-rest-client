package ui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestPrettyBodyJSON(t *testing.T) {
	th := themes[0]
	out, isJSON := prettyBody([]byte(`{"a":[1,"[x]",true,null],"b":{"c":-2.5e3},"e":[],"f":{}}`), "application/json", th)
	if !isJSON {
		t.Fatal("expected JSON")
	}
	// Strip the color tags and check the text survives intact.
	tv := tview.NewTextView().SetDynamicColors(true)
	tv.SetText(out)
	plain := tv.GetText(true)
	want := "{\n  \"a\": [\n    1,\n    \"[x]\",\n    true,\n    null\n  ],\n  \"b\": {\n    \"c\": -2.5e3\n  },\n  \"e\": [],\n  \"f\": {}\n}"
	if plain != want {
		t.Fatalf("plain text mismatch:\n%s\nwant:\n%s", plain, want)
	}
	if !strings.Contains(out, "["+th.HexJSONKey+"]") || !strings.Contains(out, "["+th.HexJSONNumber+"]") {
		t.Fatalf("missing colors: %s", out)
	}
}

func TestPrettyBodyEscapesText(t *testing.T) {
	out, isJSON := prettyBody([]byte("see [red]docs[-] here"), "text/plain", themes[0])
	if isJSON {
		t.Fatal("not JSON")
	}
	tv := tview.NewTextView().SetDynamicColors(true)
	tv.SetText(out)
	if got := tv.GetText(true); got != "see [red]docs[-] here" {
		t.Fatalf("got %q", got)
	}
}

func TestPrettyBodyBinaryAndEmpty(t *testing.T) {
	if out, _ := prettyBody([]byte{0xff, 0xfe, 0x00}, "", themes[0]); !strings.Contains(out, "binary") {
		t.Fatalf("got %q", out)
	}
	if out, _ := prettyBody(nil, "", themes[0]); !strings.Contains(out, "empty") {
		t.Fatalf("got %q", out)
	}
}
