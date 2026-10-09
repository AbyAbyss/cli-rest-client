package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/curl"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/postman"
)

// importPostman opens the Import dialog. It accepts a pasted cURL command
// (one or several) or the path of a Postman export / a file containing curl
// commands.
func (a *App) importPostman() {
	a.textDialog("Import", "Paste a cURL command, or enter the path of a Postman export · Enter imports · Esc cancels",
		a.lastImportDir, func(text string) {
			text = strings.TrimSpace(text)
			if curl.LooksLikeCurl(text) {
				a.importCurl(text, false)
				return
			}
			path := expandHome(strings.Trim(text, `"'`))
			data, err := os.ReadFile(path)
			if err != nil {
				a.setStatus(levelError, "Import failed: "+err.Error())
				return
			}
			a.lastImportDir = filepath.Dir(path) + string(filepath.Separator)
			if curl.LooksLikeCurl(string(data)) {
				a.importCurl(string(data), false)
				return
			}
			a.importPostmanData(data)
		})
}

func (a *App) importPostmanData(data []byte) {
	res, err := postman.Import(data, a.ws)
	if err != nil {
		a.setStatus(levelError, "Import failed: "+err.Error())
		return
	}
	a.persist()
	a.refreshEnvPicker()
	a.refreshVariablesTab()
	if res.Collection != nil {
		a.showSidebar(sideCollections, false)
		a.rebuildTree(res.Collection)
		a.tv.SetFocus(a.tree)
	}
	a.setStatus(levelSuccess, res.Summary())
	a.showNotes("Import notes", res.Summary(), "Some things don't map one-to-one:", res.Warnings)
}

func (a *App) showNotes(title, summary, intro string, notes []string) {
	if len(notes) == 0 {
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s::b]%s[-:-:-]\n\n", a.theme.HexSuccess, tview.Escape(summary))
	fmt.Fprintf(&sb, "[%s]%s\n\n", a.theme.HexMuted, tview.Escape(intro))
	for _, w := range notes {
		fmt.Fprintf(&sb, "[%s]• [%s]%s\n", a.theme.HexWarning, a.theme.HexText, tview.Escape(w))
	}
	a.showText(title+" · Esc to close", sb.String(), 90, 24)
}

// importCurl loads one curl command into the builder as an unsaved
// request, or saves several into a new collection. fromURLField skips the
// unsaved-changes prompt: the command was typed or pasted over the URL, so
// replacing the request is exactly what was asked for.
func (a *App) importCurl(text string, fromURLField bool) {
	results, err := curl.ParseAll(text)
	if err != nil {
		a.setStatus(levelError, "Could not read the cURL command: "+err.Error())
		return
	}
	var notes []string
	for _, r := range results {
		for _, n := range r.Notes {
			prefix := ""
			if len(results) > 1 {
				prefix = r.Request.Name + ": "
			}
			notes = append(notes, prefix+n)
		}
	}
	if len(results) == 1 {
		r := results[0].Request
		load := func() {
			a.loadIntoBuilder(r, nil)
			a.result = nil
			a.renderResponse()
			a.rebuildTree(nil)
			a.tv.SetFocus(a.urlInput)
			summary := fmt.Sprintf("Imported from cURL: %s %s. Ctrl+R sends it, Ctrl+S saves it.", r.Method, r.URL)
			a.setStatus(levelSuccess, summary)
			a.showNotes("cURL import notes", summary, "Some options don't carry over:", notes)
		}
		if fromURLField {
			load()
		} else {
			a.guardUnsaved(load)
		}
		return
	}
	name := "cURL import"
	for i := 2; ; i++ {
		taken := false
		for _, c := range a.ws.Collections {
			if strings.EqualFold(c.Name, name) {
				taken = true
			}
		}
		if !taken {
			break
		}
		name = fmt.Sprintf("cURL import %d", i)
	}
	col := &models.Collection{Name: name}
	for _, r := range results {
		req := r.Request
		col.Requests = append(col.Requests, &req)
	}
	a.ws.Collections = append(a.ws.Collections, col)
	a.persist()
	a.showSidebar(sideCollections, false)
	a.rebuildTree(col)
	a.tv.SetFocus(a.tree)
	summary := fmt.Sprintf("Imported %d cURL commands into %q", len(results), name)
	a.setStatus(levelSuccess, summary)
	a.showNotes("cURL import notes", summary, "Some options don't carry over:", notes)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// fileSafe turns a name into something usable as a file name.
func fileSafe(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" {
		return "export"
	}
	return name
}

