package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/curl"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/vars"
)

var (
	authLabels = []string{"No Auth", "Basic Auth", "Bearer Token", "API Key"}
	bodyLabels = []string{"None", "JSON", "Text", "XML", "Form (urlencoded)", "GraphQL"}
	authInOpts = []string{"Header", "Query Params"}
)

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return 0
}

// ---------- widget helpers ----------

func (a *App) newArea(title, placeholder string) *tview.TextArea {
	ta := tview.NewTextArea()
	ta.SetBorder(true).SetTitle(" "+title+" ").SetBorderPadding(0, 0, 1, 1)
	ta.SetPlaceholder(placeholder)
	ta.SetWordWrap(false)
	a.themed = append(a.themed, ta)
	a.bordered = append(a.bordered, ta)
	return ta
}

func (a *App) newInput(label string) *tview.InputField {
	in := tview.NewInputField().SetLabel(label)
	a.themed = append(a.themed, in)
	return in
}

func (a *App) newDrop(label string, options []string) *tview.DropDown {
	d := tview.NewDropDown().SetLabel(label).SetOptions(options, nil)
	d.SetTextOptions(" ", " ", "", "", "")
	a.themed = append(a.themed, d)
	return d
}

// newHint is a muted help text whose content is regenerated on theme change.
func (a *App) newHint(text func(t *Theme) string) *tview.TextView {
	tv := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	render := func() { tv.SetText(text(a.theme)) }
	a.renderers = append(a.renderers, render)
	a.themed = append(a.themed, tv)
	a.unfocusable(tv)
	render()
	return tv
}

func hintLine(t *Theme, parts ...string) string {
	for i := range parts {
		parts[i] = tview.Escape(parts[i])
	}
	return fmt.Sprintf("[%s]%s", t.HexMuted, strings.Join(parts, "  ·  "))
}

func (a *App) newFlex(dir int) *tview.Flex {
	f := tview.NewFlex().SetDirection(dir)
	a.themed = append(a.themed, f)
	return f
}

// ---------- request bar ----------

func (a *App) buildRequestBar() {
	a.methodDrop = tview.NewDropDown().SetOptions(append(append([]string(nil), models.Methods...), "WS"), func(text string, _ int) {
		if a.loading {
			return
		}
		a.setRequestType(text)
		a.layoutBody()
		a.requestChanged()
	})
	a.methodDrop.SetTextOptions(" ", " ", "", "", "")
	a.methodDrop.SetBorder(true).SetTitle(" Method ")
	a.themed = append(a.themed, a.methodDrop)
	a.bordered = append(a.bordered, a.methodDrop)

	a.urlInput = tview.NewInputField().SetPlaceholder("https://api.example.com/users/{{id}}")
	a.urlInput.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	a.urlInput.SetChangedFunc(func(text string) {
		if a.loading {
			return
		}
		a.req.URL = text
		a.requestChanged()
	})
	a.urlInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			if text := a.urlInput.GetText(); curl.LooksLikeCurl(text) {
				a.importCurl(text, true)
				return
			}
			a.send()
		case tcell.KeyEscape:
			a.tv.SetFocus(a.sidebar())
		}
	})
	a.urlInput.SetAutocompleteFunc(a.variableCompletions)
	a.urlInput.SetAutocompletedFunc(func(text string, _ int, source int) bool {
		if source != tview.AutocompletedNavigate {
			a.urlInput.SetText(text)
		}
		return source == tview.AutocompletedEnter || source == tview.AutocompletedClick
	})
	a.themed = append(a.themed, a.urlInput)
	a.bordered = append(a.bordered, a.urlInput)

	a.sendBtn = tview.NewButton("SEND").SetSelectedFunc(a.send)
	a.themed = append(a.themed, a.sendBtn)
}

