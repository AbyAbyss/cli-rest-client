package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/importer"
)

// importPostman asks for a Postman export file (collection, environment or
// globals) and adds it to the workspace.
func (a *App) importPostman() {
	a.prompt("Import from Postman", "File", a.lastImportDir, func(path string) {
		path = expandHome(strings.Trim(strings.TrimSpace(path), `"'`))
		data, err := os.ReadFile(path)
		if err != nil {
			a.setStatus(levelError, "Import failed: "+err.Error())
			return
		}
		res, err := importer.Import(data, a.ws)
		if err != nil {
			a.setStatus(levelError, "Import failed: "+err.Error())
			return
		}
		a.lastImportDir = filepath.Dir(path) + string(filepath.Separator)
		a.persist()
		a.refreshEnvPicker()
		a.refreshVariablesTab()
		if res.Collection != nil {
			a.showSidebar(sideCollections, false)
			a.rebuildTree(res.Collection)
			a.tv.SetFocus(a.tree)
		}
		a.setStatus(levelSuccess, res.Summary())
		if len(res.Warnings) > 0 {
			var sb strings.Builder
			fmt.Fprintf(&sb, "[%s::b]%s[-:-:-]\n\n", a.theme.HexSuccess, tview.Escape(res.Summary()))
			fmt.Fprintf(&sb, "[%s]Some things don't map one-to-one:\n\n", a.theme.HexMuted)
			for _, w := range res.Warnings {
				fmt.Fprintf(&sb, "[%s]• [%s]%s\n", a.theme.HexWarning, a.theme.HexText, tview.Escape(w))
			}
			a.showText("Import notes · Esc to close", sb.String(), 90, 24)
		}
	})
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
