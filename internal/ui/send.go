package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/codegen"
	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/pkg/httpclient"
)

// sendResult is what the response pane shows.
type sendResult struct {
	method, url string
	started     time.Time
	reqHeaders  http.Header
	request     models.Request // as edited, for history
	source      string         // path of the saved request, if any
	fromHistory *models.HistoryEntry
	env         string // active environment when sent ("" = none)
	resp        *httpclient.Response
	err         error
	cancelled   bool
	missing     []string
	warnings    []string
	tests       []script.Result
}

func (a *App) buildResponse() {
	a.response = tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true).SetRegions(true)
	a.response.SetBorder(true).SetTitle(" Response ").SetBorderPadding(0, 0, 1, 1)
	a.response.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() != tcell.KeyRune || ev.Modifiers() != 0 {
			return ev
		}
		if ev.Rune() == 's' {
			a.saveResponseBody()
			return nil
		}
		for _, sec := range responseSections {
			if ev.Rune() == sec.key {
				a.toggleSection(sec.id)
				return nil
			}
		}
		return ev
	})
	// Clicking a section header folds or unfolds it.
	a.response.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) == 0 {
			return
		}
		a.response.Highlight()
		if id, ok := strings.CutPrefix(added[0], "sec-"); ok {
			a.toggleSection(id)
		}
	})
	a.themed = append(a.themed, a.response)
	a.bordered = append(a.bordered, a.response)
}

// Foldable sections of the response pane, with their toggle keys.
const (
	secTests       = "tests"
	secReqHeaders  = "request_headers"
	secRespHeaders = "response_headers"
	secBody        = "body"
)

var responseSections = []struct {
	id  string
	key rune
}{{secTests, 't'}, {secReqHeaders, 'r'}, {secRespHeaders, 'h'}, {secBody, 'b'}}

func sectionKey(id string) rune {
	for _, s := range responseSections {
		if s.id == id {
			return s.key
		}
	}
	return 0
}

// sectionCollapsed reports whether a section is folded. Request headers start
// folded; everything else starts open. Choices are saved in Settings.
func (a *App) sectionCollapsed(id string) bool {
	if v, ok := a.ws.Settings.CollapsedSections[id]; ok {
		return v
	}
	return id == secReqHeaders
}

func (a *App) toggleSection(id string) {
	if a.ws.Settings.CollapsedSections == nil {
		a.ws.Settings.CollapsedSections = map[string]bool{}
	}
	a.ws.Settings.CollapsedSections[id] = !a.sectionCollapsed(id)
	a.persist()
	row, col := a.response.GetScrollOffset()
	a.renderResponse()
	a.response.ScrollTo(row, col)
}

// writeEvents lists stream events with their arrival time, type and id.
// JSON data is shown compact and highlighted; other data as text.
func writeEvents(line func(string, ...any), events []httpclient.Event, t *Theme) {
	if len(events) == 0 {
		line("  [%s]No events yet.", t.HexMuted)
		return
	}
	for _, e := range events {
		meta := fmt.Sprintf("[%s]+%s", t.HexMuted, eventTime(e.At))
		if e.Type != "" {
			meta += fmt.Sprintf(" [%s]%s", t.HexAccent, tview.Escape(e.Type))
		}
		if e.ID != "" {
			meta += fmt.Sprintf(" [%s]#%s", t.HexMuted, tview.Escape(e.ID))
		}
		data := tview.Escape(e.Data)
		var compact bytes.Buffer
		if json.Compact(&compact, []byte(e.Data)) == nil {
			data = highlightJSON(compact.String(), t)
		}
		line("  %s  [%s]%s", meta, t.HexText, strings.ReplaceAll(data, "\n", "\n      "))
	}
}

// eventTime is the offset of an event from the start of the request.
func eventTime(d time.Duration) string {
	if d <= 0 {
		return "0.000s" // events rebuilt from a saved body have no times
	}
	return fmt.Sprintf("%.3fs", d.Seconds())
}

