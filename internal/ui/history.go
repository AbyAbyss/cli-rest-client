package ui

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/pkg/httpclient"
)

// The sidebar switches between the Collections tree and the History list,
// like Postman's sidebar tabs.
const (
	sideCollections = "collections"
	sideHistory     = "history"
)

// now is the clock used for history timestamps and day grouping (tests
// replace it).
var now = time.Now

func (a *App) buildSidebar() *tview.Flex {
	a.historyView = tview.NewTreeView()
	a.historyView.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	a.historyView.SetTopLevel(1)
	a.historyView.SetSelectedFunc(a.historySelected)
	a.historyView.SetInputCapture(a.historyKeys)
	// Called from inside drawing too, while the Application is locked, so
	// it must not call Application methods such as GetFocus.
	a.historyView.SetChangedFunc(func(n *tview.TreeNode) {
		if e, ok := n.GetReference().(*models.HistoryEntry); ok && a.historyView.HasFocus() {
			a.setStatus(levelInfo, describeHistory(e))
		}
	})
	a.themed = append(a.themed, a.historyView)
	a.bordered = append(a.bordered, a.historyView)

	a.sidePages = tview.NewPages().
		AddPage(sideCollections, a.tree, true, true).
		AddPage(sideHistory, a.historyView, true, false)
	a.sideView = sideCollections

	a.sideBar = tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false)
	a.sideBar.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) == 0 {
			return
		}
		a.sideBar.Highlight()
		a.showSidebar(strings.TrimPrefix(added[0], "side-"), true)
	})
	a.unfocusable(a.sideBar)
	a.renderers = append(a.renderers, a.renderSideBar)
	a.themed = append(a.themed, a.sideBar, a.sidePages)

	side := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.sideBar, 1, 0, false).
		AddItem(a.sidePages, 0, 1, false)
	a.themed = append(a.themed, side)
	return side
}

// sidebar is whichever sidebar view is showing.
func (a *App) sidebar() tview.Primitive {
	if a.sideView == sideHistory {
		return a.historyView
	}
	return a.tree
}

func (a *App) showSidebar(view string, focus bool) {
	if view != sideHistory {
		view = sideCollections
	}
	a.sideView = view
	a.sidePages.SwitchToPage(view)
	if view == sideHistory {
		a.rebuildHistory(nil)
	}
	a.renderSideBar()
	if focus {
		a.tv.SetFocus(a.sidebar())
	}
}

func (a *App) renderSideBar() {
	if a.sideBar == nil {
		return
	}
	t := a.theme
	item := func(id, label string) string {
		if a.sideView == id {
			return fmt.Sprintf(`["side-%s"][%s:%s:b] %s [-:-:-][""]`, id, t.HexText, hexOf(t.Selection), label)
		}
		return fmt.Sprintf(`["side-%s"][%s] %s [-][""]`, id, t.HexMuted, label)
	}
	n := 0
	if a.history != nil {
		n = len(a.history.Entries)
	}
	a.sideBar.SetText(item(sideCollections, "Collections") + " " + item(sideHistory, fmt.Sprintf("History (%d)", n)))
}

// ---------- recording ----------

func (a *App) recordHistory(res *sendResult) {
	if a.ws.Settings.DisableHistory || a.history == nil || res.cancelled {
		return
	}
	e := &models.HistoryEntry{
		Time:           res.started,
		Request:        res.request,
		Source:         res.source,
		Environment:    res.env,
		Method:         res.method,
		URL:            res.url,
		RequestHeaders: res.reqHeaders,
	}
	if e.Time.IsZero() {
		e.Time = now()
	}
	if r := res.resp; r != nil {
		e.Status, e.StatusText, e.Proto = r.StatusCode, r.StatusText(), r.Proto
		e.DurationMs = r.Duration.Milliseconds()
		e.ResponseHeaders = r.Headers
		e.Body = r.Body
		e.BodySize = len(r.Body)
		e.BodyTruncated = r.Truncated
		if r.URL != res.url {
			e.FinalURL = r.URL
		}
	} else if res.err != nil {
		e.Error = res.err.Error()
		e.DurationMs = now().Sub(res.started).Milliseconds()
	}
	for _, tr := range res.tests {
		e.Tests = append(e.Tests, models.TestOutcome{
			Source: tr.Source, Passed: tr.Passed, Message: tr.Message, Capture: tr.Capture != nil,
		})
	}
	a.history.Add(e)
	a.saveHistory()
	a.renderSideBar()
	if a.sideView == sideHistory {
		a.rebuildHistory(nil)
	}
}

