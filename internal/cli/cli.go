// Package cli implements the non-interactive "list" and "run" commands.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/postman"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/pkg/httpclient"
)

// List prints every saved request as "Collection/Folder/Request" (top-level
// requests have no prefix).
func List(out io.Writer, ws *models.Workspace) int {
	ws.WalkRequests(func(path []*models.Collection, r *models.Request) {
		fmt.Fprintf(out, "%-7s %s\n", r.Method, joinPath(path, r.Name))
	})
	return 0
}

func joinPath(path []*models.Collection, name string) string {
	parts := make([]string, 0, len(path)+1)
	for _, c := range path {
		parts = append(parts, c.Name)
	}
	if name != "" {
		parts = append(parts, name)
	}
	return strings.Join(parts, "/")
}

type setFlags map[string]string

func (s setFlags) String() string { return "" }
func (s setFlags) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(k) == "" {
		return fmt.Errorf("expected name=value, got %q", v)
	}
	s[strings.TrimSpace(k)] = val
	return nil
}

// Run sends the named requests in order. A name is a request path such as
// "Collection/Folder/Request", or a collection or folder path for every
// request inside it, sub-folders included (case-insensitive). It returns 1
// if any request errors or any test fails.
func Run(out, errOut io.Writer, ws *models.Workspace, store *storage.Store, args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(errOut)
	verbose := fs.Bool("v", false, "print response headers and body")
	overrides := setFlags{}
	fs.Var(overrides, "set", "override a variable for this run (name=value, repeatable)")
	envName := fs.String("env", "", "environment to use for this run (default: the active one; \"none\" for globals only)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	activeBefore := ws.ActiveEnvironment
	if *envName != "" {
		defer func() { ws.ActiveEnvironment = activeBefore }()
		if strings.EqualFold(*envName, "none") {
			ws.SetActive("")
		} else if ws.Environment(*envName) == nil {
			fmt.Fprintf(errOut, "run: no environment named %q (see the env command)\n", *envName)
			return 2
		} else {
			ws.SetActive(*envName)
		}
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(errOut, "run: name at least one request or folder, e.g. \"User Service/Lookup/Get JSON\" (see the list command)")
		return 2
	}

	var targets []*models.Request
	for _, name := range fs.Args() {
		found := find(ws, name)
		if len(found) == 0 {
			fmt.Fprintf(errOut, "run: no request or collection named %q\n", name)
			return 2
		}
		targets = append(targets, found...)
	}

	client := httpclient.NewClient(httpclient.Options{
		Timeout:            time.Duration(ws.Settings.TimeoutSeconds) * time.Second,
		DisableRedirects:   ws.Settings.DisableRedirects,
		InsecureSkipVerify: ws.Settings.InsecureSkipVerify,
	})

	if env := ws.Active(); env != nil {
		fmt.Fprintf(out, "Environment: %s\n", env.Name)
	}
	failed := false
	changed := false
	for _, r := range targets {
		variables := ws.VariableMap()
		for k, v := range overrides {
			variables[k] = v
		}
		fmt.Fprintf(out, "%s %s\n", r.Method, r.Name)

		assignments, errs := script.RunPre(r.PreRequest, variables)
		changed = apply(ws, assignments, overrides) || changed
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(out, "  pre-request error: %v\n", e)
			}
			failed = true
			continue
		}
		p, err := engine.Prepare(*r, variables)
		if err != nil {
			fmt.Fprintf(out, "  error: %v\n", err)
			failed = true
			continue
		}
		for _, m := range p.Missing {
			fmt.Fprintf(out, "  warning: unresolved variable {{%s}}\n", m)
		}
		for _, w := range p.Warnings {
			fmt.Fprintf(out, "  warning: %s\n", w)
		}
		resp, err := client.Do(context.Background(), p.Request)
		if err != nil {
			fmt.Fprintf(out, "  error: %v\n", err)
			failed = true
			continue
		}
		fmt.Fprintf(out, "  %s  %d ms  %d bytes\n", resp.Status, resp.Duration.Milliseconds(), len(resp.Body))

		results := script.RunTests(r.Tests, script.Response{
			Status: resp.StatusCode, Headers: resp.Headers, Body: resp.Body, Duration: resp.Duration,
		}, variables)
		var captures []script.Assignment
		for _, tr := range results {
			switch {
			case tr.Capture != nil:
				captures = append(captures, *tr.Capture)
				fmt.Fprintf(out, "  → %s\n", tr.Message)
			case tr.Passed:
				fmt.Fprintf(out, "  ✓ %s\n", tr.Source)
			default:
				failed = true
				fmt.Fprintf(out, "  ✗ %s  (%s)\n", tr.Source, tr.Message)
			}
		}
		changed = apply(ws, captures, overrides) || changed

		if *verbose {
			fmt.Fprintf(out, "  > %s %s\n", p.Request.Method, p.Request.URL.RequestURI())
			printHeaders(out, "  > ", p.SentHeaders())
			fmt.Fprintf(out, "  < %s %s\n", resp.Proto, resp.Status)
			printHeaders(out, "  < ", resp.Headers)
			fmt.Fprintln(out)
			out.Write(resp.Body)
			fmt.Fprintln(out)
		}
	}

	if changed && store != nil {
		// Captured values went into the environment used for the run, but
		// the active selection is the user's: save with it unchanged.
		saveWs := *ws
		saveWs.ActiveEnvironment = activeBefore
		if err := store.Save(&saveWs); err != nil {
			fmt.Fprintf(errOut, "warning: could not save variables: %v\n", err)
		}
	}
	if failed {
		return 1
	}
	return 0
}

