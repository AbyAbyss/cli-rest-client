package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// openDialog shows p centered above everything else and gives it focus.
func (a *App) openDialog(p tview.Primitive, width, height int) {
	name := fmt.Sprintf("dialog-%d", len(a.dialogs))
	a.dialogs = append(a.dialogs, dialog{name: name, prev: a.tv.GetFocus()})
	center := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
	a.pages.AddPage(name, center, true, true)
	a.tv.SetFocus(p)
}

// closeDialog removes the top dialog and restores the previous focus.
func (a *App) closeDialog() {
	if len(a.dialogs) == 0 {
		return
	}
	d := a.dialogs[len(a.dialogs)-1]
	a.dialogs = a.dialogs[:len(a.dialogs)-1]
	a.pages.RemovePage(d.name)
	if d.prev != nil {
		a.tv.SetFocus(d.prev)
	}
}

// panel is a Flex that paints its background. tview's Flex leaves its
// background untouched, so in a dialog the screen underneath would show
// through the gaps between items.
type panel struct {
	*tview.Flex
	bg tcell.Color
}

func newPanel(bg tcell.Color) *panel {
	p := &panel{Flex: tview.NewFlex(), bg: bg}
	p.SetBackgroundColor(bg)
	return p
}

func (p *panel) Draw(screen tcell.Screen) {
	x, y, w, h := p.GetRect()
	style := tcell.StyleDefault.Background(p.bg)
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
	}
	p.Flex.Draw(screen)
}

// formDialog shows input widgets stacked vertically. Tab/Shift+Tab move
// between them, Enter in a text field calls submit, Esc cancels. tview.Form
// is deliberately not used: it moves focus to its next item after Enter,
// which would steal focus back after the dialog has closed.
func (a *App) formDialog(title, hint string, items []tview.Primitive, submit func(), width int) {
	t := a.theme
	field := tcell.StyleDefault.Background(t.Background).Foreground(t.Text)
	label := tcell.StyleDefault.Background(t.Input).Foreground(t.Muted)
	active := tcell.StyleDefault.Background(t.Button).Foreground(t.ButtonText)

	box := newPanel(t.Input)
	box.SetDirection(tview.FlexRow)
	box.SetBorder(true).SetBorderColor(t.Focus).SetTitleColor(t.Title).SetBorderPadding(1, 0, 2, 2)
	box.SetTitle(" " + title + " ")
	for _, p := range items {
		switch w := p.(type) {
		case *tview.InputField:
			w.SetBackgroundColor(t.Input)
			w.SetLabelStyle(label)
			w.SetFieldStyle(field)
			w.SetPlaceholderStyle(tcell.StyleDefault.Background(t.Background).Foreground(t.Muted))
			w.SetDoneFunc(func(key tcell.Key) {
				if key == tcell.KeyEnter {
					submit()
				}
			})
		case *tview.DropDown:
			w.SetBackgroundColor(t.Input)
			w.SetLabelStyle(label)
			w.SetFieldStyle(field)
			w.SetFocusedStyle(active)
			w.SetPrefixStyle(active)
			w.SetListStyles(field, active)
		}
		box.AddItem(p, 1, 0, false).AddItem(nil, 1, 0, false)
	}
	hintView := tview.NewTextView().SetDynamicColors(true).
		SetText(fmt.Sprintf("[%s]%s", t.HexMuted, tview.Escape(hint)))
	hintView.SetBackgroundColor(t.Input)
	box.AddItem(hintView, 1, 0, false)

	box.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		focus := a.tv.GetFocus()
		for _, p := range items {
			if d, ok := p.(*tview.DropDown); ok && d.IsOpen() {
				return ev // let the open list handle keys
			}
		}
		idx := 0
		for i, p := range items {
			if p == focus {
				idx = i
			}
		}
		switch ev.Key() {
		case tcell.KeyEsc:
			a.closeDialog()
			return nil
		case tcell.KeyTab, tcell.KeyDown:
			if _, ok := focus.(*tview.DropDown); ok && ev.Key() == tcell.KeyDown {
				return ev
			}
			a.tv.SetFocus(items[(idx+1)%len(items)])
			return nil
		case tcell.KeyBacktab, tcell.KeyUp:
			a.tv.SetFocus(items[(idx-1+len(items))%len(items)])
			return nil
		}
		return ev
	})
	a.openDialog(box, width, 2*len(items)+4)
	a.tv.SetFocus(items[0])
}

// prompt asks for a single non-empty line of text.
func (a *App) prompt(title, label, initial string, ok func(string)) {
	input := tview.NewInputField().SetLabel(label + "  ").SetText(initial)
	a.formDialog(title, "Enter to confirm · Esc to cancel", []tview.Primitive{input}, func() {
		v := strings.TrimSpace(input.GetText())
		if v == "" {
			return
		}
		a.closeDialog()
		ok(v)
	}, 56)
}

