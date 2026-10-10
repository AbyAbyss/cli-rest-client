package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func TestLoadTestFromTheBuilder(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	testutil.UseLocal(ws, srv.URL)
	h := start(t, ws)
	var copied []string
	r := models.NewRequest("Slow")
	r.URL, r.Tests = "{{baseUrl}}/slow?ms=20", "status == 200"
	h.do(func() {
		h.a.copier = func(text string) (string, error) { copied = append(copied, text); return "test", nil }
		h.a.loadIntoBuilder(r, nil)
	})

	// Alt+L opens the form with the last settings (the defaults at first).
	h.key(tcell.KeyRune, 'l', tcell.ModAlt)
	h.do(func() {
		if len(h.a.dialogs) != 1 {
			t.Fatal("Alt+L should open the load test form")
		}
	})
	s := h.screenText()
	for _, want := range []string{"Load test · Slow", "Load", "Duration", "Stages", "Pass if", "Enter starts"} {
		if !strings.Contains(s, want) {
			t.Fatalf("form missing %q:\n%s", want, s)
		}
	}
	// A bad value is reported and the form stays.
	h.do(func() {
		h.a.tv.GetFocus().(*tview.InputField).SetText("lots")
	})
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.dialogs) != 1 || !strings.Contains(h.a.status, "virtual users like 10") {
			t.Fatalf("bad load: %q", h.a.status)
		}
		h.a.tv.GetFocus().(*tview.InputField).SetText("3")
	})
	h.key(tcell.KeyTab, 0, 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("2s") })
	h.key(tcell.KeyTab, 0, 0)
	h.key(tcell.KeyTab, 0, 0)
	h.key(tcell.KeyTab, 0, 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("p95<5s, errors<1%") })
	h.key(tcell.KeyEnter, 0, 0)

	// The live view, then the summary.
	h.eventually("live view", func() bool { return h.a.loadRun != nil && len(h.a.loadRun.points) > 0 })
	s = h.screenText()
	for _, want := range []string{"3 virtual users for 2s", "Virtual users", "Requests/s", "p95", "Esc stops early"} {
		if !strings.Contains(s, want) {
			t.Fatalf("live view missing %q:\n%s", want, s)
		}
	}
	h.eventually("summary", func() bool { return h.a.loadRun != nil && h.a.loadRun.summary != nil })
	s = h.screenText()
	for _, want := range []string{"3 virtual users for 2s, ran", "requests", "response time", "Checks 100% passed", "✓ p95<5s", "✓ errors<1%", "c copies"} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary missing %q:\n%s", want, s)
		}
	}
	h.do(func() {
		if !strings.Contains(h.a.status, "Load test done") || h.a.loadSettings.Thresholds != "p95<5s, errors<1%" {
			t.Fatalf("status %q settings %+v", h.a.status, h.a.loadSettings)
		}
	})
	h.key(tcell.KeyRune, 'c', 0)
	h.do(func() {
		if len(copied) != 1 || !strings.HasPrefix(copied[0], "Load test: Slow") || !strings.Contains(copied[0], "p95") {
			t.Fatalf("copied %q", copied)
		}
	})
	h.key(tcell.KeyEsc, 0, 0)
	h.do(func() {
		if len(h.a.dialogs) != 0 || h.a.loadRun != nil {
			t.Fatal("Esc should close the results")
		}
	})
}

func TestLoadTestStopAndFolders(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	testutil.UseLocal(ws, srv.URL)
	h := start(t, ws)

	// l on a folder load tests its requests in order.
	h.do(func() {
		h.a.showSidebar(sideCollections, true)
		h.a.tree.GetRoot().Walk(func(n, _ *tview.TreeNode) bool {
			if c, ok := n.GetReference().(*models.Collection); ok && c.Name == "Auth API" {
				h.a.tree.SetCurrentNode(n)
			}
			return true
		})
		h.a.tv.SetFocus(h.a.tree)
	})
	h.key(tcell.KeyRune, 'l', 0)
	if s := h.screenText(); !strings.Contains(s, "Load test · Auth API (2 requests per iteration)") {
		t.Fatalf("folder form:\n%s", s)
	}
	// A long run, stopped early with Esc: the results stay.
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("2") })
	h.key(tcell.KeyTab, 0, 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("1m") })
	h.key(tcell.KeyEnter, 0, 0)
	h.eventually("running", func() bool { return h.a.loadRun != nil && h.a.loadRun.last.Requests > 0 })
	started := time.Now()
	h.key(tcell.KeyEsc, 0, 0)
	h.eventually("stopped", func() bool { return h.a.loadRun != nil && h.a.loadRun.summary != nil })
	if time.Since(started) > 4*time.Second {
		t.Fatal("stopping took too long")
	}
	if s := h.screenText(); !strings.Contains(s, "stopped after") || !strings.Contains(s, "Auth API/Bearer Token") {
		t.Fatalf("stopped summary:\n%s", s)
	}
	h.key(tcell.KeyEsc, 0, 0)

	// Another host asks first; Cancel sends nothing.
	h.do(func() {
		h.a.ws.SetVariable("baseUrl", "https://api.example.com")
		h.a.loadCurrent()
	})
	h.key(tcell.KeyEnter, 0, 0)
	if s := h.screenText(); !strings.Contains(s, "api.example.com") || !strings.Contains(s, "servers you own") {
		t.Fatalf("confirm:\n%s", s)
	}
	h.do(func() {
		h.a.closeDialog()
		if h.a.loadRun != nil {
			t.Fatal("nothing should run before confirming")
		}
		// WebSocket requests aren't load tested.
		r := models.NewRequest("Socket")
		r.Type, r.URL = models.TypeWebSocket, "ws://localhost/ws"
		h.a.loadIntoBuilder(r, nil)
		h.a.loadCurrent()
		if len(h.a.dialogs) != 0 || !strings.Contains(h.a.status, "WebSocket isn't supported") {
			t.Fatalf("websocket: %q", h.a.status)
		}
	})
}
