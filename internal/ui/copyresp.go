package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/pkg/httpclient"
)

// copyChoice is one entry of the response copy menu.
type copyChoice struct {
	label string // what is copied
	what  string // for the status message
	text  string
}

// responseCopyChoices lists what can be copied from the response pane.
func (a *App) responseCopyChoices() []copyChoice {
	res := a.result
	if res == nil {
		return nil
	}
	var out []copyChoice
	add := func(label, what, text string) {
		out = append(out, copyChoice{label: label, what: what, text: text})
	}
	if s := res.socket; s != nil {
		if len(s.messages) > 0 {
			type msg struct {
				Direction string `json:"direction"`
				Time      string `json:"time"`
				Data      string `json:"data"`
				Binary    bool   `json:"binary,omitempty"`
			}
			var ms []msg
			var received []string
			for _, m := range s.messages {
				dir := "received"
				if m.Sent {
					dir = "sent"
				} else {
					received = append(received, m.Data)
				}
				ms = append(ms, msg{dir, m.At.Format("2006-01-02T15:04:05.000Z07:00"), m.Data, m.Binary})
			}
			add("Messages as JSON", "the messages as JSON", indentJSON(ms))
			if len(received) > 0 {
				add("Last received message", "the last message", received[len(received)-1])
			}
		}
		if s.handshake != nil {
			add("Handshake response headers", "the handshake headers", headersText(s.handshake.Headers))
			add("Handshake response headers as JSON", "the handshake headers as JSON", headersJSON(s.handshake.Headers))
		}
		if len(res.reqHeaders) > 0 {
			add("Handshake request headers", "the request headers", headersText(res.reqHeaders))
		}
		return out
	}
	r := res.resp
	if r == nil {
		if res.err != nil {
			add("Error message", "the error", res.err.Error())
		}
		return out
	}
	add(fmt.Sprintf("Body as received (%s)", humanBytes(len(r.Body))), "the response body", string(r.Body))
	var pretty bytes.Buffer
	if json.Indent(&pretty, bytes.TrimSpace(r.Body), "", "  ") == nil && pretty.Len() > 0 {
		add("Body as pretty JSON", "the body as pretty JSON", pretty.String())
	}
	if len(r.Events) > 0 {
		add(fmt.Sprintf("Events as JSON (%d)", len(r.Events)), "the events as JSON", eventsJSON(r.Events))
	}
	add("Response headers", "the response headers", headersText(r.Headers))
	add("Response headers as JSON", "the response headers as JSON", headersJSON(r.Headers))
	if len(res.reqHeaders) > 0 {
		add("Request headers", "the request headers", headersText(res.reqHeaders))
		add("Request headers as JSON", "the request headers as JSON", headersJSON(res.reqHeaders))
	}
	full := fmt.Sprintf("%s %s\n%s\n%s", r.Proto, r.Status, headersText(r.Headers), r.Body)
	add("Full response (status, headers and body, like curl -i)", "the full response", full)
	return out
}

// headersText writes headers as "Name: value" lines, sorted by name.
func headersText(h http.Header) string {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, k := range names {
		for _, v := range h[k] {
			sb.WriteString(k + ": " + v + "\n")
		}
	}
	return sb.String()
}

// headersJSON writes headers as a JSON object: a string per header, or an
// array when a header was sent more than once.
func headersJSON(h http.Header) string {
	obj := make(map[string]any, len(h))
	for k, vs := range h {
		if len(vs) == 1 {
			obj[k] = vs[0]
		} else {
			obj[k] = vs
		}
	}
	return indentJSON(obj)
}

func eventsJSON(events []httpclient.Event) string {
	type event struct {
		AtMs int64  `json:"at_ms"`
		Type string `json:"type,omitempty"`
		ID   string `json:"id,omitempty"`
		Data any    `json:"data"`
	}
	out := make([]event, len(events))
	for i, e := range events {
		var data any = e.Data
		if json.Valid([]byte(e.Data)) {
			data = json.RawMessage(e.Data) // keep JSON events as JSON
		}
		out[i] = event{e.At.Milliseconds(), e.Type, e.ID, data}
	}
	return indentJSON(out)
}

func indentJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return buf.String()
}

// copyResponseBody copies the body straight away (c in the response pane).
func (a *App) copyResponseBody() {
	choices := a.responseCopyChoices()
	if len(choices) == 0 {
		a.setStatus(levelWarning, "Nothing to copy yet: send a request first")
		return
	}
	a.copyText(choices[0].text, choices[0].what)
}

// showCopyMenu lets the user pick what to copy from the response (y).
func (a *App) showCopyMenu() {
	choices := a.responseCopyChoices()
	if len(choices) == 0 {
		a.setStatus(levelWarning, "Nothing to copy yet: send a request first")
		return
	}
	t := a.theme
	sel := 0
	tv := tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false)
	tv.SetBackgroundColor(t.Input)
	tv.SetBorder(true).SetTitle(" Copy from the response ").SetTitleColor(t.Title).SetBorderColor(t.Focus).SetBorderPadding(1, 1, 2, 2)
	render := func() {
		var sb strings.Builder
		for i, c := range choices {
			marker, color := "  ", t.HexText
			if i == sel {
				marker, color = "▸ ", t.HexAccent
			}
			fmt.Fprintf(&sb, `["c%d"][%s]%s[%s::b]%d[-:-:-]  [%s]%s[""]`+"\n", i, color, marker, t.HexAccent, i+1, color, tview.Escape(c.label))
		}
		fmt.Fprintf(&sb, "\n[%s]↑/↓ or a number, Enter copies, Esc closes", t.HexMuted)
		tv.SetText(sb.String())
	}
	pick := func(i int) {
		a.closeDialog()
		a.copyText(choices[i].text, choices[i].what)
	}
	render()
	tv.SetHighlightedFunc(func(added, _, _ []string) { // mouse click on a line
		if len(added) > 0 {
			var i int
			if _, err := fmt.Sscanf(added[0], "c%d", &i); err == nil && i < len(choices) {
				pick(i)
			}
		}
	})
	tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyEsc || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q'):
			a.closeDialog()
		case ev.Key() == tcell.KeyUp:
			sel = (sel - 1 + len(choices)) % len(choices)
			render()
		case ev.Key() == tcell.KeyDown:
			sel = (sel + 1) % len(choices)
			render()
		case ev.Key() == tcell.KeyEnter:
			pick(sel)
		case ev.Key() == tcell.KeyRune && ev.Rune() >= '1' && int(ev.Rune()-'1') < len(choices):
			pick(int(ev.Rune() - '1'))
		}
		return nil
	})
	a.openDialog(tv, 72, len(choices)+6)
}
