package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

type harness struct {
	t      *testing.T
	a      *App
	screen tcell.SimulationScreen
	store  *storage.Store
	done   chan struct{}
	keys   chan struct{}
	runErr error
}

func start(t *testing.T, ws *models.Workspace) *harness {
	t.Helper()
	store := &storage.Store{Path: filepath.Join(t.TempDir(), "ws.json")}
	if ws == nil {
		ws = storage.SampleWorkspace()
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	a := New(ws, store, BuildInfo{Version: "test", BuildTime: "now"})
	a.tv.SetScreen(screen)
	h := &harness{t: t, a: a, screen: screen, store: store, done: make(chan struct{}), keys: make(chan struct{}, 1000)}
	// Signal every key that reaches the app so tests can wait for it.
	capture := a.tv.GetInputCapture()
	a.tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		defer func() { h.keys <- struct{}{} }()
		return capture(ev)
	})
	go func() { h.runErr = a.Run(); close(h.done) }()
	h.sync()
	screen.SetSize(160, 45)
	h.do(func() {})
	t.Cleanup(func() {
		a.tv.Stop()
		<-h.done
	})
	return h
}

// do runs f while the UI goroutine is paused and waits for it. f runs on a
// helper goroutine so that a failing t.Fatal inside it cannot kill the UI
// event loop.
func (h *harness) do(f func()) {
	h.t.Helper()
	ch := make(chan struct{})
	go h.a.tv.QueueUpdateDraw(func() {
		inner := make(chan struct{})
		go func() { defer close(inner); f() }()
		<-inner
		close(ch)
	})
	select {
	case <-ch:
	case <-h.done:
		h.t.Fatalf("UI exited (err=%v)", h.runErr)
	case <-time.After(5 * time.Second):
		h.t.Fatal("UI did not respond")
	}
	if h.t.Failed() {
		h.t.FailNow()
	}
}

func (h *harness) sync() { h.do(func() {}) }

// key injects a key and waits until the UI has processed it.
func (h *harness) key(k tcell.Key, r rune, mod tcell.ModMask) {
	h.t.Helper()
	h.screen.InjectKey(k, r, mod)
	// Wait until the input capture saw the key; the following round trip
	// then runs after the key has been fully dispatched.
	select {
	case <-h.keys:
	case <-h.done:
		h.t.Fatalf("UI exited (err=%v)", h.runErr)
	case <-time.After(5 * time.Second):
		h.t.Fatal("key was not processed")
	}
	h.sync()
}

func (h *harness) typeText(s string) {
	h.t.Helper()
	for _, r := range s {
		h.key(tcell.KeyRune, r, 0)
	}
}