// writeHeaders lists headers sorted by name.
func writeHeaders(line func(string, ...any), h http.Header, t *Theme) {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		line("  [%s]%s: [%s]%s", t.HexMuted, tview.Escape(k), t.HexText, tview.Escape(strings.Join(h[k], ", ")))
	}
}

func (a *App) buildStatusBar() {
	a.statusMsg = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.statusKeys = tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetTextAlign(tview.AlignRight)
	a.themed = append(a.themed, a.statusMsg, a.statusKeys)
}

func (a *App) clientOptions() httpclient.Options {
	s := a.ws.Settings
	return httpclient.Options{
		Timeout:            time.Duration(s.TimeoutSeconds) * time.Second,
		DisableRedirects:   s.DisableRedirects,
		InsecureSkipVerify: s.InsecureSkipVerify,
	}
}

// send runs the pre-request script, sends the request in the background and
// runs the tests when the response arrives.
func (a *App) send() {
	if a.sending {
		a.setStatus(levelWarning, "A request is already running. Press Esc to cancel it.")
		return
	}
	req := a.req.Clone()
	variables := a.ws.VariableMap()

	if strings.TrimSpace(req.PreRequest) != "" {
		assignments, errs := script.RunPre(req.PreRequest, variables)
		a.applyAssignments(assignments)
		if len(errs) > 0 {
			msgs := make([]string, len(errs))
			for i, e := range errs {
				msgs[i] = e.Error()
			}
			a.result = &sendResult{method: req.Method, url: req.URL, err: fmt.Errorf("pre-request script: %s", strings.Join(msgs, "; "))}
			a.renderResponse()
			a.setStatus(levelError, "Pre-request script failed")
			return
		}
	}

	prepared, err := engine.Prepare(req, variables)
	if err != nil {
		a.result = &sendResult{method: req.Method, url: req.URL, err: err}
		a.renderResponse()
		a.setStatus(levelError, err.Error())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.sending, a.cancel = true, cancel
	res := &sendResult{
		method:     prepared.Request.Method,
		url:        prepared.Request.URL.String(),
		started:    time.Now(),
		missing:    prepared.Missing,
		warnings:   prepared.Warnings,
		reqHeaders: prepared.SentHeaders(),
		request:    req,
		source:     a.linkedPath(),
		env:        a.ws.ActiveEnvironment,
	}
	a.result = res
	a.renderResponse()
	a.setStatus(levelInfo, "Sending "+res.method+" "+res.url)

	client := httpclient.NewClient(a.clientOptions())
	update := a.streamUpdater(res)
	go func() {
		resp, err := client.DoStream(ctx, prepared.Request, update)
		a.tv.QueueUpdateDraw(func() { a.finish(res, req.Tests, resp, err) })
	}()
}

// streamDrawInterval limits how often a stream redraws the response pane.
const streamDrawInterval = 80 * time.Millisecond

// streamUpdater returns a callback for DoStream that shows a stream as it
// arrives. Updates are coalesced: at most one redraw per interval, always
// ending with the latest snapshot.
func (a *App) streamUpdater(res *sendResult) func(*httpclient.Response) {
	var mu sync.Mutex
	var latest *httpclient.Response
	scheduled := false
	return func(snap *httpclient.Response) {
		mu.Lock()
		defer mu.Unlock()
		latest = snap
		if scheduled {
			return
		}
		scheduled = true
		time.AfterFunc(streamDrawInterval, func() {
			mu.Lock()
			snap := latest
			scheduled = false
			mu.Unlock()
			a.tv.QueueUpdateDraw(func() {
				if res != a.result || !a.sending {
					return // finished or superseded in the meantime
				}
				res.resp = snap
				a.renderResponse()
				if a.tv.GetFocus() != a.response {
					a.response.ScrollToEnd() // follow the stream unless the user is reading
				}
			})
		})
	}
}

func (a *App) finish(res *sendResult, tests string, resp *httpclient.Response, err error) {
	a.sending = false
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if res != a.result {
		return // superseded (e.g. the builder was cleared)
	}
	switch {
	case errors.Is(err, context.Canceled):
		res.cancelled = true
		a.setStatus(levelWarning, "Request cancelled")
	case err != nil:
		res.err = err
		a.setStatus(levelError, "Request failed")
	default:
		res.resp = resp
		if strings.TrimSpace(tests) != "" {
			variables := a.ws.VariableMap()
			res.tests = script.RunTests(tests, script.Response{
				Status: resp.StatusCode, Headers: resp.Headers, Body: resp.Body, Duration: resp.Duration,
				Events: eventData(resp.Events),
			}, variables)
			var assignments []script.Assignment
			for _, r := range res.tests {
				if r.Capture != nil {
					assignments = append(assignments, *r.Capture)
				}
			}
			a.applyAssignments(assignments)
		}
		passed, total := testCounts(res.tests)
		msg := fmt.Sprintf("%s in %d ms", resp.Status, resp.Duration.Milliseconds())
		if resp.Stream != "" {
			msg = fmt.Sprintf("%s, %s in %s", resp.Status, plural(len(resp.Events), "event"), streamDuration(resp.Duration))
			if resp.Stopped {
				msg = "Stream stopped: " + msg
			} else {
				msg = "Stream ended: " + msg
			}
		}
		level := levelSuccess
		if total > 0 {
			msg += fmt.Sprintf(", tests %d/%d passed", passed, total)
			if passed < total {
				level = levelWarning
			}
		}
		if resp.StatusCode >= 400 {
			level = levelWarning
		}
		a.setStatus(level, msg)
	}
	a.recordHistory(res)
	a.renderResponse()
	if resp == nil || resp.Stream == "" {
		a.response.ScrollToBeginning()
	}
}

func eventData(events []httpclient.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Data
	}
	return out
}