// confirm shows a message with buttons; done receives the chosen label, or ""
// when cancelled with Esc.
func (a *App) confirm(text string, buttons []string, done func(label string)) {
	t := a.theme
	m := tview.NewModal().SetText(text).AddButtons(buttons)
	m.SetBackgroundColor(t.Input)
	m.SetTextColor(t.Text)
	m.SetBorderColor(t.Focus)
	m.SetButtonStyle(tcell.StyleDefault.Background(t.Border).Foreground(t.Text))
	m.SetButtonActivatedStyle(tcell.StyleDefault.Background(t.Button).Foreground(t.ButtonText))
	m.SetDoneFunc(func(_ int, label string) {
		a.closeDialog()
		done(label)
	})
	// Modal centers itself, so give it the whole screen.
	a.dialogs = append(a.dialogs, dialog{name: fmt.Sprintf("dialog-%d", len(a.dialogs)), prev: a.tv.GetFocus()})
	a.pages.AddPage(a.dialogs[len(a.dialogs)-1].name, m, true, true)
	a.tv.SetFocus(m)
}

// showText displays read-only text (help, cURL) in a scrollable dialog.
func (a *App) showText(title, text string, width, height int) {
	t := a.theme
	tv := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true)
	tv.SetText(text)
	tv.SetBackgroundColor(t.Input)
	tv.SetTextColor(t.Text)
	tv.SetBorder(true).SetBorderColor(t.Focus).SetTitleColor(t.Title).SetBorderPadding(1, 1, 2, 2)
	tv.SetTitle(" " + title + " ")
	tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEsc || ev.Key() == tcell.KeyEnter || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q') {
			a.closeDialog()
			return nil
		}
		return ev
	})
	a.openDialog(tv, width, height)
}

func (a *App) showHelp() {
	a.showText("Help · Esc to close", helpText(a.theme), 78, 34)
}

// guardUnsaved runs then() right away, or after the user agrees to drop (or
// save) unsaved changes in the builder.
func (a *App) guardUnsaved(then func()) {
	if !a.isDirty() {
		then()
		return
	}
	if a.linked != nil {
		a.confirm(fmt.Sprintf("%q has unsaved changes.", a.linked.Name), []string{"Save", "Discard", "Cancel"}, func(label string) {
			switch label {
			case "Save":
				a.save()
				then()
			case "Discard":
				then()
			}
		})
		return
	}
	a.confirm("The current request has not been saved.", []string{"Discard", "Cancel"}, func(label string) {
		if label == "Discard" {
			then()
		}
	})
}

// save writes the builder back to its saved request, or asks where to save it.
func (a *App) save() {
	if a.linked == nil || !a.ws.Contains(a.linked) {
		a.saveAs()
		return
	}
	*a.linked = a.req.Clone()
	if a.persist() {
		a.setStatus(levelSuccess, "Saved "+a.linked.Name)
	}
	a.rebuildTree(a.linked)
}

// saveAs stores the builder as a new request in a chosen collection.
func (a *App) saveAs() {
	name := a.req.Name
	if name == "" {
		name = suggestName(a.req)
	}
	const newLabel = "+ New collection"
	labels, targets := a.containerChoices(nil)
	labels = append(labels, newLabel)
	var cur *models.Collection
	if a.linked != nil && a.ws.Contains(a.linked) {
		cur = a.ws.ParentOf(a.linked)
	} else {
		cur = a.targetContainer()
	}
	sel := 0
	for i, t := range targets {
		if t == cur {
			sel = i
		}
	}

	nameInput := tview.NewInputField().SetLabel("Name            ").SetText(name)
	locDrop := tview.NewDropDown().SetLabel("Save in         ").SetOptions(labels, nil).SetCurrentOption(sel)
	locDrop.SetTextOptions(" ", " ", "", "", "")
	newCol := tview.NewInputField().SetLabel("New collection  ").SetPlaceholder("used with \"" + newLabel + "\"")
	submit := func() {
		n := strings.TrimSpace(nameInput.GetText())
		if n == "" {
			return
		}
		idx, _ := locDrop.GetCurrentOption()
		var parent *models.Collection
		if idx >= 0 && idx < len(targets) {
			parent = targets[idx]
		} else {
			cn := strings.TrimSpace(newCol.GetText())
			if cn == "" {
				cn = "My Collection"
			}
			parent = &models.Collection{Name: cn}
			a.ws.Collections = append(a.ws.Collections, parent)
		}
		a.closeDialog()
		r := a.req.Clone()
		r.Name = n
		ptr := &r
		list := a.ws.RequestsIn(parent)
		*list = append(*list, ptr)
		where := topLevelLabel
		if parent != nil {
			delete(a.collapsed, parent)
			anc, _ := a.ws.AncestorsOf(parent)
			where = models.PathName(append(anc, parent))
		}
		a.req.Name = n
		a.linked = ptr
		if a.persist() {
			a.setStatus(levelSuccess, fmt.Sprintf("Saved %s to %s", n, where))
		}
		a.rebuildTree(ptr)
	}
	a.formDialog("Save Request As", "Enter to save · Tab to move · Esc to cancel",
		[]tview.Primitive{nameInput, locDrop, newCol}, submit, 72)
}

// newRequest clears the builder for a fresh, unsaved request.
func (a *App) newRequest() {
	a.guardUnsaved(func() {
		a.loadIntoBuilder(models.NewRequest(""), nil)
		a.result = nil
		a.renderResponse()
		a.rebuildTree(nil)
		a.switchTab(0)
		a.tv.SetFocus(a.urlInput)
		a.setStatus(levelInfo, "New request. Ctrl+S saves it to a collection.")
	})
}