// variableCompletions suggests {{variable}} names while typing "{{" in the URL.
func (a *App) variableCompletions(text string) []string {
	open := strings.LastIndex(text, "{{")
	if open < 0 || strings.Contains(text[open:], "}}") {
		return nil
	}
	prefix := text[open+2:]
	var out []string
	names := append([]string{}, vars.Dynamic...)
	for k := range a.ws.VariableMap() {
		names = append(names, k)
	}
	sort.Strings(names[len(vars.Dynamic):])
	for _, n := range names {
		if strings.HasPrefix(strings.ToLower(n), strings.ToLower(prefix)) {
			out = append(out, text[:open]+"{{"+n+"}}")
		}
	}
	return out
}

// ---------- tabs ----------

func (a *App) buildTabs() {
	a.tabBar = tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false)
	a.tabBar.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) == 0 {
			return
		}
		if i, err := strconv.Atoi(strings.TrimPrefix(added[0], "tab")); err == nil {
			a.switchTab(i)
		}
		a.tabBar.Highlight()
	})
	a.themed = append(a.themed, a.tabBar)
	a.unfocusable(a.tabBar)

	a.tabPages = tview.NewPages()
	a.themed = append(a.themed, a.tabPages)

	a.tabs = []*tab{
		a.buildParamsTab(),
		a.buildAuthTab(),
		a.buildHeadersTab(),
		a.buildBodyTab(),
		a.buildPreRequestTab(),
		a.buildTestsTab(),
		a.buildVariablesTab(),
		a.buildSettingsTab(),
	}
	for i, t := range a.tabs {
		a.tabPages.AddPage(t.name, t.page, true, i == 0)
	}
}

func (a *App) renderTabBar() {
	if a.tabBar == nil || len(a.tabs) == 0 {
		return
	}
	enabled := func(kvs []models.KeyValue) int {
		n := 0
		for _, kv := range kvs {
			if !kv.Disabled && kv.Key != "" {
				n++
			}
		}
		return n
	}
	badge := map[string]string{}
	if n := enabled(a.req.Params); n > 0 {
		badge["Params"] = fmt.Sprintf(" (%d)", n)
	}
	if n := enabled(a.req.Headers); n > 0 {
		badge["Headers"] = fmt.Sprintf(" (%d)", n)
	}
	if a.req.Auth.Type != models.AuthNone {
		badge["Auth"] = " •"
	}
	if a.req.BodyType != models.BodyNone {
		badge["Body"] = " •"
	}
	if strings.TrimSpace(a.req.PreRequest) != "" {
		badge["Pre-request"] = " •"
	}
	if strings.TrimSpace(a.req.Tests) != "" {
		badge["Tests"] = " •"
	}

	var sb strings.Builder
	for i, t := range a.tabs {
		label := fmt.Sprintf(" %d %s%s ", i+1, t.name, badge[t.name])
		if i == a.activeTab {
			fmt.Fprintf(&sb, `["tab%d"][%s:%s:b]%s[-:-:-][""]`, i, a.theme.HexText, hexOf(a.theme.Selection), label)
		} else {
			fmt.Fprintf(&sb, `["tab%d"][%s]%s[-][""]`, i, a.theme.HexMuted, label)
		}
		sb.WriteString(" ")
	}
	a.tabBar.SetText(sb.String())
}

func hexOf(c tcell.Color) string {
	return fmt.Sprintf("#%06x", c.Hex())
}

func (a *App) kvTab(name, title, placeholder string, area **tview.TextArea, sep string, set func([]models.KeyValue), hint func(t *Theme) string) *tab {
	*area = a.newArea(title, placeholder)
	ta := *area
	ta.SetChangedFunc(func() {
		if a.loading {
			return
		}
		set(models.ParseKV(ta.GetText(), sep))
		a.requestChanged()
	})
	page := a.newFlex(tview.FlexRow).
		AddItem(ta, 0, 1, false).
		AddItem(a.newHint(hint), 2, 0, false)
	return &tab{name: name, page: page, focus: func() []tview.Primitive { return []tview.Primitive{ta} }}
}

func (a *App) buildParamsTab() *tab {
	return a.kvTab("Params", "Query Params", "# example:\n# page=1\n# search={{term}}", &a.paramsArea, "=",
		func(kv []models.KeyValue) { a.req.Params = kv },
		func(t *Theme) string {
			return hintLine(t, "One key=value per line", "Appended to the URL query", "# disables a line", "{{var}} allowed")
		})
}