func printHeaders(out io.Writer, prefix string, h http.Header) {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(out, "%s%s: %s\n", prefix, k, strings.Join(h[k], ", "))
	}
}

// apply persists script assignments, except for variables overridden with -set.
func apply(ws *models.Workspace, as []script.Assignment, overrides map[string]string) bool {
	changed := false
	for _, a := range as {
		if _, ok := overrides[a.Name]; ok {
			continue
		}
		if a.Unset {
			ws.UnsetVariable(a.Name)
		} else {
			ws.SetVariable(a.Name, a.Value)
		}
		changed = true
	}
	return changed
}

// find resolves a request path, or a collection/folder path to all the
// requests under it. Request paths win over folder paths with the same name.
func find(ws *models.Workspace, name string) []*models.Request {
	name = strings.Trim(strings.TrimSpace(name), "/")
	var exact, under []*models.Request
	ws.WalkRequests(func(path []*models.Collection, r *models.Request) {
		if strings.EqualFold(joinPath(path, r.Name), name) {
			exact = append(exact, r)
			return
		}
		for i := range path {
			if strings.EqualFold(joinPath(path[:i+1], ""), name) {
				under = append(under, r)
				return
			}
		}
	})
	if len(exact) > 0 {
		return exact[:1]
	}
	return under
}