func (h *harness) eventually(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ok := false
		h.do(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

func (h *harness) screenText() string {
	var sb strings.Builder
	h.do(func() {
		h.a.tv.ForceDraw()
		cells, w, _ := h.screen.GetContents()
		for i, c := range cells {
			if len(c.Runes) > 0 {
				sb.WriteRune(c.Runes[0])
			} else {
				sb.WriteByte(' ')
			}
			if (i+1)%w == 0 {
				sb.WriteByte('\n')
			}
		}
	})
	return sb.String()
}

func (h *harness) focus() tview.Primitive {
	var p tview.Primitive
	h.do(func() { p = h.a.tv.GetFocus() })
	return p
}

func (h *harness) clearURL() {
	h.do(func() { h.a.urlInput.SetText(""); h.a.tv.SetFocus(h.a.urlInput) })
}

func TestTypingDigitsAndLettersInURL(t *testing.T) {
	h := start(t, nil)
	h.clearURL()
	// Digits used to switch tabs and 'e'/'d'/'n'/'r' used to trigger tree actions.
	h.typeText("http://localhost:8080/v1/items/12345?q=need")
	h.do(func() {
		if h.a.req.URL != "http://localhost:8080/v1/items/12345?q=need" {
			t.Fatalf("URL = %q", h.a.req.URL)
		}
		if h.a.activeTab != 0 {
			t.Fatalf("typing digits switched tab to %d", h.a.activeTab)
		}
	})
}

func TestTypingInBodyAndHeaders(t *testing.T) {
	h := start(t, nil)
	h.key(tcell.KeyRune, '4', tcell.ModAlt) // Body tab
	h.do(func() { h.a.tv.SetFocus(h.a.bodyArea); h.a.bodyArea.SetText("", false) })
	h.typeText(`{"n": 18}`)
	h.key(tcell.KeyRune, '3', tcell.ModAlt) // Headers tab
	h.do(func() { h.a.tv.SetFocus(h.a.headersArea) })
	h.typeText("X-A: 1")
	h.do(func() {
		if h.a.req.Body != `{"n": 18}` {
			t.Fatalf("body = %q", h.a.req.Body)
		}
		if len(h.a.req.Headers) != 1 || h.a.req.Headers[0] != (models.KeyValue{Key: "X-A", Value: "1"}) {
			t.Fatalf("headers = %+v", h.a.req.Headers)
		}
		if h.a.activeTab != 2 {
			t.Fatalf("tab = %d", h.a.activeTab)
		}
	})
}

func TestTabSwitching(t *testing.T) {
	h := start(t, nil)
	h.do(func() { h.a.tv.SetFocus(h.a.tree) })
	for i := 1; i <= 8; i++ {
		h.key(tcell.KeyRune, rune('0'+i), 0)
		h.do(func() {
			if h.a.activeTab != i-1 {
				t.Fatalf("pressing %d: tab = %d", i, h.a.activeTab)
			}
		})
	}
	// Settings is wide: the response pane is hidden and not in the cycle.
	h.do(func() {
		if h.a.focusIndex(h.a.response) >= 0 {
			t.Fatal("response should not be focusable on Settings")
		}
	})
	if !strings.Contains(h.screenText(), "Keyboard Shortcuts") {
		t.Fatal("settings page not drawn")
	}
}

func TestTabBarClick(t *testing.T) {
	h := start(t, nil)
	text := h.screenText()
	lines := strings.Split(text, "\n")
	for y, line := range lines {
		if x := strings.Index(line, "6 Tests"); x >= 0 {
			// Convert byte offset to cell offset.
			cx := len([]rune(line[:x]))
			h.screen.InjectMouse(cx+2, y, tcell.Button1, 0)
			h.screen.InjectMouse(cx+2, y, tcell.ButtonNone, 0)
			h.eventually("tests tab after click", func() bool { return h.a.activeTab == 5 })
			if h.focus() != h.a.testsArea {
				t.Fatalf("focus after click = %T", h.focus())
			}
			return
		}
	}
	t.Fatalf("tab bar not found:\n%s", text)
}

func TestFocusCycle(t *testing.T) {
	h := start(t, nil)
	h.do(func() { h.a.tv.SetFocus(h.a.tree) })
	want := []tview.Primitive{h.a.methodDrop, h.a.urlInput, h.a.sendBtn, h.a.paramsArea, h.a.response, h.a.tree}
	for i, w := range want {
		h.key(tcell.KeyTab, 0, 0)
		if f := h.focus(); f != w {
			t.Fatalf("step %d: focus %T, want %T", i, f, w)
		}
	}
	h.key(tcell.KeyBacktab, 0, 0)
	if h.focus() != h.a.response {
		t.Fatalf("shift+tab went to %T", h.focus())
	}
	// Esc returns to the tree instead of quitting, from the URL field too.
	h.key(tcell.KeyEsc, 0, 0)
	if h.focus() != h.a.tree {
		t.Fatalf("esc went to %T", h.focus())
	}
	h.do(func() { h.a.tv.SetFocus(h.a.urlInput) })
	h.key(tcell.KeyEsc, 0, 0)
	if h.focus() != h.a.tree {
		t.Fatalf("esc went to %T", h.focus())
	}
	select {
	case <-h.done:
		t.Fatal("Esc quit the app")
	default:
	}
}

func TestSendRunsScriptsAndTests(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	ws.SetVariable("baseUrl", srv.URL)
	h := start(t, ws)

	// Open Payment Gateway / Charge: it has a pre-request script and tests.
	charge := reqAt(t, ws, "Payment Gateway/Charges/Charge")
	h.do(func() {
		h.a.loadIntoBuilder(*charge, charge)
		h.a.tv.SetFocus(h.a.urlInput)
	})
	h.key(tcell.KeyCtrlR, 0, 0)
	h.eventually("response", func() bool { return h.a.result != nil && h.a.result.resp != nil })
	h.do(func() {
		res := h.a.result
		if res.resp.StatusCode != 200 {
			t.Fatalf("status %d", res.resp.StatusCode)
		}
		passed, total := testCounts(res.tests)
		if total != 3 || passed != 3 {
			t.Fatalf("tests %d/%d: %+v", passed, total, res.tests)
		}
		if !strings.HasPrefix(ws.VariableMap()["orderId"], "order-") {
			t.Fatal("pre-request variable not stored")
		}
		if !strings.Contains(h.a.varsArea.GetText(), "orderId=order-") {
			t.Fatal("variables tab not refreshed")
		}
	})
	if s := h.screenText(); !strings.Contains(s, "200 OK") || !strings.Contains(s, "3/3 passed") {
		t.Fatalf("response not rendered:\n%s", s)
	}
}

func TestSendErrorsAreShown(t *testing.T) {
	h := start(t, nil)
	h.clearURL()
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if h.a.result == nil || h.a.result.err == nil || !strings.Contains(h.a.result.err.Error(), "URL is empty") {
			t.Fatalf("result = %+v", h.a.result)
		}
	})
}

