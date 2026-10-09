# Architecture

## Packages

```
cmd/term-rest-client   main: flags, workspace loading, starts the UI or the list/run commands
internal/models        Workspace, Collection (with nested Folders), Request, ...; tree helpers in tree.go
internal/storage       JSON workspace file: Load, atomic Save, DefaultPath, SampleWorkspace
internal/vars          {{name}} substitution and dynamic variables ($uuid, $timestamp, ...)
internal/engine        Prepare(Request, vars) -> *http.Request (+ missing vars, warnings); Curl()
internal/script        RunPre (set/unset) and RunTests (assertions and captures)
internal/curl          curl command line parser (shell quoting, curl options -> Request) and export (Resolved, Template)
internal/codegen       request -> curl / Python / JavaScript / Go / HTTPie (one Spec, a generator per language)
internal/clipboard     Copy(text): pbcopy / wl-copy / xclip / xsel / clip, OSC 52 fallback
internal/postman       Postman import and export (collections, environments, globals)
internal/cli           headless commands (list, run, env, import, export, curl, history)
internal/ui            tview application
internal/testutil      httpbin-compatible test server
pkg/httpclient         http.Client wrapper: timeout, redirects, TLS, cancellation, timing
```

Dependencies point one way: `ui` and `cli` use `engine`, `script`, `storage` and `pkg/httpclient`; those use `models` and `vars`. Nothing below `ui` imports tview, so everything except the screen code can be tested without a terminal.

## Sending a request

1. The builder holds a `models.Request` (`App.req`). Every widget's change handler updates it directly.
2. `script.RunPre` runs the pre-request script against a copy of the workspace variables. Assignments are written back to the workspace and saved.
3. `engine.Prepare` substitutes variables, appends params, applies auth, encodes the body, sets a default `Content-Type`, and validates the URL and header names.
4. `httpclient.Client.Do` runs on a goroutine with a cancellable context. The result comes back to the UI goroutine through `QueueUpdateDraw`.
5. `script.RunTests` evaluates the tests. Captures (`set x = json.y`) are written to the workspace variables.
6. The response pane is re-rendered from a `sendResult`, which is also what theme changes re-render from.

## UI structure

`internal/ui` splits the `App` type across files:

| File | Responsibility |
|------|----------------|
| `app.go` | layout, focus cycle, global keys, status bar, draft save/restore |
| `builder.go` | request bar and the eight tabs, syncing widgets with `App.req` |
| `tree.go` | collections tree and its key bindings |
| `dialogs.go` | dialog stack, prompts, confirmations, save / save-as, unsaved-changes guard |
| `send.go` | sending, cancelling, rendering the response, cURL, theming |
| `format.go` | JSON pretty-printing and highlighting, with tview tag escaping |
| `theme.go` / `help.go` | colour themes and the shortcut reference |

A few rules keep keyboard handling predictable:

- Global shortcuts use Ctrl, Alt and function keys. Plain characters only act globally (tab switching with `1`-`8`, `?` for help) when the focused widget is not a text field, so typing never triggers commands.
- `Tab` cycles through `App.focusables()`, which is computed from the current tab. Widgets that are only decoration redirect focus when clicked.
- While a dialog is open, every key except quit goes to the dialog.
- Dialogs with text fields use a small custom form instead of `tview.Form`, because `tview.Form` moves focus to its next item after `Enter`, which would steal focus back after the dialog closes.
- Text from responses, names and errors is passed through `tview.Escape` before display, so content like `["a"]` isn't interpreted as a colour tag.

## Persistence

The workspace file is written with a temp file and rename, so a crash can't leave a half-written file. It is saved after every collection change, save, settings change, variable capture, when leaving the Variables tab, and on quit. On quit the builder state is stored as `Workspace.Draft` together with the location of the saved request it came from (folder indices from the top plus the request index, see `Workspace.Location`), and restored on the next start.

Collections form a tree: `Collection.Folders` holds nested collections (shown as folders) and `Workspace.Requests` holds top-level requests. Code that needs "the list this item lives in" uses `Workspace.RequestsIn(parent)` / `FoldersIn(parent)`, where a nil parent means the top level, so the same code handles every level.

## Environments

`Workspace.Variables` are the globals; `Workspace.Environments` are named sets and `ActiveEnvironment` names the active one. `VariableMap()` merges globals with the active environment on top, and everything that resolves variables (engine, scripts, CLI) goes through it. `SetVariable` updates a variable where it already lives (active environment first, then globals) and puts new ones in the active environment, so script captures land where you'd expect. `ui/env.go` holds the picker and the Variables tab's environment manager.

## Code generation

`codegen.build` turns a request into a `Spec`: method, URL, query fields, headers, Basic auth, and the body as text plus, for JSON, an ordered tree (`Node`). It calls `engine.Prepare` first, so the snippet fails in the same cases a send would. Strings are `Str` values, a list of literal and `{{variable}}` parts: in resolved mode every part is literal, in template mode variables stay as parts and are collected (in order of first use) for the declarations at the top. A variable used as a whole JSON value (`{"n": {{count}}}`) becomes a `NodeBare`, found by quoting such references with a marker before parsing. Each language file (`python.go`, `javascript.go`, `golang.go`, `httpie.go`) only formats a `Spec`; curl goes through `internal/curl`. `run_test.go` executes every snippet against a capture server and compares the request with what the app sends.

## History

`models.History` (newest first, capped at 200 entries, bodies cut at 64 KB) is saved by `storage.Store.SaveHistory` to `<workspace>.history.json`, separate from the workspace. `ui/send.go` records an entry in `finish` for every request that got a response or failed on the network (not for cancelled ones or ones that never left, such as an empty URL). `ui/history.go` draws the sidebar view and rebuilds a `sendResult` from an entry, so the response pane renders history exactly like a live response.

## Testing

- Unit tests cover variables, the request engine, scripts, storage and the HTTP client.
- `internal/cli` runs every sample request against `testutil.NewHTTPBin()` and requires all their tests to pass.
- `internal/ui/app_test.go` starts the real app on a `tcell.SimulationScreen`, injects keys and mouse clicks, and checks both state and screen contents. The harness waits for each key to reach the app's input handler before continuing, so the tests are deterministic, and they pass under `-race`.
