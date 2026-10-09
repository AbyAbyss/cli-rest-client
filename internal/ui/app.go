// Package ui implements the terminal user interface.
package ui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/clipboard"
	"github.com/AbyAbyss/cli-rest-client/internal/curl"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
)

// BuildInfo is shown in the Settings tab.
type BuildInfo struct {
	Version   string
	BuildTime string
}

// tab is one entry in the request tab bar.
type tab struct {
	name  string
	page  tview.Primitive
	focus func() []tview.Primitive
	// wide tabs hide the response pane.
	wide bool
}

// App is the running user interface.
type App struct {
	tv    *tview.Application
	pages *tview.Pages
	ws    *models.Workspace
	store *storage.Store
	info  BuildInfo
	theme *Theme

	// req is the request being edited in the builder. linked points at the
	// saved request it was loaded from, or is nil for an unsaved request.
	req     models.Request
	linked  *models.Request
	loading bool

	// Layout.
	main       *tview.Flex
	rightCol   *tview.Flex
	contentRow *tview.Flex
	tree       *tview.TreeView
	// Sidebar: Collections tree or History list.
	sideBar          *tview.TextView
	sidePages        *tview.Pages
	sideView         string
	historyView      *tview.TreeView
	history          *models.History
	historyFilter    string
	historyCollapsed map[string]bool
	methodDrop       *tview.DropDown
	envDrop          *tview.DropDown
	urlInput         *tview.InputField
	sendBtn          *tview.Button
	tabBar           *tview.TextView
	tabPages         *tview.Pages
	tabs             []*tab
	activeTab        int
	response         *tview.TextView
	statusMsg        *tview.TextView
	statusKeys       *tview.TextView

	// Tab editors.
	paramsArea, headersArea, bodyArea, preArea, testsArea, varsArea *tview.TextArea
	varsTarget                                                      *tview.DropDown
	envButtons                                                      []*tview.Button
	editingEnv                                                      string
	lastImportDir                                                   string
	// screen is captured while drawing, for the OSC 52 clipboard fallback.
	screen tcell.Screen
	// copier puts text on the clipboard (replaced in tests).
	copier                                            func(text string) (string, error)
	bodyType                                          *tview.DropDown
	authType, authIn                                  *tview.DropDown
	authUser, authPass, authToken, authKey, authValue *tview.InputField
	authFields                                        *tview.Flex
	themeDrop                                         *tview.DropDown
	timeoutInput                                      *tview.InputField
	redirectsBox, insecureBox                         *tview.Checkbox
	settingsInfo                                      *tview.TextView

	// Theming: every primitive registered here is restyled on theme change,
	// renderers regenerate text that embeds color tags.
	themed    []tview.Primitive
	bordered  []borderBox
	renderers []func()

	// Request execution.
	result  *sendResult
	sending bool
	cancel  context.CancelFunc

	dialogs   []dialog
	collapsed map[*models.Collection]bool

	status      string
	statusLevel int
}

type borderBox interface {
	tview.Primitive
	SetBorderColor(tcell.Color) *tview.Box
}

type dialog struct {
	name string
	prev tview.Primitive
}

// New builds the UI for ws. Changes are persisted through store.
func New(ws *models.Workspace, store *storage.Store, info BuildInfo) *App {
	theme, _ := themeByName(ws.Settings.Theme)
	a := &App{
		tv:        tview.NewApplication(),
		ws:        ws,
		store:     store,
		info:      info,
		theme:     theme,
		collapsed: map[*models.Collection]bool{},

		history:          &models.History{},
		historyCollapsed: map[string]bool{},
	}
	var historyErr error
	if store != nil {
		a.history, historyErr = store.LoadHistory()
	}
	a.setGlobalStyles()
	a.build()
	a.restoreDraft()
	a.refreshEnvPicker()
	a.applyTheme()
	a.rebuildTree(a.linked)
	a.renderResponse()
	a.setStatus(levelInfo, "Ready. Press F1 for help.")
	if historyErr != nil {
		a.setStatus(levelWarning, "History not loaded: "+historyErr.Error())
	}
	return a
}

// Application exposes the tview application (used by tests).
func (a *App) Application() *tview.Application { return a.tv }

// Run starts the event loop and blocks until the user quits.
func (a *App) Run() error {
	a.tv.EnableMouse(true)
	a.tv.EnablePaste(true)
	return a.tv.Run()
}