func TestCancelRequest(t *testing.T) {
	block := make(chan struct{})
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	slow := newSlowServer(block)
	defer slow.Close()
	defer close(block)

	h := start(t, nil)
	h.clearURL()
	h.typeText(slow.URL)
	h.key(tcell.KeyF5, 0, 0)
	h.eventually("sending", func() bool { return h.a.sending })
	h.key(tcell.KeyEsc, 0, 0)
	h.eventually("cancelled", func() bool { return !h.a.sending && h.a.result.cancelled })
}

func TestSaveAsAndSave(t *testing.T) {
	h := start(t, nil)
	h.key(tcell.KeyCtrlN, 0, 0) // sample request is clean, so no confirmation
	h.do(func() {
		if h.a.linked != nil {
			t.Fatal("new request should be unlinked")
		}
	})
	h.typeText("http://example.test/widgets")
	h.key(tcell.KeyCtrlS, 0, 0)
	h.do(func() {
		if len(h.a.dialogs) != 1 {
			t.Fatal("save-as dialog not shown")
		}
	})
	h.key(tcell.KeyEnter, 0, 0) // accept suggested name in the first collection
	h.do(func() {
		if h.a.linked == nil || h.a.linked.Name != "GET widgets" || h.a.isDirty() {
			t.Fatalf("linked = %+v dirty=%v", h.a.linked, h.a.isDirty())
		}
		if h.a.ws.ParentOf(h.a.linked) != h.a.ws.Collections[0] {
			t.Fatal("saved to wrong collection")
		}
	})
	saved, _, err := h.store.Load()
	if err != nil || len(saved.Collections[0].Requests) != 3 {
		t.Fatalf("not persisted: %v", err)
	}

	// Edit and save in place.
	h.typeText("/1")
	h.do(func() {
		if !h.a.isDirty() {
			t.Fatal("should be dirty")
		}
	})
	h.key(tcell.KeyCtrlS, 0, 0)
	h.do(func() {
		if h.a.isDirty() || h.a.linked.URL != "http://example.test/widgets/1" {
			t.Fatalf("save failed: %+v", h.a.linked)
		}
	})
}

func TestUnsavedChangesGuard(t *testing.T) {
	h := start(t, nil)
	h.do(func() { h.a.tv.SetFocus(h.a.urlInput) })
	h.typeText("/changed")
	first := h.a.linked
	// Select the second request in the tree.
	h.do(func() {
		h.a.tv.SetFocus(h.a.tree)
		h.a.rebuildTree(h.a.ws.Collections[0].Requests[1])
	})
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.dialogs) != 1 || h.a.linked != first {
			t.Fatal("expected unsaved-changes prompt")
		}
	})
	// Buttons: Save, Discard, Cancel. Move to Discard.
	h.key(tcell.KeyRight, 0, 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.dialogs) != 0 || h.a.linked != h.a.ws.Collections[0].Requests[1] {
			t.Fatalf("discard did not open the other request")
		}
		if strings.Contains(first.URL, "changed") {
			t.Fatal("discard modified the saved request")
		}
	})
}

