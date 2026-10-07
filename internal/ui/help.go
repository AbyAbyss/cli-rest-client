package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

type helpSection struct {
	title string
	keys  [][2]string
}

var helpSections = []helpSection{
	{"Requests", [][2]string{
		{"Ctrl+R / F5", "Send the request (also Enter in the URL field, Ctrl+Enter where the terminal supports it)"},
		{"Esc", "Cancel a running request"},
		{"Ctrl+S", "Save (asks for a name and collection the first time)"},
		{"Alt+S", "Save as a new request"},
		{"Ctrl+N", "New empty request"},
		{"Ctrl+P", "Pretty-print the JSON body"},
		{"Ctrl+G / F4", "Show the request as a cURL command"},
	}},
	{"Navigation", [][2]string{
		{"Tab / Shift+Tab", "Move between fields"},
		{"Alt+1 … Alt+8", "Switch tab (plain 1-8 works when not typing)"},
		{"Esc", "Jump back to the collections tree"},
		{"Mouse", "Click to focus, click a tab to open it, scroll to scroll"},
	}},
	{"Collections tree", [][2]string{
		{"Enter / Space", "Open request, or expand/collapse collection"},
		{"Left / Right", "Collapse / expand collection"},
		{"n", "New collection"},
		{"a  (or r)", "New request in the selected collection"},
		{"e  (or F2)", "Rename"},
		{"c", "Duplicate"},
		{"d  (or Delete)", "Delete (asks first)"},
		{"Shift+Up/Down, K/J", "Move up / down"},
	}},
	{"Response pane", [][2]string{
		{"Arrows, PgUp/PgDn, g/G", "Scroll"},
		{"s", "Save the response body to a file"},
	}},
	{"Application", [][2]string{
		{"F1 / ?", "This help"},
		{"Ctrl+Q / Ctrl+C", "Quit (unsaved edits are kept for next time)"},
	}},
}

func helpText(t *Theme) string {
	var sb strings.Builder
	for i, s := range helpSections {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "[%s::b]%s[-:-:-]\n", t.HexText, s.title)
		for _, k := range s.keys {
			fmt.Fprintf(&sb, "  [%s]%-22s[-] [%s]%s[-]\n", t.HexAccent, tview.Escape(k[0]), t.HexMuted, tview.Escape(k[1]))
		}
	}
	return sb.String()
}