func (a *App) buildHeadersTab() *tab {
	return a.kvTab("Headers", "Headers", "# example:\n# Accept: application/json\n# X-Request-Id: {{$uuid}}", &a.headersArea, ":",
		func(kv []models.KeyValue) { a.req.Headers = kv },
		func(t *Theme) string {
			return hintLine(t, "One Name: value per line", "# disables a line", "Content-Type is set from the body type unless given here")
		})
}

func (a *App) buildAuthTab() *tab {
	a.authType = a.newDrop("Type       ", authLabels)
	a.authType.SetSelectedFunc(func(_ string, i int) {
		if a.loading {
			return
		}
		a.req.Auth.Type = models.AuthTypes[i]
		a.rebuildAuthFields()
		a.requestChanged()
	})
	field := func(label string, set func(string)) *tview.InputField {
		in := a.newInput(label)
		in.SetChangedFunc(func(text string) {
			if a.loading {
				return
			}
			set(text)
			a.requestChanged()
		})
		return in
	}
	a.authUser = field("Username   ", func(s string) { a.req.Auth.Username = s })
	a.authPass = field("Password   ", func(s string) { a.req.Auth.Password = s })
	a.authPass.SetMaskCharacter('*')
	a.authToken = field("Token      ", func(s string) { a.req.Auth.Token = s })
	a.authKey = field("Key        ", func(s string) { a.req.Auth.Key = s })
	a.authValue = field("Value      ", func(s string) { a.req.Auth.Value = s })
	a.authIn = a.newDrop("Add to     ", authInOpts)
	a.authIn.SetSelectedFunc(func(_ string, i int) {
		if a.loading {
			return
		}
		a.req.Auth.In = []string{"header", "query"}[i]
		a.requestChanged()
	})

	a.authFields = a.newFlex(tview.FlexRow)
	page := a.newFlex(tview.FlexRow).
		AddItem(a.authType, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(a.authFields, 0, 1, false).
		AddItem(a.newHint(func(t *Theme) string {
			return hintLine(t, "Fields accept {{variables}}", "Basic/Bearer set the Authorization header")
		}), 2, 0, false)
	page.SetBorder(true).SetTitle(" Authorization ").SetBorderPadding(1, 0, 1, 1)
	a.bordered = append(a.bordered, page)

	return &tab{name: "Auth", page: page, focus: func() []tview.Primitive {
		return append([]tview.Primitive{a.authType}, a.authFieldList()...)
	}}
}

func (a *App) authFieldList() []tview.Primitive {
	switch a.req.Auth.Type {
	case models.AuthBasic:
		return []tview.Primitive{a.authUser, a.authPass}
	case models.AuthBearer:
		return []tview.Primitive{a.authToken}
	case models.AuthAPIKey:
		return []tview.Primitive{a.authKey, a.authValue, a.authIn}
	}
	return nil
}

func (a *App) rebuildAuthFields() {
	focused := a.tv.GetFocus()
	a.authFields.Clear()
	for _, p := range a.authFieldList() {
		a.authFields.AddItem(p, 1, 0, false).AddItem(nil, 1, 0, false)
	}
	if a.req.Auth.Type == models.AuthNone {
		a.authFields.AddItem(a.newHintOnce("This request does not send credentials."), 1, 0, false)
	}
	// If a field that just disappeared had focus, go back to the type picker.
	if isTextEntry(focused) && a.focusIndex(focused) < 0 && a.activeTab == 1 {
		a.tv.SetFocus(a.authType)
	}
}

func (a *App) newHintOnce(text string) *tview.TextView {
	tv := tview.NewTextView().SetDynamicColors(true)
	tv.SetText(fmt.Sprintf("[%s]%s", a.theme.HexMuted, tview.Escape(text)))
	tv.SetBackgroundColor(a.theme.Background)
	return tv
}

func (a *App) buildBodyTab() *tab {
	a.bodyType = a.newDrop("Body type ", bodyLabels)
	a.bodyType.SetSelectedFunc(func(_ string, i int) {
		if a.loading {
			return
		}
		a.req.BodyType = models.BodyTypes[i]
		a.layoutBody()
		a.requestChanged()
	})
	a.bodyArea = a.newArea("Body", "{\n  \"name\": \"{{name}}\"\n}")
	a.bodyArea.SetChangedFunc(func() {
		if a.loading {
			return
		}
		a.req.Body = a.bodyArea.GetText()
		a.requestChanged()
	})
	a.gqlVarsArea = a.newArea("Variables · JSON", "{\n  \"code\": \"{{country}}\"\n}")
	a.gqlVarsArea.SetChangedFunc(func() {
		if a.loading {
			return
		}
		a.req.GraphQLVariables = a.gqlVarsArea.GetText()
		a.requestChanged()
	})
	a.bodyEditors = a.newFlex(tview.FlexColumn)
	a.layoutBody()
	page := a.newFlex(tview.FlexRow).
		AddItem(a.bodyType, 1, 0, false).
		AddItem(a.bodyEditors, 0, 1, false).
		AddItem(a.newHint(func(t *Theme) string {
			if a.req.Type == models.TypeWebSocket {
				return hintLine(t, "WebSocket: Ctrl+R connects, then sends this message", "Esc disconnects", "Tests run on the received messages when it closes")
			}
			if a.req.BodyType == models.BodyGraphQL {
				return hintLine(t, "F6 browses the schema and writes a query for you", "Ctrl+P formats the variables", "GET sends the query in the URL")
			}
			return hintLine(t, "Ctrl+P formats JSON", "Form: key=value per line", "{{var}} allowed")
		}), 2, 0, false)
	return &tab{name: "Body", page: page, focus: func() []tview.Primitive {
		if a.req.BodyType == models.BodyGraphQL {
			return []tview.Primitive{a.bodyType, a.bodyArea, a.gqlVarsArea}
		}
		return []tview.Primitive{a.bodyType, a.bodyArea}
	}}
}

// layoutBody shows the query and variables editors side by side for
// GraphQL, and the single body editor otherwise.
func (a *App) layoutBody() {
	if a.bodyEditors == nil {
		return
	}
	focused := a.tv.GetFocus()
	a.bodyEditors.Clear()
	a.bodyEditors.AddItem(a.bodyArea, 0, 3, false)
	if a.req.BodyType == models.BodyGraphQL {
		a.bodyEditors.AddItem(a.gqlVarsArea, 0, 2, false)
	} else if focused == a.gqlVarsArea {
		a.tv.SetFocus(a.bodyType)
	}
	for _, r := range a.renderers {
		r() // the hint line depends on the body type
	}
}

func (a *App) buildPreRequestTab() *tab {
	a.preArea = a.newArea("Pre-request Script", "# Runs before the request is sent\nset requestId = {{$uuid}}\nset ts = {{$timestamp}}")
	a.preArea.SetChangedFunc(func() {
		if a.loading {
			return
		}
		a.req.PreRequest = a.preArea.GetText()
		a.requestChanged()
	})
	page := a.newFlex(tview.FlexRow).
		AddItem(a.preArea, 0, 1, false).
		AddItem(a.newHint(func(t *Theme) string {
			return hintLine(t, "set name = value", "unset name", "Values may use {{variables}}", "Changes are saved to Variables")
		}), 2, 0, false)
	return &tab{name: "Pre-request", page: page, focus: func() []tview.Primitive { return []tview.Primitive{a.preArea} }}
}

func (a *App) buildTestsTab() *tab {
	a.testsArea = a.newArea("Tests", "status == 200\ntime < 1000\nheader Content-Type contains json\njson.data.id exists\nset token = json.token")
	a.testsArea.SetChangedFunc(func() {
		if a.loading {
			return
		}
		a.req.Tests = a.testsArea.GetText()
		a.requestChanged()
	})
	page := a.newFlex(tview.FlexRow).
		AddItem(a.testsArea, 0, 1, false).
		AddItem(a.newHint(func(t *Theme) string {
			return hintLine(t,
				"Subjects: status, time (ms), size, body, header <Name>, json.path[0].key",
				"Operators: == != < <= > >= contains matches exists (prefix ! to negate)",
				"set var = <subject> captures a value")
		}), 3, 0, false)
	return &tab{name: "Tests", page: page, focus: func() []tview.Primitive { return []tview.Primitive{a.testsArea} }}
}

func (a *App) buildSettingsTab() *tab {
	s := &a.ws.Settings
	_, themeIdx := themeByName(s.Theme)
	a.themeDrop = a.newDrop("Theme              ", ThemeNames())
	a.themeDrop.SetCurrentOption(themeIdx)
	a.themeDrop.SetSelectedFunc(func(text string, _ int) {
		if s.Theme == text {
			return
		}
		s.Theme = text
		a.theme, _ = themeByName(text)
		a.applyTheme()
		a.persist()
	})

	a.timeoutInput = a.newInput("Timeout (seconds)  ")
	a.timeoutInput.SetText(strconv.Itoa(s.TimeoutSeconds))
	a.timeoutInput.SetFieldWidth(8)
	a.timeoutInput.SetAcceptanceFunc(tview.InputFieldInteger)
	a.timeoutInput.SetChangedFunc(func(text string) {
		if n, err := strconv.Atoi(text); err == nil && n > 0 {
			s.TimeoutSeconds = n
			a.persist()
		}
	})

	a.redirectsBox = tview.NewCheckbox().SetLabel("Follow redirects   ").SetChecked(!s.DisableRedirects)
	a.redirectsBox.SetChangedFunc(func(checked bool) {
		s.DisableRedirects = !checked
		a.persist()
	})
	a.insecureBox = tview.NewCheckbox().SetLabel("Verify TLS certs   ").SetChecked(!s.InsecureSkipVerify)
	a.insecureBox.SetChangedFunc(func(checked bool) {
		s.InsecureSkipVerify = !checked
		a.persist()
		if !checked {
			a.setStatus(levelWarning, "TLS certificate verification is off")
		}
	})
	historyBox := tview.NewCheckbox().SetLabel("Record history     ").SetChecked(!s.DisableHistory)
	historyBox.SetChangedFunc(func(checked bool) {
		s.DisableHistory = !checked
		a.persist()
	})
	a.themed = append(a.themed, a.redirectsBox, a.insecureBox, historyBox)

	a.settingsInfo = a.newHint(func(t *Theme) string {
		path := "(not saved)"
		if a.store != nil {
			path = a.store.Path
		}
		hist := "(not saved)"
		if a.store != nil {
			hist = a.store.HistoryPath()
		}
		return fmt.Sprintf("[%s]Version   [%s]%s\n[%s]Built     [%s]%s\n[%s]Data file [%s]%s\n[%s]History   [%s]%s",
			t.HexMuted, t.HexText, tview.Escape(a.info.Version),
			t.HexMuted, t.HexText, tview.Escape(a.info.BuildTime),
			t.HexMuted, t.HexText, tview.Escape(path),
			t.HexMuted, t.HexText, tview.Escape(hist))
	})

	keys := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true)
	keys.SetBorder(true).SetTitle(" Keyboard Shortcuts ").SetBorderPadding(0, 0, 1, 1)
	a.renderers = append(a.renderers, func() { keys.SetText(helpText(a.theme)) })
	a.themed = append(a.themed, keys)
	a.bordered = append(a.bordered, keys)

	page := a.newFlex(tview.FlexRow).
		AddItem(a.themeDrop, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(a.timeoutInput, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(a.redirectsBox, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(a.insecureBox, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(historyBox, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(a.settingsInfo, 4, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(keys, 0, 1, false)
	page.SetBorder(true).SetTitle(" Settings ").SetBorderPadding(1, 0, 1, 1)
	a.bordered = append(a.bordered, page)
	return &tab{name: "Settings", page: page, wide: true, focus: func() []tview.Primitive {
		return []tview.Primitive{a.themeDrop, a.timeoutInput, a.redirectsBox, a.insecureBox, historyBox, keys}
	}}
}

// ---------- builder state ----------

// loadIntoBuilder shows r in the editor. linked is the saved request it
// belongs to, or nil.
func (a *App) loadIntoBuilder(r models.Request, linked *models.Request) {
	r = r.Clone()
	r.Normalize()
	a.loading = true
	a.req = r
	a.linked = linked
	a.wsDisconnect() // a connection belongs to the request it was opened from
	if r.Type == models.TypeWebSocket {
		a.methodDrop.SetCurrentOption(len(models.Methods))
	} else {
		a.methodDrop.SetCurrentOption(indexOf(models.Methods, r.Method))
	}
	if indexOf(models.Methods, r.Method) == 0 && r.Method != "GET" {
		a.req.Method = "GET"
	}
	a.urlInput.SetText(r.URL)
	a.paramsArea.SetText(models.FormatKV(r.Params, "="), false)
	a.headersArea.SetText(models.FormatKV(r.Headers, ": "), false)
	a.authType.SetCurrentOption(indexOf(models.AuthTypes, r.Auth.Type))
	a.authUser.SetText(r.Auth.Username)
	a.authPass.SetText(r.Auth.Password)
	a.authToken.SetText(r.Auth.Token)
	a.authKey.SetText(r.Auth.Key)
	a.authValue.SetText(r.Auth.Value)
	if r.Auth.In == "query" {
		a.authIn.SetCurrentOption(1)
	} else {
		a.authIn.SetCurrentOption(0)
	}
	a.bodyType.SetCurrentOption(indexOf(models.BodyTypes, r.BodyType))
	a.bodyArea.SetText(r.Body, false)
	a.gqlVarsArea.SetText(r.GraphQLVariables, false)
	a.layoutBody()
	a.preArea.SetText(r.PreRequest, false)
	a.testsArea.SetText(r.Tests, false)
	a.refreshVariablesArea()
	a.rebuildAuthFields()
	a.loading = false
	a.requestChanged()
}

// requestChanged refreshes everything derived from the builder state.
func (a *App) requestChanged() {
	a.renderTabBar()
	a.renderTitles()
}

func (a *App) isDirty() bool {
	if a.linked != nil {
		return !a.req.Equal(*a.linked)
	}
	return !a.req.Equal(models.NewRequest(a.req.Name))
}

func (a *App) renderTitles() {
	if a.urlInput == nil {
		return
	}
	where := "Unsaved request"
	if a.linked != nil {
		if path, ok := a.ws.PathOf(a.linked); ok {
			where = a.linked.Name
			if len(path) > 0 {
				where = models.PathName(path) + " / " + where
			}
		}
	}
	dirty := ""
	if a.isDirty() {
		dirty = fmt.Sprintf(" [%s]●[-]", a.theme.HexWarning)
	}
	a.urlInput.SetTitle(fmt.Sprintf(" [%s]%s[-]%s ", a.theme.HexAccent, tview.Escape(where), dirty))
	label := "SEND"
	switch {
	case a.req.Type == models.TypeWebSocket && a.wsSession().open():
		a.bodyArea.SetTitle(" Message · WebSocket ")
	case a.req.Type == models.TypeWebSocket:
		label = "CONNECT"
		a.bodyArea.SetTitle(" Message · WebSocket ")
	case a.req.BodyType == models.BodyGraphQL:
		a.bodyArea.SetTitle(" Query · GraphQL ")
	default:
		a.bodyArea.SetTitle(fmt.Sprintf(" Body · %s ", bodyLabels[indexOf(models.BodyTypes, a.req.BodyType)]))
	}
	if a.sendBtn.GetLabel() != label {
		a.sendBtn.SetLabel(label)
	}
}

// urlField is the URL input with one extra trick: pasting a curl command
// turns it into a request instead of inserting the text.
type urlField struct {
	*tview.InputField
	onPaste func(text string) bool
}

func (u *urlField) PasteHandler() func(string, func(tview.Primitive)) {
	inner := u.InputField.PasteHandler()
	return func(text string, setFocus func(tview.Primitive)) {
		if u.onPaste != nil && u.onPaste(text) {
			return
		}
		inner(text, setFocus)
	}
}