func TestTreeCreateRenameDelete(t *testing.T) {
	h := start(t, nil)
	h.do(func() { h.a.tv.SetFocus(h.a.tree) })
	before := len(h.a.ws.Collections)

	h.key(tcell.KeyRune, 'n', 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("") })
	h.typeText("Orders")
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.ws.Collections) != before+1 || h.a.ws.Collections[before].Name != "Orders" {
			t.Fatalf("collection not created")
		}
	})

	h.key(tcell.KeyRune, 'a', 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("List orders") })
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		c := h.a.ws.Collections[before]
		if len(c.Requests) != 1 || c.Requests[0].Name != "List orders" || h.a.linked != c.Requests[0] {
			t.Fatalf("request not created/linked")
		}
		h.a.tv.SetFocus(h.a.tree)
	})

	h.key(tcell.KeyRune, 'c', 0) // duplicate
	h.do(func() {
		if n := len(h.a.ws.Collections[before].Requests); n != 2 {
			t.Fatalf("duplicate: %d requests", n)
		}
	})

	h.key(tcell.KeyRune, 'd', 0)
	h.key(tcell.KeyEnter, 0, 0) // "Delete" is the default button
	h.do(func() {
		if n := len(h.a.ws.Collections[before].Requests); n != 1 {
			t.Fatalf("delete: %d requests", n)
		}
	})

	saved, _, _ := h.store.Load()
	if len(saved.Collections) != before+1 {
		t.Fatal("tree changes not persisted")
	}
}

func TestQuitSavesDraft(t *testing.T) {
	h := start(t, nil)
	h.do(func() { h.a.tv.SetFocus(h.a.urlInput) })
	h.typeText("/draft")
	h.screen.InjectKey(tcell.KeyCtrlQ, 0, tcell.ModCtrl)
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		t.Fatal("Ctrl+Q did not quit")
	}
	ws, _, err := h.store.Load()
	if err != nil || ws.Draft == nil || !strings.HasSuffix(ws.Draft.Request.URL, "/draft") || len(ws.Draft.Folders) != 1 || ws.Draft.Folders[0] != 0 || ws.Draft.Index != 0 {
		t.Fatalf("draft = %+v err=%v", ws.Draft, err)
	}

	// Restarting restores the edit, still linked and dirty.
	h2 := start(t, ws)
	h2.do(func() {
		if h2.a.linked != ws.Collections[0].Requests[0] || !h2.a.isDirty() {
			t.Fatal("draft not restored")
		}
	})
}

func TestThemesAndDialogs(t *testing.T) {
	h := start(t, nil)
	for _, name := range ThemeNames() {
		h.do(func() {
			h.a.theme, _ = themeByName(name)
			h.a.applyTheme()
		})
		h.sync()
	}
	h.key(tcell.KeyF1, 0, 0)
	if !strings.Contains(h.screenText(), "Collections tree") {
		t.Fatal("help not shown")
	}
	h.key(tcell.KeyEsc, 0, 0)
	h.key(tcell.KeyCtrlG, 0, 0)
	if !strings.Contains(h.screenText(), "curl") {
		t.Fatal("curl not shown")
	}
	h.key(tcell.KeyEsc, 0, 0)
	h.do(func() {
		if len(h.a.dialogs) != 0 {
			t.Fatal("dialog not closed")
		}
	})
}

func TestAuthFieldsFollowType(t *testing.T) {
	h := start(t, nil)
	h.key(tcell.KeyRune, '2', tcell.ModAlt)
	h.do(func() {
		h.a.authType.SetCurrentOption(3) // API Key
		if h.a.req.Auth.Type != models.AuthAPIKey || len(h.a.authFieldList()) != 3 {
			t.Fatalf("api key fields: %v", h.a.req.Auth.Type)
		}
		h.a.authType.SetCurrentOption(0)
		if h.a.req.Auth.Type != models.AuthNone || len(h.a.authFieldList()) != 0 {
			t.Fatal("none should have no fields")
		}
	})
}