func (a *App) saveHistory() {
	if a.store == nil {
		return
	}
	if err := a.store.SaveHistory(a.history); err != nil {
		a.setStatus(levelError, "Could not save history: "+err.Error())
	}
}

// resultFromHistory rebuilds what the response pane shows from an entry.
func resultFromHistory(e *models.HistoryEntry) *sendResult {
	res := &sendResult{
		method:      e.Method,
		url:         e.URL,
		started:     e.Time,
		reqHeaders:  e.RequestHeaders,
		request:     e.Request,
		source:      e.Source,
		fromHistory: e,
		env:         e.Environment,
	}
	if e.Error != "" {
		res.err = errors.New(e.Error)
	} else {
		final := e.FinalURL
		if final == "" {
			final = e.URL
		}
		res.resp = &httpclient.Response{
			StatusCode: e.Status,
			Status:     strings.TrimSpace(strconv.Itoa(e.Status) + " " + e.StatusText),
			Proto:      e.Proto,
			Headers:    e.ResponseHeaders,
			Body:       e.Body,
			Truncated:  e.BodyTruncated,
			Duration:   time.Duration(e.DurationMs) * time.Millisecond,
			URL:        final,
		}
	}
	for _, t := range e.Tests {
		r := script.Result{Source: t.Source, Passed: t.Passed, Message: t.Message}
		if t.Capture {
			r.Capture = &script.Assignment{}
		}
		res.tests = append(res.tests, r)
	}
	return res
}

// ---------- list ----------

func dayLabel(t time.Time) string {
	today := now()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := today.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "Today"
	}
	yest := today.AddDate(0, 0, -1)
	y3, m3, d3 := yest.Date()
	if y1 == y3 && m1 == m3 && d1 == d3 {
		return "Yesterday"
	}
	if y1 == y2 {
		return t.Format("Mon 02 Jan")
	}
	return t.Format("Mon 02 Jan 2006")
}

// shortURL keeps the path and query: the sidebar is narrow, and the host is
// usually the same for every entry. The full URL is shown in the status bar
// when the entry is highlighted.
func shortURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.RequestURI()
	}
	return raw
}

// describeHistory is the status bar line for a highlighted entry.
func describeHistory(e *models.HistoryEntry) string {
	parts := []string{e.Method + " " + e.URL}
	if e.Error != "" {
		parts = append(parts, "error: "+e.Error)
	} else {
		parts = append(parts, strings.TrimSpace(strconv.Itoa(e.Status)+" "+e.StatusText), fmt.Sprintf("%d ms", e.DurationMs))
	}
	if e.Environment != "" {
		parts = append(parts, "env "+e.Environment)
	}
	if e.Source != "" {
		parts = append(parts, "from "+e.Source)
	}
	return strings.Join(parts, " · ")
}

func (a *App) historyMatches(e *models.HistoryEntry) bool {
	if a.historyFilter == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{e.Method, e.URL, strconv.Itoa(e.Status), e.Source, e.Request.Name, e.Error, e.Environment}, " "))
	return strings.Contains(hay, strings.ToLower(a.historyFilter))
}

func (a *App) statusColor(code int, failed bool) string {
	t := a.theme
	switch {
	case failed || code >= 500:
		return t.HexError
	case code >= 400:
		return t.HexWarning
	case code >= 300:
		return t.HexInfo
	}
	return t.HexSuccess
}

// rebuildHistory redraws the history list and selects sel (an entry), or
// keeps the current selection when sel is nil.
func (a *App) rebuildHistory(sel *models.HistoryEntry) {
	if a.historyView == nil || a.history == nil {
		return
	}
	if sel == nil {
		if cur := a.historyView.GetCurrentNode(); cur != nil {
			sel, _ = cur.GetReference().(*models.HistoryEntry)
		}
	}
	t := a.theme
	root := tview.NewTreeNode("")
	var day *tview.TreeNode
	var dayName string
	var selected, first *tview.TreeNode
	shown := 0
	for _, e := range a.history.Entries {
		if !a.historyMatches(e) {
			continue
		}
		shown++
		if label := dayLabel(e.Time.Local()); label != dayName || day == nil {
			dayName = label
			day = tview.NewTreeNode(fmt.Sprintf("[%s::b]%s", t.HexText, label)).SetReference(label).
				SetExpanded(!a.historyCollapsed[label])
			a.styleNode(day)
			root.AddChild(day)
		}
		status := strconv.Itoa(e.Status)
		if e.Error != "" {
			status = "ERR"
		}
		n := tview.NewTreeNode(fmt.Sprintf("[%s]%s [%s]%-6s[-] [%s]%-3s[-] [%s]%s",
			t.HexMuted, e.Time.Local().Format("15:04"),
			t.methodColor(e.Method), e.Method,
			a.statusColor(e.Status, e.Error != ""), status,
			t.HexText, tview.Escape(shortURL(e.URL)))).SetReference(e)
		a.styleNode(n)
		day.AddChild(n)
		if first == nil {
			first = n
		}
		if e == sel {
			selected = n
		}
	}
	if len(root.GetChildren()) == 0 {
		msg := "No requests sent yet."
		if a.historyFilter != "" {
			msg = "Nothing matches the filter."
		}
		empty := tview.NewTreeNode(fmt.Sprintf("[%s]%s", t.HexMuted, msg)).SetSelectable(false)
		a.styleNode(empty)
		root.AddChild(empty)
	}
	a.historyView.SetRoot(root)
	switch {
	case selected != nil:
		a.historyView.SetCurrentNode(selected)
	case first != nil:
		a.historyView.SetCurrentNode(first)
	}

	title := fmt.Sprintf(" History (%d) ", len(a.history.Entries))
	if a.historyFilter != "" {
		title = fmt.Sprintf(" History · [%s]%s[-] (%d/%d) ", t.HexWarning, tview.Escape(a.historyFilter), shown, len(a.history.Entries))
	}
	a.historyView.SetTitle(title)
	a.renderSideBar()
}

