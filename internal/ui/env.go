package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// Environments work like Postman's: Globals are always available, and the
// active environment's variables override globals with the same name. The
// picker in the request bar switches the active environment; the Variables
// tab edits Globals or any environment.

const noEnvLabel = "No Environment"

// buildEnvPicker creates the environment dropdown shown next to the URL.
func (a *App) buildEnvPicker() {
	a.envDrop = tview.NewDropDown()
	a.envDrop.SetTextOptions(" ", " ", "", "", "")
	a.envDrop.SetBorder(true).SetTitle(" Environment ")
	a.themed = append(a.themed, a.envDrop)
	a.bordered = append(a.bordered, a.envDrop)
}

// envNames lists environment names in order.
func (a *App) envNames() []string {
	names := make([]string, len(a.ws.Environments))
	for i, e := range a.ws.Environments {
		names[i] = e.Name
	}
	return names
}

// refreshEnvPicker reloads the picker's options and selection from the
// workspace (after environments are added, renamed or removed).
func (a *App) refreshEnvPicker() {
	if a.envDrop == nil {
		return
	}
	prev := a.loading
	a.loading = true
	a.envDrop.SetOptions(append([]string{noEnvLabel}, a.envNames()...), func(_ string, i int) {
		if a.loading {
			return
		}
		name := ""
		if i > 0 {
			name = a.ws.Environments[i-1].Name
		}
		a.activateEnv(name)
	})
	idx := 0
	for i, e := range a.ws.Environments {
		if e == a.ws.Active() {
			idx = i + 1
		}
	}
	a.envDrop.SetCurrentOption(idx)
	a.loading = prev
}

// activateEnv switches the active environment ("" for none).
func (a *App) activateEnv(name string) {
	a.ws.SetActive(name)
	a.persist()
	a.editingEnv = a.ws.ActiveEnvironment
	a.refreshEnvPicker()
	a.refreshVariablesTab()
	if env := a.ws.Active(); env != nil {
		a.setStatus(levelInfo, "Environment: "+env.Name)
	} else {
		a.setStatus(levelInfo, "No environment: only Globals are used")
	}
}

// cycleEnv moves to the next environment (Alt+E), wrapping through "none".
func (a *App) cycleEnv() {
	names := append([]string{""}, a.envNames()...)
	cur := 0
	for i, n := range names {
		if n == a.ws.ActiveEnvironment {
			cur = i
		}
	}
	a.activateEnv(names[(cur+1)%len(names)])
}

// ---------- Variables tab ----------