func TestFormatBody(t *testing.T) {
	h := start(t, nil)
	h.do(func() {
		h.a.bodyArea.SetText(`{"a":1,"b":[true]}`, false)
		h.a.formatBody()
		if h.a.bodyArea.GetText() != "{\n  \"a\": 1,\n  \"b\": [\n    true\n  ]\n}" {
			t.Fatalf("got %q", h.a.bodyArea.GetText())
		}
		if h.a.req.BodyType != models.BodyJSON {
			t.Fatal("body type should switch to JSON")
		}
	})
}

// reqAt finds a saved request by its "Collection/Folder/Request" path.
func reqAt(t *testing.T, ws *models.Workspace, path string) *models.Request {
	t.Helper()
	var found *models.Request
	ws.WalkRequests(func(p []*models.Collection, r *models.Request) {
		names := []string{}
		for _, c := range p {
			names = append(names, c.Name)
		}
		if strings.Join(append(names, r.Name), "/") == path {
			found = r
		}
	})
	if found == nil {
		t.Fatalf("no request %q", path)
	}
	return found
}

func TestFoldersAndTopLevelRequests(t *testing.T) {
	empty := &models.Workspace{}
	empty.Normalize()
	h := start(t, empty)
	h.do(func() { h.a.tv.SetFocus(h.a.tree) })

	// n: collection "Shop"; f: folder "Orders" inside it; f again: "Refunds" inside Orders.
	h.key(tcell.KeyRune, 'n', 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("Shop") })
	h.key(tcell.KeyEnter, 0, 0)
	h.key(tcell.KeyRune, 'f', 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("Orders") })
	h.key(tcell.KeyEnter, 0, 0)
	h.key(tcell.KeyRune, 'f', 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("Refunds") })
	h.key(tcell.KeyEnter, 0, 0)

	// a: request inside the selected folder (Refunds).
	h.key(tcell.KeyRune, 'a', 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText("Refund order") })
	h.key(tcell.KeyEnter, 0, 0)
	var shop, orders, refunds *models.Collection
	h.do(func() {
		if len(h.a.ws.Collections) != 1 {
			t.Fatalf("collections: %d", len(h.a.ws.Collections))
		}
		shop = h.a.ws.Collections[0]
		if len(shop.Folders) != 1 || len(shop.Folders[0].Folders) != 1 {
			t.Fatalf("folders not nested: %+v", shop)
		}
		orders, refunds = shop.Folders[0], shop.Folders[0].Folders[0]
		if len(refunds.Requests) != 1 || h.a.linked != refunds.Requests[0] {
			t.Fatal("request not created in the nested folder")
		}
		if !strings.Contains(h.a.urlInput.GetTitle(), "Shop / Orders / Refunds / Refund order") {
			t.Fatalf("title = %q", h.a.urlInput.GetTitle())
		}
		h.a.tv.SetFocus(h.a.tree)
	})
	if s := h.screenText(); !strings.Contains(s, "Refunds (1)") || !strings.Contains(s, "Orders (1)") || !strings.Contains(s, "Shop (1)") {
		t.Fatalf("tree not drawn as expected:\n%s", s)
	}

	// m: move the request to the top level (first option).
	h.key(tcell.KeyRune, 'm', 0)
	h.do(func() { h.a.tv.GetFocus().(*tview.DropDown).SetCurrentOption(0) })
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(refunds.Requests) != 0 || len(h.a.ws.Requests) != 1 || h.a.ws.Requests[0] != h.a.linked {
			t.Fatal("request not moved to top level")
		}
		if !strings.Contains(h.a.urlInput.GetTitle(), "]Refund order[") || strings.Contains(h.a.urlInput.GetTitle(), "Shop") {
			t.Fatalf("title after move = %q", h.a.urlInput.GetTitle())
		}
		// Select Refunds and move it to the top level: it becomes a collection.
		h.a.rebuildTree(refunds)
	})
	h.key(tcell.KeyRune, 'm', 0)
	h.do(func() {
		d := h.a.tv.GetFocus().(*tview.DropDown)
		for i := 0; i < d.GetOptionCount(); i++ {
			if _, label := func() (int, string) { d.SetCurrentOption(i); return d.GetCurrentOption() }(); label == "Shop / Orders / Refunds" {
				t.Fatal("a folder must not be offered as a destination inside itself")
			}
		}
		d.SetCurrentOption(0)
	})
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.ws.Collections) != 2 || h.a.ws.Collections[1] != refunds || len(orders.Folders) != 0 {
			t.Fatal("folder not moved to top level")
		}
		// Delete Shop: Orders goes with it.
		h.a.rebuildTree(shop)
	})
	h.key(tcell.KeyRune, 'd', 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.ws.Collections) != 1 || h.a.ws.Collections[0] != refunds {
			t.Fatal("delete collection failed")
		}
	})

	// Persisted, and the draft link survives a top-level request.
	h.screen.InjectKey(tcell.KeyCtrlQ, 0, tcell.ModCtrl)
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		t.Fatal("Ctrl+Q did not quit")
	}
	saved, _, err := h.store.Load()
	if err != nil || len(saved.Requests) != 1 || saved.Requests[0].Name != "Refund order" || saved.Collections[0].Name != "Refunds" {
		t.Fatalf("not persisted: %v", err)
	}
	if saved.Draft == nil || saved.Draft.Index != 0 || len(saved.Draft.Folders) != 0 {
		t.Fatalf("draft = %+v", saved.Draft)
	}
	h2 := start(t, saved)
	h2.do(func() {
		if h2.a.linked != saved.Requests[0] {
			t.Fatal("top-level draft link not restored")
		}
	})
}