// streamDuration shows short streams in ms and long ones in seconds.
func streamDuration(d time.Duration) string {
	if d < 10*time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return d.Round(100 * time.Millisecond).String()
}

// linkedPath is "Collection / Folder / Request" for the open saved request.
func (a *App) linkedPath() string {
	if a.linked == nil {
		return ""
	}
	path, ok := a.ws.PathOf(a.linked)
	if !ok {
		return ""
	}
	if len(path) == 0 {
		return a.linked.Name
	}
	return models.PathName(path) + " / " + a.linked.Name
}

func testCounts(results []script.Result) (passed, total int) {
	for _, r := range results {
		if r.Capture != nil {
			continue
		}
		total++
		if r.Passed {
			passed++
		}
	}
	return passed, total
}

func (a *App) applyAssignments(as []script.Assignment) {
	if len(as) == 0 {
		return
	}
	for _, x := range as {
		if x.Unset {
			a.ws.UnsetVariable(x.Name)
		} else {
			a.ws.SetVariable(x.Name, x.Value)
		}
	}
	a.refreshVariablesArea()
	a.persist()
}

func (a *App) cancelRequest() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) renderResponse() {
	t := a.theme
	res := a.result
	var sb strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&sb, format+"\n", args...) }
	esc := tview.Escape

	if res == nil {
		line("[%s]No response yet.", t.HexMuted)
		line("")
		line("[%s]Press [%s::b]Ctrl+R[-:-:-][%s], [%s::b]F5[-:-:-][%s] or [%s::b]Enter[-:-:-][%s] in the URL field to send.",
			t.HexMuted, t.HexAccent, t.HexMuted, t.HexAccent, t.HexMuted, t.HexAccent, t.HexMuted)
		a.response.SetTitle(" Response ")
		a.response.SetText(sb.String())
		return
	}

	if res.url != "" {
		line("[%s::b]%s[-:-:-] [%s]%s", t.methodColor(res.method), esc(res.method), t.HexMuted, esc(res.url))
		envLabel := "No Environment"
		if res.env != "" {
			envLabel = res.env
		}
		line("[%s]Environment: [%s]%s", t.HexMuted, t.HexInfo, esc(envLabel))
	}
	for _, m := range res.missing {
		where := "Globals"
		if res.env != "" {
			where = res.env + " or Globals"
		}
		line("[%s]! Unresolved variable {{%s}} (not set in %s)", t.HexWarning, esc(m), esc(where))
	}
	for _, w := range res.warnings {
		line("[%s]! %s", t.HexWarning, esc(w))
	}
	if e := res.fromHistory; e != nil {
		note := "From history · " + e.Time.Local().Format("Mon 02 Jan 2006 15:04:05")
		if e.BodyTruncated {
			note += fmt.Sprintf(" · body kept up to %s of %s", humanBytes(len(e.Body)), humanBytes(e.BodySize))
		}
		line("[%s]%s", t.HexInfo, esc(note))
	}
	line("")

	switch {
	case a.sending && res == a.result && res.resp == nil && res.err == nil && !res.cancelled:
		line("[%s]Sending…  [%s](Esc to cancel)", t.HexWarning, t.HexMuted)
		a.response.SetTitle(" Response ")
	case res.cancelled:
		line("[%s]Request cancelled.", t.HexWarning)
		a.response.SetTitle(" Response ")
	case res.err != nil:
		line("[%s::b]Error[-:-:-]", t.HexError)
		line("[%s]%s", t.HexText, esc(res.err.Error()))
		a.response.SetTitle(fmt.Sprintf(" Response · [%s]Error[-] ", t.HexError))
	case res.resp != nil:
		r := res.resp
		color := t.HexSuccess
		switch {
		case r.StatusCode >= 500:
			color = t.HexError
		case r.StatusCode >= 400:
			color = t.HexWarning
		case r.StatusCode >= 300:
			color = t.HexInfo
		}
		size := humanBytes(len(r.Body))
		if r.Truncated {
			size += " (truncated)"
		}
		elapsed := fmt.Sprintf("%d ms", r.Duration.Milliseconds())
		if r.Stream != "" {
			elapsed = streamDuration(r.Duration)
		}
		line("[%s::b]%d %s[-:-:-]   [%s]Time [%s]%s   [%s]Size [%s]%s   [%s]%s",
			color, r.StatusCode, esc(r.StatusText()),
			t.HexMuted, t.HexText, elapsed,
			t.HexMuted, t.HexText, size,
			t.HexMuted, esc(r.Proto))
		if r.Stream != "" {
			kind := map[string]string{"sse": "Server-Sent Events", "ndjson": "NDJSON stream"}[r.Stream]
			switch {
			case !r.Done:
				line("[%s::b]● Streaming[-:-:-] [%s]%s · %s so far  [%s](Esc to stop)", t.HexSuccess, t.HexText, kind, plural(len(r.Events), "event"), t.HexMuted)
			case r.Stopped:
				line("[%s]■ Stopped [%s]%s · %s received", t.HexWarning, t.HexText, kind, plural(len(r.Events), "event"))
			default:
				line("[%s]■ Ended [%s]%s · %s received", t.HexInfo, t.HexText, kind, plural(len(r.Events), "event"))
			}
		}
		if r.URL != "" && r.URL != res.url {
			line("[%s]Redirected to %s", t.HexInfo, esc(r.URL))
		}
		a.response.SetTitle(fmt.Sprintf(" Response · [%s]%d[-] ", color, r.StatusCode))

		// section writes a clickable, foldable heading and reports whether
		// the section's content should be shown.
		section := func(id, title, extra string) bool {
			icon := "▾"
			if a.sectionCollapsed(id) {
				icon = "▸"
			}
			line("")
			line(`["sec-%s"][%s]%s [%s::b]%s[-:-:-] %s  [%s]· %c[""]`,
				id, t.HexMuted, icon, t.HexAccent, title, extra, t.HexMuted, sectionKey(id))
			return !a.sectionCollapsed(id)
		}

		if len(res.tests) > 0 {
			passed, total := testCounts(res.tests)
			summary := t.HexSuccess
			if passed < total {
				summary = t.HexError
			}
			if section(secTests, "Tests", fmt.Sprintf("[%s]%d/%d passed", summary, passed, total)) {
				for _, tr := range res.tests {
					switch {
					case tr.Capture != nil:
						line("  [%s]→ [%s]%s", t.HexInfo, t.HexText, esc(tr.Message))
					case tr.Passed:
						line("  [%s]✓ [%s]%s", t.HexSuccess, t.HexText, esc(tr.Source))
					default:
						line("  [%s]✗ [%s]%s  [%s]%s", t.HexError, t.HexText, esc(tr.Source), t.HexError, esc(tr.Message))
					}
				}
			}
		}

		if section(secReqHeaders, "Request Headers", fmt.Sprintf("[%s](%d)", t.HexMuted, len(res.reqHeaders))) {
			writeHeaders(line, res.reqHeaders, t)
		}
		if section(secRespHeaders, "Response Headers", fmt.Sprintf("[%s](%d)", t.HexMuted, len(r.Headers))) {
			writeHeaders(line, r.Headers, t)
		}
		switch {
		case r.Stream != "":
			if section(secBody, "Events", fmt.Sprintf("[%s](%d)  · s saves the raw stream", t.HexMuted, len(r.Events))) {
				writeEvents(line, r.Events, t)
			}
		case section(secBody, "Body", fmt.Sprintf("[%s]%s  · s to save", t.HexMuted, size)):
			body, _ := prettyBody(r.Body, r.Headers.Get("Content-Type"), t)
			sb.WriteString("[" + t.HexText + "]")
			sb.WriteString(body)
		}
	}
	a.response.SetText(sb.String())
}