func (a *App) build() {
	a.buildTree()
	a.buildRequestBar()
	a.buildEnvPicker()
	a.buildTabs()
	a.buildResponse()
	a.buildStatusBar()

	a.contentRow = tview.NewFlex().
		AddItem(a.tabPages, 0, 1, false).
		AddItem(a.response, 0, 1, false)

	a.rightCol = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewFlex().
			AddItem(a.methodDrop, 13, 0, false).
			AddItem(&urlField{InputField: a.urlInput, onPaste: func(text string) bool {
				if !curl.LooksLikeCurl(text) {
					return false
				}
				a.importCurl(text, true)
				return true
			}}, 0, 1, true).
			AddItem(a.envDrop, 22, 0, false).
			AddItem(a.sendBtn, 10, 0, false), 3, 0, true).
		AddItem(a.tabBar, 1, 0, false).
		AddItem(a.contentRow, 0, 1, false)

	body := tview.NewFlex().
		AddItem(a.buildSidebar(), 34, 0, false).
		AddItem(a.rightCol, 0, 1, true)

	status := tview.NewFlex().
		AddItem(a.statusMsg, 0, 1, false).
		AddItem(a.statusKeys, 58, 0, false)

	a.main = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(body, 0, 1, true).
		AddItem(status, 1, 0, false)

	a.pages = tview.NewPages().AddPage("main", a.main, true, true)
	a.themed = append(a.themed, a.contentRow, a.rightCol, body, status, a.main, a.pages,
		a.rightCol.GetItem(0))
	a.unfocusable(a.statusMsg)
	a.unfocusable(a.statusKeys)

	a.tv.SetRoot(a.pages, true)
	a.tv.SetInputCapture(a.handleKey)
	a.copier = func(text string) (string, error) {
		var osc52 func([]byte)
		if s := a.screen; s != nil {
			osc52 = s.SetClipboard
		}
		return clipboard.Copy(text, osc52)
	}
	a.tv.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		a.screen = screen
		for _, b := range a.bordered {
			if b.HasFocus() {
				b.SetBorderColor(a.theme.Focus)
			} else {
				b.SetBorderColor(a.theme.Border)
			}
		}
		return false
	})
	a.switchTab(0)
	a.tv.SetFocus(a.urlInput)
}

// unfocusable makes a decorative primitive hand focus on to the current tab
// when it is clicked, so keyboard navigation never gets stuck on it.
func (a *App) unfocusable(p interface {
	SetFocusFunc(func()) *tview.Box
}) {
	p.SetFocusFunc(func() {
		if fs := a.tabs[a.activeTab].focus(); len(fs) > 0 {
			a.tv.SetFocus(fs[0])
			return
		}
		a.tv.SetFocus(a.urlInput)
	})
}

// focusables is the Tab-key cycle for the current tab.
func (a *App) focusables() []tview.Primitive {
	fs := []tview.Primitive{a.sidebar(), a.methodDrop, a.urlInput, a.envDrop, a.sendBtn}
	fs = append(fs, a.tabs[a.activeTab].focus()...)
	if !a.tabs[a.activeTab].wide {
		fs = append(fs, a.response)
	}
	return fs
}

func (a *App) focusIndex(p tview.Primitive) int {
	for i, f := range a.focusables() {
		if f == p {
			return i
		}
	}
	return -1
}

func (a *App) cycleFocus(delta int) {
	fs := a.focusables()
	i := a.focusIndex(a.tv.GetFocus())
	if i < 0 {
		i = 0
	} else {
		i = (i + delta + len(fs)) % len(fs)
	}
	a.tv.SetFocus(fs[i])
}

func (a *App) switchTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	focus := a.tv.GetFocus()
	wasInTab := false
	for _, p := range a.tabs[a.activeTab].focus() {
		if p == focus {
			wasInTab = true
		}
	}
	wasInResponse := focus == a.response

	a.activeTab = i
	a.tabPages.SwitchToPage(a.tabs[i].name)
	if a.tabs[i].wide {
		a.contentRow.ResizeItem(a.response, 0, 0)
	} else {
		a.contentRow.ResizeItem(a.response, 0, 1)
	}
	a.renderTabBar()

	if wasInTab || (wasInResponse && a.tabs[i].wide) {
		if fs := a.tabs[i].focus(); len(fs) > 0 {
			a.tv.SetFocus(fs[0])
		} else {
			a.tv.SetFocus(a.urlInput)
		}
	}
}

func isTextEntry(p tview.Primitive) bool {
	switch p.(type) {
	case *tview.InputField, *tview.TextArea:
		return true
	}
	return false
}

