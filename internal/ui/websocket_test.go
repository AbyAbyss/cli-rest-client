package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func TestWebSocketSession(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)

	r := models.NewRequest("Echo")
	r.URL = srv.URL + "/ws" // http:// is accepted and becomes ws://
	r.Params = []models.KeyValue{{Key: "room", Value: "{{room}}"}}
	r.Auth = models.Auth{Type: models.AuthBearer, Token: "{{token}}", In: "header"}
	r.Body = `{"hello":"{{room}}"}`
	r.Tests = "events == 3\nevent[0].json.authorization == \"Bearer tok\"\nevent[1].json.hello == lobby\nevent[-1] == second"
	h.do(func() {
		h.a.ws.SetVariable("room", "lobby")
		h.a.ws.SetVariable("token", "tok")
		h.a.tv.SetFocus(h.a.urlInput)
	})
	// Pick WS in the method dropdown, as a user would.
	h.do(func() {
		h.a.loadIntoBuilder(r, nil)
		h.a.methodDrop.SetCurrentOption(len(models.Methods))
		if h.a.req.Type != models.TypeWebSocket || h.a.sendBtn.GetLabel() != "CONNECT" {
			t.Fatalf("type %q, button %q", h.a.req.Type, h.a.sendBtn.GetLabel())
		}
	})

	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("connected and welcomed", func() bool {
		s := h.a.wsSession()
		return s.open() && len(s.messages) == 1
	})
	if s := h.screenText(); !strings.Contains(s, "● Connected") || !strings.Contains(s, `"type":"welcome"`) {
		t.Fatalf("connected view:\n%s", s)
	}
	h.do(func() {
		if h.a.sendBtn.GetLabel() != "SEND" || !strings.Contains(h.a.wsSession().messages[0].Data, `"query":"room=lobby"`) {
			t.Fatalf("button %q, welcome %q", h.a.sendBtn.GetLabel(), h.a.wsSession().messages[0].Data)
		}
	})

	// Ctrl+R now sends the Body tab, with variables filled in.
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("echo", func() bool { return len(h.a.wsSession().messages) == 3 })
	h.do(func() {
		m := h.a.wsSession().messages
		if !m[1].Sent || m[1].Data != `{"hello":"lobby"}` || m[2].Sent || m[2].Data != `{"hello":"lobby"}` {
			t.Fatalf("messages %+v", m)
		}
		h.a.bodyArea.SetText("second", false)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("second echo", func() bool { return len(h.a.wsSession().messages) == 5 })
	if s := h.screenText(); !strings.Contains(s, "↑ 2 sent · ↓ 3 received") || !strings.Contains(s, "Messages") {
		t.Fatalf("log:\n%s", s)
	}

	// Esc disconnects; the tests run on the received messages.
	h.key(tcell.KeyEsc, 0, 0)
	h.eventually("closed", func() bool { return !h.a.wsSession().open() })
	h.do(func() {
		res := h.a.result
		if passed, total := testCounts(res.tests); passed != 4 || total != 4 {
			t.Fatalf("tests %+v", res.tests)
		}
		if !strings.HasPrefix(h.a.status, "WebSocket disconnected after") || !strings.Contains(h.a.status, "2 sent, 3 received, tests 4/4 passed") {
			t.Fatalf("status %q", h.a.status)
		}
		if h.a.sendBtn.GetLabel() != "CONNECT" {
			t.Fatalf("button %q", h.a.sendBtn.GetLabel())
		}
	})
	if s := h.screenText(); !strings.Contains(s, "■ Disconnected") || !strings.Contains(s, "4/4 passed") {
		t.Fatalf("closed view:\n%s", s)
	}
}

func TestWebSocketServerCloseAndErrors(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)

	r := models.NewRequest("Bye")
	r.Type = models.TypeWebSocket
	r.URL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	r.Body = "bye"
	h.do(func() { h.a.loadIntoBuilder(r, nil) })
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("connected", func() bool { return h.a.wsSession().open() && len(h.a.wsSession().messages) == 1 })
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl) // sends "bye": the server closes
	h.eventually("server close", func() bool { return h.a.wsSession() != nil && !h.a.wsSession().open() })
	h.do(func() {
		if got := h.a.wsSession().how; got != "closed normally (1000): bye" {
			t.Fatalf("how %q", got)
		}
	})

	// A server that doesn't upgrade is reported as an error.
	r.URL = srv.URL + "/json"
	h.do(func() { h.a.loadIntoBuilder(r, nil) })
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("refused", func() bool { return h.a.result != nil && h.a.result.err != nil })
	if s := h.screenText(); !strings.Contains(s, "refused the upgrade with 200 OK") {
		t.Fatalf("error view:\n%s", s)
	}

	// The tree shows WS for WebSocket requests.
	h.do(func() {
		h.a.ws.Requests = append(h.a.ws.Requests, &r)
		h.a.rebuildTree(&r)
	})
	if s := h.screenText(); !strings.Contains(s, "WS     Bye") {
		t.Fatalf("tree label:\n%s", s)
	}
}