func (a *App) buildVariablesTab() *tab {
	a.editingEnv = a.ws.ActiveEnvironment

	a.varsTarget = a.newDrop("Editing  ", nil)
	a.varsArea = a.newArea("Globals", "# example:\n# baseUrl=https://httpbin.org\n# token=secret")
	a.varsArea.SetChangedFunc(func() {
		if a.loading {
			return
		}
		kvs := models.ParseKV(a.varsArea.GetText(), "=")
		if env := a.ws.Environment(a.editingEnv); env != nil {
			env.Variables = kvs
		} else {
			a.ws.Variables = kvs
		}
	})
	a.varsArea.SetBlurFunc(func() { a.persist() })

	button := func(label string, fn func()) *tview.Button {
		b := tview.NewButton(label).SetSelectedFunc(fn)
		a.envButtons = append(a.envButtons, b)
		return b
	}
	buttons := tview.NewFlex().
		AddItem(button("New", a.newEnv), 5, 0, false).AddItem(nil, 1, 0, false).
		AddItem(button("Rename", a.renameEnv), 8, 0, false).AddItem(nil, 1, 0, false).
		AddItem(button("Duplicate", a.duplicateEnv), 11, 0, false).AddItem(nil, 1, 0, false).
		AddItem(button("Delete", a.deleteEnv), 8, 0, false).AddItem(nil, 1, 0, false).
		AddItem(button("Use it", a.useEditedEnv), 8, 0, false).AddItem(nil, 1, 0, false).
		AddItem(button("Export", a.exportEditedVariables), 8, 0, false).AddItem(nil, 0, 1, false)
	a.themed = append(a.themed, buttons)
	a.renderers = append(a.renderers, a.styleEnvButtons)

	page := a.newFlex(tview.FlexRow).
		AddItem(a.varsTarget, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(buttons, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(a.varsArea, 0, 1, false).
		AddItem(a.newHint(func(t *Theme) string {
			return hintLine(t, "name=value per line, use as {{name}}",
				"The active environment overrides Globals",
				"Built-in: {{$uuid}} {{$timestamp}} {{$isoTimestamp}} {{$randomInt}}")
		}), 2, 0, false)
	a.refreshVariablesTab()
	return &tab{name: "Variables", page: page, focus: func() []tview.Primitive {
		fs := []tview.Primitive{a.varsTarget, a.varsArea}
		for _, b := range a.envButtons {
			fs = append(fs, b)
		}
		return fs
	}}
}

func (a *App) styleEnvButtons() {
	t := a.theme
	for _, b := range a.envButtons {
		b.SetStyle(tcell.StyleDefault.Background(t.Input).Foreground(t.Text))
		b.SetActivatedStyle(tcell.StyleDefault.Background(t.Button).Foreground(t.ButtonText))
		b.SetDisabledStyle(tcell.StyleDefault.Background(t.Background).Foreground(t.Muted))
	}
}

// refreshVariablesTab shows the variable set being edited.
func (a *App) refreshVariablesTab() {
	if a.varsTarget == nil {
		return
	}
	env := a.ws.Environment(a.editingEnv)
	if env == nil {
		a.editingEnv = ""
	}
	prev := a.loading
	a.loading = true
	options := []string{"Globals"}
	idx := 0
	for i, e := range a.ws.Environments {
		label := e.Name
		if e == a.ws.Active() {
			label += "  (active)"
		}
		options = append(options, label)
		if e == env {
			idx = i + 1
		}
	}
	a.varsTarget.SetOptions(options, func(_ string, i int) {
		if a.loading {
			return
		}
		a.persist()
		if i == 0 {
			a.editingEnv = ""
		} else {
			a.editingEnv = a.ws.Environments[i-1].Name
		}
		a.refreshVariablesTab()
	})
	a.varsTarget.SetCurrentOption(idx)

	var kvs []models.KeyValue
	title := " Globals · used in every environment "
	if env != nil {
		kvs = env.Variables
		state := "not active"
		if env == a.ws.Active() {
			state = "active"
		}
		title = fmt.Sprintf(" Environment: %s · %s ", tview.Escape(env.Name), state)
	} else {
		kvs = a.ws.Variables
	}
	a.varsArea.SetTitle(title)
	a.varsArea.SetText(models.FormatKV(kvs, "="), false)
	a.loading = prev

	// Environment-only buttons are disabled while editing Globals; "Use it"
	// also when the edited environment is already active.
	for i, b := range a.envButtons {
		switch i {
		case 0, 5: // New, Export (Globals can be exported too)
			b.SetDisabled(false)
		case 4: // Use it
			b.SetDisabled(env == nil || env == a.ws.Active())
		default:
			b.SetDisabled(env == nil)
		}
	}
}

// refreshVariablesArea is called after scripts change variables.
func (a *App) refreshVariablesArea() { a.refreshVariablesTab() }

func (a *App) envAfterChange(focus tview.Primitive) {
	a.persist()
	a.refreshEnvPicker()
	a.refreshVariablesTab()
	if focus != nil {
		a.tv.SetFocus(focus)
	}
}

func (a *App) newEnv() {
	a.prompt("New Environment", "Name", a.ws.UniqueEnvName("New Environment"), func(name string) {
		if a.ws.Environment(name) != nil {
			a.setStatus(levelError, fmt.Sprintf("An environment called %q already exists", name))
			return
		}
		a.ws.Environments = append(a.ws.Environments, &models.Environment{Name: name})
		a.editingEnv = name
		a.envAfterChange(a.varsArea)
		a.setStatus(levelSuccess, "Created environment "+name+". Add name=value lines, then use it from the picker or \"Use it\".")
	})
}

func (a *App) renameEnv() {
	env := a.ws.Environment(a.editingEnv)
	if env == nil {
		return
	}
	a.prompt("Rename Environment", "Name", env.Name, func(name string) {
		if other := a.ws.Environment(name); other != nil && other != env {
			a.setStatus(levelError, fmt.Sprintf("An environment called %q already exists", name))
			return
		}
		wasActive := env == a.ws.Active()
		env.Name = name
		if wasActive {
			a.ws.ActiveEnvironment = name
		}
		a.editingEnv = name
		a.envAfterChange(a.varsTarget)
	})
}

func (a *App) duplicateEnv() {
	env := a.ws.Environment(a.editingEnv)
	if env == nil {
		return
	}
	cp := env.Clone()
	cp.Name = a.ws.UniqueEnvName(env.Name + " copy")
	a.ws.Environments = append(a.ws.Environments, cp)
	a.editingEnv = cp.Name
	a.envAfterChange(a.varsTarget)
	a.setStatus(levelSuccess, "Duplicated as "+cp.Name)
}

func (a *App) deleteEnv() {
	env := a.ws.Environment(a.editingEnv)
	if env == nil {
		return
	}
	msg := fmt.Sprintf("Delete environment %q with %d variable(s)?", env.Name, len(env.Variables))
	a.confirm(msg, []string{"Delete", "Cancel"}, func(label string) {
		if label != "Delete" {
			return
		}
		for i, e := range a.ws.Environments {
			if e == env {
				a.ws.Environments = append(a.ws.Environments[:i], a.ws.Environments[i+1:]...)
				break
			}
		}
		if strings.EqualFold(a.ws.ActiveEnvironment, env.Name) {
			a.ws.ActiveEnvironment = ""
		}
		a.editingEnv = a.ws.ActiveEnvironment
		a.envAfterChange(a.varsTarget)
		a.setStatus(levelInfo, "Deleted environment "+env.Name)
	})
}

func (a *App) useEditedEnv() {
	if env := a.ws.Environment(a.editingEnv); env != nil {
		a.activateEnv(env.Name)
	}
}