func (a *App) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	key, mod := ev.Key(), ev.Modifiers()

	if key == tcell.KeyCtrlC || key == tcell.KeyCtrlQ {
		a.quit()
		return nil
	}
	// Dialogs handle their own keys.
	if len(a.dialogs) > 0 {
		return ev
	}

	focus := a.tv.GetFocus()
	inCycle := a.focusIndex(focus) >= 0
	typing := isTextEntry(focus)

	switch {
	case key == tcell.KeyCtrlR || key == tcell.KeyF5 || (key == tcell.KeyEnter && mod&tcell.ModCtrl != 0):
		a.send()
		return nil
	case key == tcell.KeyCtrlS:
		a.save()
		return nil
	case key == tcell.KeyRune && mod&tcell.ModAlt != 0 && (ev.Rune() == 's' || ev.Rune() == 'S'):
		a.saveAs()
		return nil
	case key == tcell.KeyF3:
		if a.sideView == sideHistory {
			a.showSidebar(sideCollections, true)
		} else {
			a.showSidebar(sideHistory, true)
		}
		return nil
	case key == tcell.KeyRune && mod&tcell.ModAlt != 0 && (ev.Rune() == 'h' || ev.Rune() == 'H'):
		a.showSidebar(sideHistory, true)
		return nil
	case key == tcell.KeyRune && mod&tcell.ModAlt != 0 && (ev.Rune() == 'c' || ev.Rune() == 'C'):
		a.showSidebar(sideCollections, true)
		return nil
	case key == tcell.KeyRune && mod&tcell.ModAlt != 0 && (ev.Rune() == 'e' || ev.Rune() == 'E'):
		a.cycleEnv()
		return nil
	case key == tcell.KeyCtrlO:
		a.importPostman()
		return nil
	case key == tcell.KeyCtrlN:
		a.newRequest()
		return nil
	case key == tcell.KeyCtrlG || key == tcell.KeyF4:
		a.showCurl()
		return nil
	case key == tcell.KeyCtrlP:
		a.formatBody()
		return nil
	case key == tcell.KeyF1 || (key == tcell.KeyRune && ev.Rune() == '?' && !typing && inCycle):
		a.showHelp()
		return nil
	case key == tcell.KeyEsc:
		if a.sending {
			a.cancelRequest()
			return nil
		}
		if focus == a.urlInput {
			return ev // closes autocomplete first; the done func handles the rest
		}
		if inCycle && focus != a.sidebar() {
			a.tv.SetFocus(a.sidebar())
			return nil
		}
		return ev
	case key == tcell.KeyRune && ev.Rune() >= '1' && ev.Rune() <= '9':
		idx := int(ev.Rune() - '1')
		if idx < len(a.tabs) && (mod&tcell.ModAlt != 0 || (inCycle && !typing && mod == 0)) {
			a.switchTab(idx)
			return nil
		}
		return ev
	case key == tcell.KeyTab && inCycle:
		a.cycleFocus(1)
		return nil
	case key == tcell.KeyBacktab && inCycle:
		a.cycleFocus(-1)
		return nil
	}
	return ev
}

// quit saves the workspace (including the unsaved builder state) and exits.
func (a *App) quit() {
	if a.cancel != nil {
		a.cancel()
	}
	a.saveDraft()
	a.tv.Stop()
}

// persist writes the workspace to disk and reports failures in the status bar.
func (a *App) persist() bool {
	if a.store == nil {
		return true
	}
	if err := a.store.Save(a.ws); err != nil {
		a.setStatus(levelError, "Could not save workspace: "+err.Error())
		return false
	}
	return true
}

func (a *App) saveDraft() {
	d := &models.Draft{Request: a.req.Clone(), Index: -1}
	if a.linked != nil {
		if folders, index, ok := a.ws.Location(a.linked); ok {
			d.Folders, d.Index = folders, index
		}
	}
	a.ws.Draft = d
	a.persist()
}

func (a *App) restoreDraft() {
	if d := a.ws.Draft; d != nil {
		var linked *models.Request
		if d.Index >= 0 {
			linked = a.ws.AtLocation(d.Folders, d.Index)
		}
		a.loadIntoBuilder(d.Request, linked)
		return
	}
	// First run: open the first saved request so there is something to send.
	var first *models.Request
	a.ws.WalkRequests(func(_ []*models.Collection, r *models.Request) {
		if first == nil {
			first = r
		}
	})
	if first != nil {
		a.loadIntoBuilder(*first, first)
		return
	}
	a.loadIntoBuilder(models.NewRequest(""), nil)
}

const (
	levelInfo = iota
	levelSuccess
	levelWarning
	levelError
)

func (a *App) setStatus(level int, msg string) {
	a.status, a.statusLevel = msg, level
	a.renderStatus()
}

func (a *App) renderStatus() {
	color := a.theme.HexMuted
	switch a.statusLevel {
	case levelSuccess:
		color = a.theme.HexSuccess
	case levelWarning:
		color = a.theme.HexWarning
	case levelError:
		color = a.theme.HexError
	}
	a.statusMsg.SetText(fmt.Sprintf(" [%s]%s", color, tview.Escape(a.status)))
	k := func(key, label string) string {
		return fmt.Sprintf("[%s::b]%s[-:-:-] [%s]%s[-]  ", a.theme.HexAccent, key, a.theme.HexMuted, label)
	}
	a.statusKeys.SetText(k("^R", "send") + k("^S", "save") + k("^N", "new") + k("Tab", "focus") + k("F1", "help") + k("^Q", "quit"))
}