// codeLanguage is the language the code window and y use: the last one
// picked in the code window, curl at first.
func (a *App) codeLanguage() codegen.Language {
	if l, ok := codegen.Lookup(a.ws.Settings.CodeLanguage); ok {
		return l
	}
	return codegen.Languages[0]
}

// codeLabel names a language in status messages ("as curl", "as Python").
func codeLabel(l codegen.Language) string {
	if l.ID == "curl" {
		return "curl"
	}
	return l.Name
}

// copyText puts text on the clipboard and reports how it went.
func (a *App) copyText(text, what string) {
	how, err := a.copier(text)
	if err != nil {
		a.setStatus(levelError, "Could not copy: "+err.Error())
		return
	}
	a.setStatus(levelSuccess, fmt.Sprintf("Copied %s to the clipboard (via %s)", what, how))
}

// copyCode copies r, with real values, in the last language picked in the
// code window.
func (a *App) copyCode(r models.Request) {
	l := a.codeLanguage()
	snip, err := codegen.Generate(l.ID, r, a.ws.VariableMap(), false)
	if err != nil {
		a.setStatus(levelError, "Cannot build "+l.Name+": "+err.Error())
		return
	}
	name := r.Name
	if name == "" {
		name = r.Method + " request"
	}
	a.copyText(snip.Text, fmt.Sprintf("%q as %s", name, codeLabel(l)))
	if len(snip.Missing) > 0 {
		a.setStatus(levelWarning, fmt.Sprintf("Copied, but {{%s}} has no value", strings.Join(snip.Missing, "}}, {{")))
	}
}

