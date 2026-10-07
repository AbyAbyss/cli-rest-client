# Architecture

## Packages

```
cmd/term-rest-client   main: flags, workspace loading, starts the UI or the list/run commands
internal/models        Workspace, Collection, Request, KeyValue, Auth, Settings, Draft
internal/storage       JSON workspace file: Load, atomic Save, DefaultPath, SampleWorkspace
internal/vars          {{name}} substitution and dynamic variables ($uuid, $timestamp, ...)
internal/engine        Prepare(Request, vars) -> *http.Request (+ missing vars, warnings); Curl()
internal/script        RunPre (set/unset) and RunTests (assertions and captures)
internal/cli           headless "list" and "run" commands
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

The workspace file is written with a temp file and rename, so a crash can't leave a half-written file. It is saved after every collection change, save, settings change, variable capture, when leaving the Variables tab, and on quit. On quit the builder state is stored as `Workspace.Draft` together with the position of the saved request it came from, and restored on the next start.

## Testing

- Unit tests cover variables, the request engine, scripts, storage and the HTTP client.
- `internal/cli` runs every sample request against `testutil.NewHTTPBin()` and requires all their tests to pass.
- `internal/ui/app_test.go` starts the real app on a `tcell.SimulationScreen`, injects keys and mouse clicks, and checks both state and screen contents. The harness waits for each key to reach the app's input handler before continuing, so the tests are deterministic, and they pass under `-race`.