func TestSaveAsIntoNestedFolder(t *testing.T) {
	h := start(t, nil)
	h.key(tcell.KeyCtrlN, 0, tcell.ModCtrl)
	h.typeText("http://example.test/admins")
	h.key(tcell.KeyRune, 's', tcell.ModAlt)
	h.do(func() {
		h.a.tv.SetFocus(h.a.tv.GetFocus()) // name field
	})
	h.key(tcell.KeyTab, 0, 0)
	h.do(func() {
		d := h.a.tv.GetFocus().(*tview.DropDown)
		found := false
		for i := 0; i < d.GetOptionCount(); i++ {
			d.SetCurrentOption(i)
			if _, label := d.GetCurrentOption(); label == "User Service / Users / Admin" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("nested folder not offered in Save As")
		}
	})
	h.key(tcell.KeyBacktab, 0, 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		admin := h.a.ws.Collections[1].Folders[0].Folders[0]
		if h.a.ws.ParentOf(h.a.linked) != admin || len(admin.Requests) != 2 {
			t.Fatalf("not saved into Admin folder")
		}
	})
}

func TestResponseSectionsFold(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	ws.SetVariable("baseUrl", srv.URL)
	h := start(t, ws)
	bearer := reqAt(t, ws, "Auth API/Bearer Token")
	h.do(func() { h.a.loadIntoBuilder(*bearer, bearer); h.a.tv.SetFocus(h.a.urlInput) })
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("response", func() bool { return h.a.result != nil && h.a.result.resp != nil })

	// Defaults: request headers folded, everything else open.
	s := h.screenText()
	for _, want := range []string{"▸ Request Headers (4)", "▾ Response Headers (3)", "▾ Tests 2/2 passed", "▾ Body", "Content-Type: application/json"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Authorization: Bearer") {
		t.Fatal("request headers should start folded")
	}

	// Keys work when the response pane has focus.
	h.do(func() { h.a.tv.SetFocus(h.a.response) })
	h.key(tcell.KeyRune, 'r', 0)
	h.key(tcell.KeyRune, 'h', 0)
	h.key(tcell.KeyRune, 'b', 0)
	s = h.screenText()
	for _, want := range []string{"▾ Request Headers (4)", "Authorization: Bearer my-secret-token", "User-Agent: term-rest-client", "Accept-Encoding: gzip", "▸ Response Headers (3)", "▸ Body"} {
		if !strings.Contains(s, want) {
			t.Fatalf("after toggling, missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Content-Type: application/json") || strings.Contains(s, `"authenticated"`) {
		t.Fatal("folded sections still shown")
	}

	// Clicking a section heading toggles it too.
	lines := strings.Split(s, "\n")
	clicked := false
	for y, line := range lines {
		if x := strings.Index(line, "▸ Response Headers"); x >= 0 {
			cx := len([]rune(line[:x])) + 4
			h.screen.InjectMouse(cx, y, tcell.Button1, 0)
			h.screen.InjectMouse(cx, y, tcell.ButtonNone, 0)
			clicked = true
			break
		}
	}
	if !clicked {
		t.Fatal("heading not on screen")
	}
	h.eventually("click toggles", func() bool { return !h.a.sectionCollapsed(secRespHeaders) })

	// Choices are saved.
	saved, _, err := h.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := saved.Settings.CollapsedSections
	if got[secReqHeaders] || got[secRespHeaders] || !got[secBody] {
		t.Fatalf("collapsed sections not persisted: %v", got)
	}

	// The keys only act in the response pane, not while typing elsewhere.
	h.do(func() { h.a.tv.SetFocus(h.a.urlInput) })
	h.key(tcell.KeyRune, 'b', 0)
	h.do(func() {
		if !h.a.sectionCollapsed(secBody) || !strings.HasSuffix(h.a.req.URL, "b") {
			t.Fatal("typing b in the URL must not toggle the body")
		}
	})
}

func TestHistory(t *testing.T) {
	srv := testutil.NewHTTPBin()
	defer srv.Close()
	ws := storage.SampleWorkspace()
	ws.SetVariable("baseUrl", srv.URL)
	h := start(t, ws)

	send := func(r *models.Request) {
		h.do(func() { h.a.loadIntoBuilder(*r, r); h.a.tv.SetFocus(h.a.urlInput) })
		before := 0
		h.do(func() { before = len(h.a.history.Entries) })
		h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
		h.eventually("history entry", func() bool { return len(h.a.history.Entries) == before+1 })
	}
	bearer := reqAt(t, ws, "Auth API/Bearer Token")
	send(bearer)
	send(reqAt(t, ws, "Health Check"))

	// Newest first, with where it came from, and saved to its own file.
	h.do(func() {
		e := h.a.history.Entries
		if e[0].Request.Name != "Health Check" || e[1].Source != "Auth API / Bearer Token" || e[1].Status != 200 {
			t.Fatalf("entries: %+v / %+v", e[0], e[1])
		}
		if e[1].Request.URL != "{{baseUrl}}/bearer" || e[1].URL != srv.URL+"/bearer" {
			t.Fatal("history must keep the editable request and the resolved URL")
		}
		if e[1].RequestHeaders["Authorization"][0] != "Bearer my-secret-token" || len(e[1].Tests) != 2 {
			t.Fatal("sent headers or test results not recorded")
		}
	})
	if saved, err := h.store.LoadHistory(); err != nil || len(saved.Entries) != 2 {
		t.Fatalf("history not saved: %v", err)
	}

	// Alt+H shows the history list.
	h.key(tcell.KeyRune, 'h', tcell.ModAlt)
	if h.focus() != h.a.historyView {
		t.Fatalf("focus = %T", h.focus())
	}
	s := h.screenText()
	for _, want := range []string{"History (2)", "Today", "GET    200 /bearer"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}

	// Moving the cursor shows the full URL in the status bar.
	h.key(tcell.KeyDown, 0, 0)
	if s := h.screenText(); !strings.Contains(s, "GET "+srv.URL+"/bearer · 200 OK") || !strings.Contains(s, "from Auth API / Bearer Token") {
		t.Fatalf("status bar should describe the entry:\n%s", s)
	}

	// Open the older entry: request and its old response come back, unsaved.
	h.do(func() {
		h.a.loadIntoBuilder(models.NewRequest(""), nil) // clean builder, no prompt
		h.a.rebuildHistory(h.a.history.Entries[1])
		h.a.tv.SetFocus(h.a.historyView)
	})
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if h.a.req.URL != "{{baseUrl}}/bearer" || h.a.req.Auth.Type != models.AuthBearer || h.a.linked != nil {
			t.Fatalf("request not restored: %+v linked=%v", h.a.req, h.a.linked)
		}
		if h.a.result == nil || h.a.result.fromHistory == nil || h.a.result.resp.StatusCode != 200 {
			t.Fatal("response not restored")
		}
	})
	if s := h.screenText(); !strings.Contains(s, "From history") || !strings.Contains(s, `"authenticated"`) {
		t.Fatalf("history response not shown:\n%s", s)
	}

	// Filter, then clear the filter.
	h.do(func() { h.a.tv.SetFocus(h.a.historyView) })
	h.key(tcell.KeyRune, '/', 0)
	h.typeText("health")
	h.key(tcell.KeyEnter, 0, 0)
	if s := h.screenText(); !strings.Contains(s, "(1/2)") || strings.Contains(s, "200 /bearer") || !strings.Contains(s, "200 /get") {
		t.Fatalf("filter not applied:\n%s", s)
	}
	h.key(tcell.KeyRune, 'c', 0)
	h.do(func() {
		if h.a.historyFilter != "" {
			t.Fatal("filter not cleared")
		}
	})

	// A failed request is recorded too.
	h.do(func() {
		r := models.NewRequest("down")
		r.URL = "http://127.0.0.1:1/nothing"
		h.a.loadIntoBuilder(r, nil)
		h.a.tv.SetFocus(h.a.urlInput)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("error entry", func() bool { return len(h.a.history.Entries) == 3 && h.a.history.Entries[0].Error != "" })
	if s := h.screenText(); !strings.Contains(s, "ERR /nothing") {
		t.Fatalf("error entry not listed:\n%s", s)
	}

	// d deletes the selected entry, X clears everything after confirming.
	h.do(func() { h.a.rebuildHistory(h.a.history.Entries[0]); h.a.tv.SetFocus(h.a.historyView) })
	h.key(tcell.KeyRune, 'd', 0)
	h.do(func() {
		if len(h.a.history.Entries) != 2 || h.a.history.Entries[0].Error != "" {
			t.Fatal("delete failed")
		}
	})
	h.key(tcell.KeyRune, 'X', 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.do(func() {
		if len(h.a.history.Entries) != 0 {
			t.Fatal("clear failed")
		}
	})
	if saved, _ := h.store.LoadHistory(); len(saved.Entries) != 0 {
		t.Fatal("cleared history not saved")
	}

	// With history turned off nothing is recorded.
	h.do(func() {
		h.a.ws.Settings.DisableHistory = true
		h.a.loadIntoBuilder(*bearer, bearer)
		h.a.tv.SetFocus(h.a.urlInput)
	})
	h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
	h.eventually("response", func() bool { return !h.a.sending && h.a.result != nil && h.a.result.resp != nil })
	h.do(func() {
		if len(h.a.history.Entries) != 0 {
			t.Fatal("recorded while history is off")
		}
	})

	// F3 switches back to the collections tree.
	h.key(tcell.KeyF3, 0, 0)
	if h.focus() != h.a.tree {
		t.Fatalf("F3 focus = %T", h.focus())
	}
}

func TestDayLabel(t *testing.T) {
	now = func() time.Time { return time.Date(2026, 10, 8, 9, 0, 0, 0, time.Local) }
	defer func() { now = time.Now }()
	cases := map[time.Time]string{
		time.Date(2026, 10, 8, 0, 1, 0, 0, time.Local):   "Today",
		time.Date(2026, 10, 7, 23, 59, 0, 0, time.Local): "Yesterday",
		time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local):  "Thu 01 Oct",
		time.Date(2025, 12, 31, 12, 0, 0, 0, time.Local): "Wed 31 Dec 2025",
	}
	for in, want := range cases {
		if got := dayLabel(in); got != want {
			t.Errorf("dayLabel(%v) = %q, want %q", in, got, want)
		}
	}
}