// showCode shows the request being edited as code: curl, Python,
// JavaScript, Go or HTTPie. Left/Right switch language, c copies, v
// switches between real values and {{variables}}.
func (a *App) showCode() {
	req := a.req.Clone()
	template := false
	lang := 0
	for i, l := range codegen.Languages {
		if l.ID == a.codeLanguage().ID {
			lang = i
		}
	}
	t := a.theme
	tv := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true)
	tv.SetBackgroundColor(t.Input)
	tv.SetTextColor(t.Text)
	tv.SetBorder(true).SetBorderColor(t.Focus).SetTitleColor(t.Title).SetBorderPadding(0, 0, 2, 2)

	var current string
	render := func() bool {
		l := codegen.Languages[lang]
		snip, err := codegen.Generate(l.ID, req, a.ws.VariableMap(), template)
		if err != nil {
			a.setStatus(levelError, "Cannot build "+l.Name+": "+err.Error())
			return false
		}
		current = snip.Text
		mode, other := "real values", "{{variables}}"
		if template {
			mode, other = "{{variables}} kept", "real values"
		}
		tv.SetTitle(" " + l.Name + " · " + tview.Escape(mode) + " ")

		var sb strings.Builder
		for i, x := range codegen.Languages {
			if i == lang {
				fmt.Fprintf(&sb, "[%s::bu]%s[-:-:-]   ", t.HexAccent, x.Name)
			} else {
				fmt.Fprintf(&sb, "[%s]%s[-]   ", t.HexMuted, x.Name)
			}
		}
		fmt.Fprintf(&sb, "\n\n[%s]%s", t.HexText, tview.Escape(strings.TrimRight(snip.Text, "\n")))
		if len(snip.Missing) > 0 {
			fmt.Fprintf(&sb, "\n\n[%s]No value for {{%s}}", t.HexWarning, tview.Escape(strings.Join(snip.Missing, "}}, {{")))
		}
		if strings.TrimSpace(req.PreRequest) != "" && (!template || l.ID != "curl") {
			fmt.Fprintf(&sb, "\n\n[%s]The pre-request script was applied (nothing was saved).", t.HexMuted)
		}
		fmt.Fprintf(&sb, "\n\n[%s::b]←/→[-:-:-] [%s]language   [%s::b]c[-:-:-] [%s]copy   [%s::b]v[-:-:-] [%s]show %s   [%s::b]Esc[-:-:-] [%s]close",
			t.HexAccent, t.HexMuted, t.HexAccent, t.HexMuted, t.HexAccent, t.HexMuted, tview.Escape(other), t.HexAccent, t.HexMuted)
		tv.SetText(sb.String())
		tv.ScrollToBeginning()
		return true
	}
	if !render() {
		return
	}
	switchTo := func(i int) {
		n := len(codegen.Languages)
		lang = (i%n + n) % n
		a.ws.Settings.CodeLanguage = codegen.Languages[lang].ID
		a.persist()
		render()
	}
	tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyEsc || ev.Key() == tcell.KeyEnter || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q'):
			a.closeDialog()
		case ev.Key() == tcell.KeyRight || (ev.Key() == tcell.KeyRune && ev.Rune() == 'l'):
			switchTo(lang + 1)
		case ev.Key() == tcell.KeyLeft || (ev.Key() == tcell.KeyRune && ev.Rune() == 'h'):
			switchTo(lang - 1)
		case ev.Key() == tcell.KeyRune && ev.Rune() >= '1' && int(ev.Rune()-'1') < len(codegen.Languages):
			switchTo(int(ev.Rune() - '1'))
		case ev.Key() == tcell.KeyRune && (ev.Rune() == 'c' || ev.Rune() == 'y'):
			a.closeDialog()
			l := codegen.Languages[lang]
			what := "the " + codeLabel(l) + " code"
			if l.ID == "curl" {
				what = "the curl command"
			}
			if template {
				what += " (with {{variables}})"
			}
			a.copyText(current, what)
		case ev.Key() == tcell.KeyRune && ev.Rune() == 'v':
			template = !template
			render()
		default:
			return ev
		}
		return nil
	})
	a.openDialog(tv, 100, 30)
}

