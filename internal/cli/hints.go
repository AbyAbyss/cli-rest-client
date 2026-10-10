package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
)

// isGlobalsOnly reports whether an -env or env argument means "no
// environment": none, or global(s) as the app calls those variables.
func isGlobalsOnly(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "none", "global", "globals", "no environment":
		return true
	}
	return false
}

// envHint explains which environments exist, after an unknown name.
func envHint(ws *models.Workspace) string {
	if len(ws.Environments) == 0 {
		return "This workspace has no environments; create them in the app's Variables tab, " +
			"or use -env none (or -env global) for the global variables."
	}
	names := make([]string, len(ws.Environments))
	for i, e := range ws.Environments {
		names[i] = fmt.Sprintf("%q", e.Name)
	}
	return "Environments in this workspace: " + strings.Join(names, ", ") +
		". Use -env none (or -env global) for the global variables only."
}

// pathHint suggests saved paths close to one that matched nothing.
func pathHint(ws *models.Workspace, name string) string {
	var sb strings.Builder
	if s := suggest(ws, name, 5); len(s) > 0 {
		sb.WriteString("Did you mean:\n")
		for _, p := range s {
			fmt.Fprintf(&sb, "  %s\n", p)
		}
	}
	if len(find(storage.SampleWorkspace(), name)) > 0 {
		sb.WriteString("That path is in the sample workspace. To try the samples without changing your own\n" +
			"workspace, use a new file: term-rest-client -data demo.json list\n")
	}
	sb.WriteString("term-rest-client list shows every request path.")
	return sb.String()
}

// suggest ranks saved request, collection and folder paths by how many of
// the words in name they contain.
func suggest(ws *models.Workspace, name string, limit int) []string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '/' || r == ' ' })
	if len(words) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	ws.WalkCollections(func(anc []*models.Collection, c *models.Collection) {
		add(joinPath(append(append([]*models.Collection(nil), anc...), c), ""))
	})
	ws.WalkRequests(func(path []*models.Collection, r *models.Request) { add(joinPath(path, r.Name)) })

	type scored struct {
		path  string
		score int
	}
	var out []scored
	for _, p := range paths {
		lp := strings.ToLower(p)
		score := 0
		for _, w := range words {
			if strings.Contains(lp, w) {
				score += len(w)
			}
		}
		if score > 0 {
			out = append(out, scored{p, score})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return len(out[i].path) < len(out[j].path)
	})
	var res []string
	for i := 0; i < len(out) && i < limit; i++ {
		res = append(res, out[i].path)
	}
	return res
}