// History prints the most recent history entries, newest first.
func History(out, errOut io.Writer, store *storage.Store, args []string) int {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	fs.SetOutput(errOut)
	limit := fs.Int("n", 20, "number of entries to show (0 for all)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	h, err := store.LoadHistory()
	if err != nil {
		fmt.Fprintln(errOut, "history:", err)
		return 1
	}
	if len(h.Entries) == 0 {
		fmt.Fprintln(out, "No history yet. Requests sent from the app are recorded here.")
		return 0
	}
	for i, e := range h.Entries {
		if *limit > 0 && i >= *limit {
			break
		}
		status := fmt.Sprintf("%d", e.Status)
		if e.Error != "" {
			status = "ERR"
		}
		line := fmt.Sprintf("%s  %-7s %-3s %6d ms  %s", e.Time.Local().Format("2006-01-02 15:04:05"), e.Method, status, e.DurationMs, e.URL)
		if e.Source != "" {
			line += "  (" + e.Source + ")"
		}
		fmt.Fprintln(out, line)
	}
	return 0
}

// Env lists environments, or with a name switches the active one
// ("none" for globals only).
func Env(out, errOut io.Writer, ws *models.Workspace, store *storage.Store, args []string) int {
	if len(args) == 0 {
		if len(ws.Environments) == 0 {
			fmt.Fprintln(out, "No environments. Create one in the app (Variables tab).")
		}
		for _, e := range ws.Environments {
			mark := " "
			if e == ws.Active() {
				mark = "*"
			}
			fmt.Fprintf(out, "%s %s (%d variables)\n", mark, e.Name, len(e.Variables))
		}
		if ws.Active() == nil {
			fmt.Fprintln(out, "* No Environment (globals only)")
		}
		return 0
	}
	name := strings.Join(args, " ")
	switch {
	case strings.EqualFold(name, "none"):
		ws.SetActive("")
	case ws.Environment(name) == nil:
		fmt.Fprintf(errOut, "env: no environment named %q\n", name)
		return 2
	default:
		ws.SetActive(name)
	}
	if store != nil {
		if err := store.Save(ws); err != nil {
			fmt.Fprintln(errOut, "env:", err)
			return 1
		}
	}
	if e := ws.Active(); e != nil {
		fmt.Fprintf(out, "Active environment: %s\n", e.Name)
	} else {
		fmt.Fprintln(out, "Active environment: none (globals only)")
	}
	return 0
}

// Import reads Postman exports (collections, environments, globals) into
// the workspace and saves it.
func Import(out, errOut io.Writer, ws *models.Workspace, store *storage.Store, files []string) int {
	if len(files) == 0 {
		fmt.Fprintln(errOut, "import: give one or more Postman export files (collection, environment or globals JSON)")
		return 2
	}
	failed := false
	imported := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(errOut, "%s: %v\n", f, err)
			failed = true
			continue
		}
		res, err := postman.Import(data, ws)
		if err != nil {
			fmt.Fprintf(errOut, "%s: %v\n", f, err)
			failed = true
			continue
		}
		imported++
		fmt.Fprintf(out, "%s: %s\n", f, res.Summary())
		for _, w := range res.Warnings {
			fmt.Fprintf(out, "  note: %s\n", w)
		}
	}
	if imported > 0 && store != nil {
		if err := store.Save(ws); err != nil {
			fmt.Fprintln(errOut, "import: could not save workspace:", err)
			return 1
		}
	}
	if failed {
		return 1
	}
	return 0
}

// Export writes a collection or folder ("Collection/Folder"), an
// environment (-env) or the globals (-globals) in Postman format, to -o or
// standard output.
func Export(out, errOut io.Writer, ws *models.Workspace, args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(errOut)
	file := fs.String("o", "", "write to this file instead of standard output")
	envName := fs.String("env", "", "export this environment")
	globals := fs.Bool("globals", false, "export the global variables")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var data []byte
	var err error
	var notes []string
	summary := ""
	switch {
	case *globals:
		data, err = postman.ExportGlobals(ws.Variables)
		summary = fmt.Sprintf("Exported %d global variable(s)", len(ws.Variables))
	case *envName != "":
		env := ws.Environment(*envName)
		if env == nil {
			fmt.Fprintf(errOut, "export: no environment named %q\n", *envName)
			return 2
		}
		data, err = postman.ExportEnvironment(env)
		summary = fmt.Sprintf("Exported environment %q", env.Name)
	case fs.NArg() == 1:
		c := findCollection(ws, fs.Arg(0))
		if c == nil {
			fmt.Fprintf(errOut, "export: no collection or folder named %q\n", fs.Arg(0))
			return 2
		}
		var res *postman.ExportResult
		if res, err = postman.ExportCollection(c); err == nil {
			data, notes = res.Data, res.Notes
			summary = fmt.Sprintf("Exported %q: %d request(s), %d folder(s)", c.Name, res.Requests, res.Folders)
		}
	default:
		fmt.Fprintln(errOut, `export: name one collection or folder ("Collection/Folder"), or use -env NAME or -globals`)
		return 2
	}
	if err != nil {
		fmt.Fprintln(errOut, "export:", err)
		return 1
	}

	if *file == "" {
		out.Write(data)
	} else {
		if err := os.WriteFile(*file, data, 0o644); err != nil {
			fmt.Fprintln(errOut, "export:", err)
			return 1
		}
		fmt.Fprintf(out, "%s to %s\n", summary, *file)
	}
	for _, n := range notes {
		fmt.Fprintf(errOut, "note: %s\n", n)
	}
	return 0
}

// findCollection resolves "Collection/Folder/..." (case-insensitive).
func findCollection(ws *models.Workspace, path string) *models.Collection {
	path = strings.Trim(strings.TrimSpace(path), "/")
	var found *models.Collection
	ws.WalkCollections(func(anc []*models.Collection, c *models.Collection) {
		if found == nil && strings.EqualFold(joinPath(append(append([]*models.Collection(nil), anc...), c), ""), path) {
			found = c
		}
	})
	return found
}