// exportTo asks for a file name (suggesting suggested in the last used
// folder), confirms before overwriting, and writes data there.
func (a *App) exportTo(title, suggested string, data []byte, summary string, notes []string) {
	start := a.lastImportDir
	if start == "" {
		if wd, err := os.Getwd(); err == nil {
			start = wd + string(filepath.Separator)
		}
	}
	a.prompt(title, "Save to", start+suggested, func(path string) {
		path = expandHome(strings.Trim(strings.TrimSpace(path), `"'`))
		write := func() {
			if err := os.WriteFile(path, data, 0o644); err != nil {
				a.setStatus(levelError, "Export failed: "+err.Error())
				return
			}
			a.lastImportDir = filepath.Dir(path) + string(filepath.Separator)
			a.setStatus(levelSuccess, fmt.Sprintf("%s to %s", summary, path))
			if len(notes) > 0 {
				var sb strings.Builder
				fmt.Fprintf(&sb, "[%s::b]%s[-:-:-]\n[%s]%s\n\n", a.theme.HexSuccess, tview.Escape(summary), a.theme.HexMuted, tview.Escape(path))
				fmt.Fprintf(&sb, "[%s]These script lines have no Postman equivalent and were kept as comments:\n\n", a.theme.HexMuted)
				for _, n := range notes {
					fmt.Fprintf(&sb, "[%s]• [%s]%s\n", a.theme.HexWarning, a.theme.HexText, tview.Escape(n))
				}
				a.showText("Export notes · Esc to close", sb.String(), 90, 24)
			}
		}
		if _, err := os.Stat(path); err == nil {
			a.confirm(fmt.Sprintf("%s already exists. Replace it?", filepath.Base(path)), []string{"Replace", "Cancel"}, func(label string) {
				if label == "Replace" {
					write()
				}
			})
			return
		}
		write()
	})
}

// exportSelected exports the selected collection or folder (or the one
// holding the selected request; top-level requests export together).
func (a *App) exportSelected() {
	c, r := a.selection()
	if r != nil {
		c = a.ws.ParentOf(r)
	}
	var res *postman.ExportResult
	var err error
	name := ""
	if c != nil {
		name = c.Name
		res, err = postman.ExportCollection(c)
	} else {
		if len(a.ws.Requests) == 0 {
			a.setStatus(levelWarning, "Select a collection or folder to export")
			return
		}
		name = "Requests"
		res, err = postman.ExportRequests(name, a.ws.Requests)
	}
	if err != nil {
		a.setStatus(levelError, "Export failed: "+err.Error())
		return
	}
	summary := fmt.Sprintf("Exported %q (%s, %s) as a Postman collection", name, plural(res.Requests, "request"), plural(res.Folders, "folder"))
	a.exportTo("Export to Postman", fileSafe(name)+".postman_collection.json", res.Data, summary, res.Notes)
}

// exportEditedVariables exports the environment (or Globals) being edited
// in the Variables tab.
func (a *App) exportEditedVariables() {
	if env := a.ws.Environment(a.editingEnv); env != nil {
		data, err := postman.ExportEnvironment(env)
		if err != nil {
			a.setStatus(levelError, "Export failed: "+err.Error())
			return
		}
		a.exportTo("Export Environment", fileSafe(env.Name)+".postman_environment.json", data,
			fmt.Sprintf("Exported environment %q", env.Name), nil)
		return
	}
	data, err := postman.ExportGlobals(a.ws.Variables)
	if err != nil {
		a.setStatus(levelError, "Export failed: "+err.Error())
		return
	}
	a.exportTo("Export Globals", "globals.postman_globals.json", data, "Exported Globals", nil)
}