func (a *App) formatBody() {
	if a.req.BodyType == models.BodyGraphQL {
		text := strings.TrimSpace(a.gqlVarsArea.GetText())
		if text == "" {
			return
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(text), "", "  "); err != nil {
			a.setStatus(levelError, "Variables are not valid JSON: "+err.Error())
			return
		}
		a.gqlVarsArea.SetText(buf.String(), false)
		a.setStatus(levelSuccess, "Formatted the GraphQL variables")
		return
	}
	text := strings.TrimSpace(a.bodyArea.GetText())
	if text == "" {
		return
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(text), "", "  "); err != nil {
		a.setStatus(levelError, "Body is not valid JSON: "+err.Error())
		return
	}
	a.bodyArea.SetText(buf.String(), false)
	if a.req.BodyType == models.BodyNone {
		a.bodyType.SetCurrentOption(indexOf(models.BodyTypes, models.BodyJSON))
	}
	a.setStatus(levelSuccess, "Formatted JSON body")
}

func (a *App) saveResponseBody() {
	if a.result == nil || a.result.resp == nil {
		a.setStatus(levelWarning, "No response body to save")
		return
	}
	body := a.result.resp.Body
	name := "response.txt"
	if strings.Contains(a.result.resp.Headers.Get("Content-Type"), "json") {
		name = "response.json"
	}
	a.prompt("Save Response Body", "File", name, func(path string) {
		if err := os.WriteFile(path, body, 0o644); err != nil {
			a.setStatus(levelError, "Save failed: "+err.Error())
			return
		}
		a.setStatus(levelSuccess, fmt.Sprintf("Saved %s to %s", humanBytes(len(body)), path))
	})
}

