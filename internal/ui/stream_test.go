package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

func TestStreamingResponse(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)

	r := models.NewRequest("Deploy progress")
	r.URL = srv.URL + "/sse?count=4&interval=60"
	r.Tests = "status == 200\nevents == 4\nevent[-1].json.status == deploying\nevent[0].json.step == 1"
	h.do(func() {
		h.a.loadIntoBuilder(r, nil)
		h.a.tv.SetFocus(h.a.urlInput)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)

	// While it streams, the pane shows the events received so far.
	h.eventually("live events", func() bool {
		res := h.a.result
		return h.a.sending && res != nil && res.resp != nil && !res.resp.Done && len(res.resp.Events) >= 1
	})
	if s := h.screenText(); !strings.Contains(s, "Streaming") || !strings.Contains(s, "progress") {
		t.Fatalf("live view:\n%s", s)
	}

	h.eventually("stream end", func() bool { return !h.a.sending })
	h.do(func() {
		res := h.a.result.resp
		if res.Stream != "sse" || len(res.Events) != 4 || res.Stopped {
			t.Fatalf("stream %q, %d events, stopped %v", res.Stream, len(res.Events), res.Stopped)
		}
		passed, total := testCounts(h.a.result.tests)
		if passed != 4 || total != 4 {
			t.Fatalf("tests %d/%d: %+v", passed, total, h.a.result.tests)
		}
		if !strings.Contains(h.a.status, "Stream ended: 200 OK, 4 events") {
			t.Fatalf("status %q", h.a.status)
		}
		if len(h.a.history.Entries) != 1 {
			t.Fatal("a finished stream is recorded in history")
		}
	})
	if s := h.screenText(); !strings.Contains(s, "Ended") || !strings.Contains(s, "#4") || !strings.Contains(s, "Events") {
		t.Fatalf("final view:\n%s", s)
	}

	// History rebuilds the events from the saved body.
	h.do(func() {
		res := resultFromHistory(h.a.history.Entries[0])
		if res.resp.Stream != "sse" || len(res.resp.Events) != 4 {
			t.Fatalf("from history: %+v", res.resp)
		}
	})
}

func TestStopStream(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	h := start(t, nil)

	r := models.NewRequest("Forever")
	r.URL = srv.URL + "/ndjson?count=0&interval=30" // never ends
	r.Tests = "events >= 2"
	h.do(func() {
		h.a.loadIntoBuilder(r, nil)
		h.a.tv.SetFocus(h.a.urlInput)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("a few lines", func() bool {
		res := h.a.result
		return res != nil && res.resp != nil && len(res.resp.Events) >= 3
	})
	h.key(tcell.KeyEsc, 0, 0)
	h.eventually("stopped", func() bool { return !h.a.sending })
	h.do(func() {
		res := h.a.result
		if res.cancelled || res.err != nil || res.resp == nil || !res.resp.Stopped || res.resp.Stream != "ndjson" {
			t.Fatalf("a stopped stream keeps its data: %+v", res)
		}
		if passed, total := testCounts(res.tests); passed != 1 || total != 1 {
			t.Fatalf("tests run on what arrived: %+v", res.tests)
		}
		if !strings.HasPrefix(h.a.status, "Stream stopped: 200 OK") {
			t.Fatalf("status %q", h.a.status)
		}
	})
	if s := h.screenText(); !strings.Contains(s, "Stopped") || !strings.Contains(s, `"status": "queued"`) && !strings.Contains(s, `"status":"queued"`) {
		t.Fatalf("stopped view:\n%s", s)
	}
}
