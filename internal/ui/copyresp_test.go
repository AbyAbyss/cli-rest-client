package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func TestCopyFromResponse(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)
	var copied []string
	r := models.NewRequest("JSON")
	r.URL = srv.URL + "/json"
	r.Headers = []models.KeyValue{{Key: "X-Trace", Value: "abc"}}
	h.do(func() {
		h.a.copier = func(text string) (string, error) { copied = append(copied, text); return "test", nil }
		h.a.loadIntoBuilder(r, nil)
		h.a.tv.SetFocus(h.a.response)
	})
	// Nothing sent yet.
	h.key(tcell.KeyRune, 'c', 0)
	h.do(func() {
		if len(copied) != 0 || !strings.Contains(h.a.status, "Nothing to copy yet") {
			t.Fatalf("before sending: %q %q", copied, h.a.status)
		}
		h.a.tv.SetFocus(h.a.urlInput)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("response", func() bool { return !h.a.sending && h.a.result != nil && h.a.result.resp != nil })
	h.do(func() { h.a.tv.SetFocus(h.a.response) })

	// c copies the body as received.
	h.key(tcell.KeyRune, 'c', 0)
	h.do(func() {
		if len(copied) != 1 || !strings.HasPrefix(copied[0], `{"slideshow"`) || !strings.Contains(h.a.status, "Copied the response body") {
			t.Fatalf("c: %q %q", copied, h.a.status)
		}
	})

	// y opens the menu with every option.
	h.key(tcell.KeyRune, 'y', 0)
	s := h.screenText()
	for _, want := range []string{"Copy from the response", "Body as received", "Body as pretty JSON", "Response headers as JSON", "Request headers", "Full response"} {
		if !strings.Contains(s, want) {
			t.Fatalf("menu missing %q:\n%s", want, s)
		}
	}
	h.key(tcell.KeyRune, '2', 0) // pretty JSON
	h.key(tcell.KeyRune, 'y', 0)
	h.key(tcell.KeyRune, '4', 0) // response headers as JSON
	h.key(tcell.KeyRune, 'y', 0)
	h.key(tcell.KeyDown, 0, 0) // arrows and Enter: 6 = request headers as JSON
	h.key(tcell.KeyDown, 0, 0)
	h.key(tcell.KeyDown, 0, 0)
	h.key(tcell.KeyDown, 0, 0)
	h.key(tcell.KeyDown, 0, 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.key(tcell.KeyRune, 'y', 0)
	h.key(tcell.KeyRune, '7', 0) // full response
	h.do(func() {
		if len(copied) != 5 || len(h.a.dialogs) != 0 {
			t.Fatalf("copied %d, dialogs %d", len(copied), len(h.a.dialogs))
		}
		if !strings.HasPrefix(copied[1], "{\n  \"slideshow\": {") {
			t.Errorf("pretty: %q", copied[1])
		}
		var hdr map[string]any
		if json.Unmarshal([]byte(copied[2]), &hdr) != nil || hdr["Content-Type"] != "application/json" {
			t.Errorf("response headers JSON: %q", copied[2])
		}
		if json.Unmarshal([]byte(copied[3]), &hdr) != nil || hdr["X-Trace"] != "abc" {
			t.Errorf("request headers JSON: %q", copied[3])
		}
		if !strings.HasPrefix(copied[4], "HTTP/1.1 200 OK\nContent-Length:") || !strings.Contains(copied[4], "\n\n{\"slideshow\"") {
			t.Errorf("full: %q", copied[4])
		}
	})
}

func TestCopyStreamAndSocket(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)
	var copied []string
	r := models.NewRequest("SSE")
	r.URL = srv.URL + "/sse?count=2&interval=10"
	h.do(func() {
		h.a.copier = func(text string) (string, error) { copied = append(copied, text); return "test", nil }
		h.a.loadIntoBuilder(r, nil)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("stream end", func() bool { return !h.a.sending && h.a.result != nil && h.a.result.resp != nil })
	h.do(func() { h.a.tv.SetFocus(h.a.response) })
	h.key(tcell.KeyRune, 'y', 0)
	h.key(tcell.KeyRune, '2', 0) // events as JSON (the body isn't JSON)
	h.do(func() {
		var evs []map[string]any
		if len(copied) != 1 || json.Unmarshal([]byte(copied[0]), &evs) != nil || len(evs) != 2 || evs[1]["type"] != "progress" {
			t.Fatalf("events: %q", copied)
		}
		if d, ok := evs[0]["data"].(map[string]any); !ok || d["status"] != "queued" {
			t.Fatalf("JSON data should stay JSON: %v", evs[0])
		}
	})

	ws := models.NewRequest("WS")
	ws.Type, ws.URL, ws.Body = models.TypeWebSocket, srv.URL+"/ws", "hi"
	h.do(func() { h.a.loadIntoBuilder(ws, nil) })
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("welcome", func() bool { s := h.a.wsSession(); return s.open() && len(s.messages) == 1 })
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("echo", func() bool { return len(h.a.wsSession().messages) == 3 })
	h.do(func() { h.a.tv.SetFocus(h.a.response) })
	h.key(tcell.KeyRune, 'y', 0)
	h.key(tcell.KeyRune, '1', 0)
	h.key(tcell.KeyRune, 'c', 0) // c on a socket copies the first choice too
	h.do(func() {
		var ms []map[string]any
		if len(copied) != 3 || json.Unmarshal([]byte(copied[1]), &ms) != nil || len(ms) != 3 || ms[1]["direction"] != "sent" || ms[2]["data"] != "hi" {
			t.Fatalf("messages: %q", copied)
		}
	})
}