// ---------- theming ----------

func (a *App) setGlobalStyles() {
	t := a.theme
	tview.Styles.PrimitiveBackgroundColor = t.Background
	tview.Styles.ContrastBackgroundColor = t.Input
	tview.Styles.MoreContrastBackgroundColor = t.Selection
	tview.Styles.BorderColor = t.Border
	tview.Styles.TitleColor = t.Title
	tview.Styles.GraphicsColor = t.Border
	tview.Styles.PrimaryTextColor = t.Text
	tview.Styles.SecondaryTextColor = t.Muted
	tview.Styles.TertiaryTextColor = t.Muted
	tview.Styles.InverseTextColor = t.ButtonText
	tview.Styles.ContrastSecondaryTextColor = t.Muted
}

func (a *App) applyTheme() {
	a.setGlobalStyles()
	t := a.theme
	base := tcell.StyleDefault.Background(t.Background).Foreground(t.Text)
	field := tcell.StyleDefault.Background(t.Input).Foreground(t.Text)
	muted := tcell.StyleDefault.Background(t.Input).Foreground(t.Muted)
	label := tcell.StyleDefault.Background(t.Background).Foreground(t.Muted)
	active := tcell.StyleDefault.Background(t.Button).Foreground(t.ButtonText)

	for _, p := range a.themed {
		if b, ok := p.(interface {
			SetBackgroundColor(tcell.Color) *tview.Box
			SetBorderColor(tcell.Color) *tview.Box
			SetTitleColor(tcell.Color) *tview.Box
		}); ok {
			b.SetBackgroundColor(t.Background)
			b.SetBorderColor(t.Border)
			b.SetTitleColor(t.Title)
		}
		switch w := p.(type) {
		case *tview.InputField:
			w.SetLabelStyle(label)
			w.SetFieldStyle(field)
			w.SetPlaceholderStyle(muted)
			w.SetAutocompleteStyles(t.Input, field, active)
		case *tview.TextArea:
			w.SetTextStyle(base)
			w.SetPlaceholderStyle(tcell.StyleDefault.Background(t.Background).Foreground(t.Muted))
			w.SetSelectedStyle(tcell.StyleDefault.Background(t.Selection).Foreground(t.Text))
		case *tview.DropDown:
			w.SetLabelStyle(label)
			w.SetFieldStyle(field)
			w.SetFocusedStyle(active)
			w.SetPrefixStyle(active)
			w.SetListStyles(field, active)
		case *tview.TextView:
			w.SetTextStyle(base)
		case *tview.TreeView:
			w.SetGraphicsColor(t.Border)
		case *tview.Button:
			w.SetStyle(active)
			w.SetActivatedStyle(tcell.StyleDefault.Background(t.Focus).Foreground(t.ButtonText).Bold(true))
		case *tview.Checkbox:
			w.SetLabelStyle(label)
			w.SetUncheckedStyle(field)
			w.SetCheckedStyle(field)
			w.SetActivatedStyle(active)
		}
	}
	for _, r := range a.renderers {
		r()
	}
	a.renderTabBar()
	a.renderTitles()
	a.renderStatus()
	a.renderResponse()
	if a.tree.GetRoot() != nil {
		a.rebuildTree(nil)
	}
	a.rebuildHistory(nil)
	if a.authFields != nil {
		prev := a.loading
		a.loading = true
		a.rebuildAuthFields()
		a.loading = prev
	}
}
