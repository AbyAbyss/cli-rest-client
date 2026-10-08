//go:build screenshots

// Screenshot generator for the README. It drives the real UI on a simulated
// terminal and writes each frame as an HTML page with the exact colors the
// app drew; scripts/screenshots.cjs turns those pages into PNGs.
//
//	make screenshots
package ui

import (
	"bytes"
	"fmt"
	"html"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/cli"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/testutil"
)

const shotW, shotH = 140, 38

// localHTTPBin serves the httpbin clone on localhost:8080 so the URLs in the
// pictures look like a normal local dev setup.
func localHTTPBin(t *testing.T) (string, func()) {
	srv := testutil.NewHTTPBin()
	l, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		return srv.URL, srv.Close // port busy: fall back to a random one
	}
	srv.Close()
	srv = httptest.NewUnstartedServer(testutil.NewHTTPBin().Config.Handler)
	srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	return "http://localhost:8080", srv.Close
}

func TestScreenshots(t *testing.T) {
	dir := os.Getenv("SCREENSHOT_DIR")
	if dir == "" {
		t.Skip("set SCREENSHOT_DIR")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base, stop := localHTTPBin(t)
	defer stop()

	newApp := func(theme string) (*harness, *models.Workspace) {
		ws := storage.SampleWorkspace()
		ws.Environment("Local").Variables[0].Value = base
		ws.SetActive("Local")
		ws.Settings.Theme = theme
		h := start(t, ws)
		h.screen.SetSize(shotW, shotH)
		h.sync()
		return h, ws
	}
	open := func(h *harness, r *models.Request, tab int) {
		h.do(func() {
			h.a.loadIntoBuilder(*r, r)
			h.a.rebuildTree(r)
			h.a.switchTab(tab)
			h.a.tv.SetFocus(h.a.urlInput)
		})
	}
	send := func(h *harness) {
		h.key(tcell.KeyCtrlR, 0, tcell.ModCtrl)
		h.eventually("response", func() bool { return h.a.result != nil && h.a.result.resp != nil })
		h.do(func() { h.a.tv.SetFocus(h.a.tree) })
	}

	// 1. Main view: JSON body, response, tests.
	h, ws := newApp("Catppuccin Mocha")
	open(h, reqAt(t, ws, "User Service/Users/Create User"), 3)
	send(h)
	writeShot(t, h, dir, "main", "Term REST Client")

	// 1b. Request and response headers open, body folded.
	h.do(func() {
		h.a.ws.Settings.CollapsedSections = map[string]bool{secReqHeaders: false, secBody: true}
		h.a.renderResponse()
	})
	writeShot(t, h, dir, "headers", "Request and response headers")
	h.do(func() {
		h.a.ws.Settings.CollapsedSections = map[string]bool{secTests: true, secReqHeaders: true, secRespHeaders: true}
		h.a.renderResponse()
	})
	writeShot(t, h, dir, "folded", "Sections folded")
	h.do(func() {
		h.a.ws.Settings.CollapsedSections = nil
		h.a.renderResponse()
	})

	// 2. Query params and headers with variables.
	open(h, reqAt(t, ws, "User Service/Lookup/Query Params"), 0)
	send(h)
	writeShot(t, h, dir, "params", "Query params")

	// 3. Auth tab.
	open(h, reqAt(t, ws, "Auth API/Bearer Token"), 1)
	send(h)
	writeShot(t, h, dir, "auth", "Bearer token auth")

	// 4. Pre-request script + tests, including a failing assertion.
	charge := reqAt(t, ws, "Payment Gateway/Charges/Charge")
	charge.Tests += "\nstatus == 201"
	open(h, charge, 5)
	send(h)
	writeShot(t, h, dir, "tests", "Pre-request script and tests")

	// 5. Help overlay.
	h.key(tcell.KeyF1, 0, 0)
	writeShot(t, h, dir, "help", "Keyboard shortcuts")
	h.key(tcell.KeyEsc, 0, 0)

	// 6. Save dialog for a new request.
	h.key(tcell.KeyCtrlN, 0, tcell.ModCtrl)
	h.typeText(base + "/anything/orders?limit=20")
	h.key(tcell.KeyRune, 's', tcell.ModAlt)
	writeShot(t, h, dir, "save", "Save request")
	h.key(tcell.KeyEsc, 0, 0)

	// 6b. Move dialog: reorganising requests into folders.
	h.do(func() {
		h.a.rebuildTree(reqAt(t, ws, "User Service/Lookup/Query Params"))
		h.a.tv.SetFocus(h.a.tree)
	})
	h.key(tcell.KeyRune, 'm', 0)
	h.do(func() {
		d := h.a.tv.GetFocus().(*tview.DropDown)
		for i := 0; i < d.GetOptionCount(); i++ {
			d.SetCurrentOption(i)
			if _, l := d.GetCurrentOption(); l == "User Service / Users" {
				break
			}
		}
	})
	writeShot(t, h, dir, "move", "Move a request")
	h.key(tcell.KeyEsc, 0, 0)

	// 6c. History in the sidebar.
	h.do(func() { h.a.loadIntoBuilder(models.NewRequest(""), nil) })
	h.key(tcell.KeyRune, 'h', tcell.ModAlt)
	h.key(tcell.KeyDown, 0, 0)
	h.key(tcell.KeyEnter, 0, 0) // open it: request + the response it got
	h.do(func() { h.a.tv.SetFocus(h.a.historyView) })
	h.key(tcell.KeyUp, 0, 0)
	h.key(tcell.KeyDown, 0, 0)
	writeShot(t, h, dir, "history", "Request history")
	h.key(tcell.KeyRune, 'c', tcell.ModAlt)

	// 6d. Environments in the Variables tab.
	h.do(func() {
		h.a.ws.Environments = append(h.a.ws.Environments, &models.Environment{Name: "Staging", Variables: []models.KeyValue{
			{Key: "baseUrl", Value: "https://staging.api.example.com"},
			{Key: "token", Value: "{{$uuid}}"},
		}})
		h.a.refreshEnvPicker()
		h.a.editingEnv = "Local"
		h.a.switchTab(6)
		h.a.refreshVariablesTab()
		h.a.tv.SetFocus(h.a.varsTarget)
	})
	writeShot(t, h, dir, "environments", "Environments")
	h.do(func() { h.a.switchTab(0) })

	// 6e. Import from Postman: the imported tree and the notes dialog.
	imp, _ := filepath.Abs("../postman/testdata/shop.postman_collection.json")
	h.do(func() {
		for _, c := range h.a.ws.Collections[:3] {
			h.a.collapsed[c] = true
		}
		h.a.tv.SetFocus(h.a.tree)
	})
	h.key(tcell.KeyCtrlO, 0, tcell.ModCtrl)
	h.do(func() { h.a.tv.GetFocus().(*tview.InputField).SetText(imp) })
	h.key(tcell.KeyEnter, 0, 0)
	writeShot(t, h, dir, "import", "Import from Postman")
	h.key(tcell.KeyEsc, 0, 0)

	// 7. Settings in another theme.
	g, gws := newApp("Gruvbox Dark")
	open(g, reqAt(t, gws, "User Service/Lookup/Get JSON"), 7)
	g.do(func() { g.a.tv.SetFocus(g.a.themeDrop) })
	writeShot(t, g, dir, "settings", "Settings (Gruvbox Dark)")

	// 8. Light theme main view.
	l, lws := newApp("Light")
	open(l, reqAt(t, lws, "User Service/Users/Update User"), 3)
	send(l)
	writeShot(t, l, dir, "light", "Light theme")

	// 9. Command-line mode.
	cws := storage.SampleWorkspace()
	cws.Environment("Local").Variables[0].Value = base
	cws.SetActive("Local")
	var out bytes.Buffer
	cli.Run(&out, &out, cws, nil, []string{"User Service", "Payment Gateway"})
	writeCLIShot(t, dir, "cli", `term-rest-client run "User Service" "Payment Gateway"`, out.String(), themes[0])
}

func hexColor(c tcell.Color, fallback tcell.Color) string {
	if c == tcell.ColorDefault || c.Hex() < 0 {
		c = fallback
	}
	return fmt.Sprintf("#%06x", c.Hex())
}

func writeShot(t *testing.T, h *harness, dir, name, title string) {
	t.Helper()
	var body strings.Builder
	var bg string
	h.do(func() {
		th := h.a.theme
		bg = hexColor(th.Background, th.Background)
		h.a.tv.ForceDraw()
		cells, w, hgt := h.screen.GetContents()
		cx, cy, cursor := h.screen.GetCursor()
		for y := 0; y < hgt; y++ {
			body.WriteString(`<div class="r">`)
			prev := ""
			for x := 0; x < w; x++ {
				c := cells[y*w+x]
				fg, cbg, attr := c.Style.Decompose()
				f, b := hexColor(fg, th.Text), hexColor(cbg, th.Background)
				if attr&tcell.AttrReverse != 0 || (cursor && x == cx && y == cy) {
					f, b = b, f
				}
				css := fmt.Sprintf("color:%s;background:%s", f, b)
				if attr&tcell.AttrBold != 0 {
					css += ";font-weight:bold"
				}
				if attr&tcell.AttrUnderline != 0 {
					css += ";text-decoration:underline"
				}
				if css != prev {
					if prev != "" {
						body.WriteString("</span>")
					}
					body.WriteString(`<span style="` + css + `">`)
					prev = css
				}
				ch := " "
				if len(c.Runes) > 0 && c.Runes[0] != 0 {
					ch = string(c.Runes)
				}
				body.WriteString(html.EscapeString(ch))
			}
			body.WriteString("</span></div>")
		}
	})
	writePage(t, dir, name, title, bg, body.String())
}

func writeCLIShot(t *testing.T, dir, name, cmd, output string, th *Theme) {
	t.Helper()
	var body strings.Builder
	fmt.Fprintf(&body, `<span style="color:%s">$ </span><span style="color:%s">%s</span>`+"\n",
		th.HexSuccess, th.HexText, html.EscapeString(cmd))
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		color := th.HexText
		switch {
		case strings.Contains(line, "✓"):
			color = th.HexSuccess
		case strings.Contains(line, "✗"), strings.Contains(line, "error"):
			color = th.HexError
		case strings.Contains(line, "→"):
			color = th.HexInfo
		case !strings.HasPrefix(line, " "):
			color = th.HexAccent
		}
		fmt.Fprintf(&body, `<span style="color:%s">%s</span>`+"\n", color, html.EscapeString(line))
	}
	fmt.Fprintf(&body, `<span style="color:%s">$ </span>`+"\n", th.HexSuccess)
	writePage(t, dir, name, "zsh", hexColor(th.Background, th.Background), body.String())
}

func writePage(t *testing.T, dir, name, title, bg, screen string) {
	t.Helper()
	page := `<!doctype html><html><head><meta charset="utf-8"><style>
body{margin:0;padding:28px;background:#d9dce3;display:inline-block}
.win{border-radius:10px;overflow:hidden;box-shadow:0 18px 40px rgba(0,0,0,.35);background:` + bg + `;display:inline-block}
.bar{height:30px;background:#2b2b36;display:flex;align-items:center;padding:0 12px;gap:8px;color:#b8bccb;font:13px -apple-system,Helvetica,Arial,sans-serif}
.dot{width:12px;height:12px;border-radius:50%}
.title{flex:1;text-align:center;margin-right:52px}
pre{margin:0;padding:8px 10px;font:14px/16px "DejaVu Sans Mono",Menlo,monospace;white-space:pre}
.r{height:16px}.r span{display:inline-block;height:16px;vertical-align:top}
</style></head><body><div class="win" id="shot"><div class="bar">
<span class="dot" style="background:#ff5f57"></span><span class="dot" style="background:#febc2e"></span><span class="dot" style="background:#28c840"></span>
<span class="title">` + html.EscapeString(title) + `</span></div><pre>` + screen + `</pre></div></body></html>`
	if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
}