func (a *App) selectedHistory() *models.HistoryEntry {
	if n := a.historyView.GetCurrentNode(); n != nil {
		e, _ := n.GetReference().(*models.HistoryEntry)
		return e
	}
	return nil
}

func (a *App) historySelected(node *tview.TreeNode) {
	switch ref := node.GetReference().(type) {
	case string: // day heading
		a.historyCollapsed[ref] = !a.historyCollapsed[ref]
		a.rebuildHistory(nil)
	case *models.HistoryEntry:
		a.openHistory(ref)
	}
}

// openHistory loads an entry into the builder as an unsaved request and
// shows the response it got at the time.
func (a *App) openHistory(e *models.HistoryEntry) {
	a.guardUnsaved(func() {
		a.loadIntoBuilder(e.Request, nil)
		a.result = resultFromHistory(e)
		a.renderResponse()
		a.response.ScrollToBeginning()
		a.rebuildTree(nil)
		a.setStatus(levelInfo, "Opened from history ("+e.Time.Local().Format("Mon 02 Jan 15:04")+"). Ctrl+R sends it again, Ctrl+S saves it.")
		a.tv.SetFocus(a.urlInput)
	})
}

func (a *App) historyKeys(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 && ev.Key() == tcell.KeyRune {
		return ev
	}
	switch ev.Key() {
	case tcell.KeyDelete, tcell.KeyBackspace, tcell.KeyBackspace2:
		a.deleteHistoryEntry()
		return nil
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'd':
			a.deleteHistoryEntry()
		case 'X':
			a.clearHistory()
		case 's':
			if e := a.selectedHistory(); e != nil {
				a.guardUnsaved(func() {
					a.loadIntoBuilder(e.Request, nil)
					a.result = resultFromHistory(e)
					a.renderResponse()
					a.saveAs()
				})
			}
		case '/':
			a.prompt("Filter History", "Contains", a.historyFilter, func(v string) {
				a.historyFilter = v
				a.rebuildHistory(nil)
				a.tv.SetFocus(a.historyView)
			})
			// An empty filter can't be submitted from prompt; c clears it.
		case 'c':
			if a.historyFilter != "" {
				a.historyFilter = ""
				a.rebuildHistory(nil)
			}
		case ' ':
			if n := a.historyView.GetCurrentNode(); n != nil {
				a.historySelected(n)
			}
		default:
			return ev
		}
		return nil
	}
	return ev
}

func (a *App) deleteHistoryEntry() {
	e := a.selectedHistory()
	if e == nil {
		return
	}
	// Select the neighbour that will take its place.
	var next *models.HistoryEntry
	for i, x := range a.history.Entries {
		if x == e {
			if i+1 < len(a.history.Entries) {
				next = a.history.Entries[i+1]
			} else if i > 0 {
				next = a.history.Entries[i-1]
			}
		}
	}
	a.history.Remove(e)
	a.saveHistory()
	a.rebuildHistory(next)
}

func (a *App) clearHistory() {
	if len(a.history.Entries) == 0 {
		return
	}
	a.confirm(fmt.Sprintf("Clear all %d history entries?", len(a.history.Entries)), []string{"Clear", "Cancel"}, func(label string) {
		if label != "Clear" {
			return
		}
		a.history.Entries = nil
		a.saveHistory()
		a.rebuildHistory(nil)
		a.setStatus(levelInfo, "History cleared")
	})
}
