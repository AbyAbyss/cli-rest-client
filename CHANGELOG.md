# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Streaming responses: Server-Sent Events and NDJSON are shown live, each event with
  its arrival time, type and id. The timeout stops at the headers, `Esc` stops the
  stream and keeps what arrived, and History keeps it. Test subjects `events` and
  `event[i]` / `event[i].json.path`. `run` follows streams until `-stream` runs out.
- GraphQL: a GraphQL body type with Query and Variables editors, sent as JSON (or in
  the URL for GET). `F6` browses the schema by introspection and writes a query with
  variables for a field. Code snippets, curl import/export and Postman's graphql body
  mode all handle it.
- WebSocket requests (WS in the method dropdown): connect with params, headers and
  auth in the handshake, send the Body tab as messages, and watch a live sent/received
  log; tests run on the received messages when the connection ends. `run` connects,
  sends and collects replies for CI. Snippets in JavaScript, Python, Go and websocat.
- Sample "API Types" collection: GraphQL (countries API), SSE (Wikimedia live edits)
  and WebSocket (Postman echo).

## [0.2.0] - 2026-10-09

### Added
- Prebuilt binaries: `make release` / `scripts/release.sh` build macOS, Linux and
  Windows archives with SHA256SUMS; a Release workflow publishes them on a `v*` tag;
  `get.sh` installs the right one with a single `curl ... | sh`, no Go needed.
- Code generation: the `Ctrl+G` window shows the request as cURL, Python (requests),
  JavaScript (fetch), Go (net/http) or HTTPie; `←`/`→` switch language and the choice
  is remembered for `y`. Real values or `{{variables}}` as named variables in the code.
  CLI: `term-rest-client export -lang LANG [-raw]`. Each snippet is run against a test
  server in the test suite and must send the same request as the app.
- Copy as cURL: `c` in the `Ctrl+G` window copies the command to the clipboard and `v`
  switches between filled-in values and `{{variables}}`; `y` in the Collections tree
  and History copies a request directly. Uses pbcopy, wl-copy, xclip, xsel or clip,
  and OSC 52 over SSH. CLI: `term-rest-client export -curl [-raw]`.
- Import from cURL: paste a curl command into the URL field (or `Ctrl+O`) to get a
  request with method, params, headers, auth and body; several commands become a
  collection. Handles browser "Copy as cURL" output ($'...' quoting, continuations,
  --data-raw, --compressed, cookies), -u / bearer headers to Auth, --json, -G, -F,
  --data-urlencode. CLI: `term-rest-client curl` and curl files in `import`.
- Export to Postman: collections and folders as Collection v2.1 files (`x` in the tree),
  environments and globals (Export button in the Variables tab), and
  `term-rest-client export`. Requests imported from Postman get their original scripts
  back; scripts written here are translated into `pm.test` JavaScript with matching
  comparison rules. Import and export round-trip.
- Import from Postman: collections v2.0/v2.1 (nested folders, inherited auth, raw /
  urlencoded / form-data / GraphQL bodies, query and path variables, collection
  variables), environments and globals. Common test/pre-request script patterns are
  converted; the original JavaScript is kept as comments. `Ctrl+O` (or `i` in the
  tree) in the app, `term-rest-client import FILE...` on the command line.
- Environments, like Postman: named variable sets layered over Globals. Environment
  picker next to the URL and `Alt+E` to switch; the Variables tab edits Globals or any
  environment and can create, rename, duplicate, delete and activate them. The response
  pane and History show which environment was used. CLI: `run -env NAME` and
  `env [NAME|none]`. Sample workspace has httpbin.org and Local environments.
- Request history, like Postman: a History view in the sidebar (Alt+H / Alt+C / F3,
  or click), grouped by day. Open an entry to get the request back with the response
  it got, save it to a collection, filter, delete, or clear all. Stored in
  `<workspace>.history.json`; newest 200 entries, bodies up to 64 KB; can be turned
  off in Settings. `term-rest-client history [-n N]` lists it.
- Response pane shows the request headers that were actually sent (including
  Authorization, Content-Type, User-Agent, Host, Content-Length, Accept-Encoding).
- Tests, Request Headers, Response Headers and Body are foldable sections: keys
  `t` / `r` / `h` / `b` in the response pane, or click the heading. Choices are saved.
- `run -v` prints sent (`>`) and received (`<`) headers like `curl -v`.
- Folders inside collections, nested to any depth, and requests at the top level outside
  any collection (like Postman). New tree keys: `f` new folder, `m` move to another
  collection/folder/top level; `Left` on a request jumps to its folder.
- Save As can save into any folder or the top level; the request title shows the full path.
- `list` prints full paths and `run` accepts folder paths (runs everything inside).
- Workspace file version 2 (`folders` and top-level `requests`). Version 1 files load
  as before and the saved draft link is migrated.
- Params, Auth, Headers, Body, Pre-request and Tests tabs now work (they were placeholders).
- Auth: Basic, Bearer token and API key (header or query parameter).
- Body types: JSON, text, XML and form-urlencoded, with automatic `Content-Type`.
- Workspace variables (`{{name}}`) with a Variables tab, URL autocompletion and built-ins
  `{{$uuid}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$randomInt}}`.
- Pre-request scripts (`set` / `unset`) and response tests with value capture.
- Collections, variables and settings are saved to a JSON workspace file (`-data`,
  `TERM_REST_CLIENT_DATA`), and unsaved edits are restored on the next start.
- Save / Save As dialog, unsaved-changes prompt, duplicate and reorder in the tree.
- Response pane: status colours, size, sorted headers, highlighted JSON, test results,
  save body to file.
- Copy as cURL, JSON formatter, request cancellation, HEAD and OPTIONS methods.
- Settings: Gruvbox Dark and Light themes, timeout, redirect following, TLS verification.
- `install.sh` with `make install` / `make uninstall` to put the binary on your PATH
  (macOS and Linux, with shell-specific PATH hints and a Go version check),
  `make build-mac-universal`, and linux/arm64 builds.
- Headless `list` and `run` commands with test-based exit codes, plus `-version`.
- README screenshots generated from the real UI (`make screenshots`).
- Test suite covering the engine, scripting, storage, CLI and the UI (simulated terminal).

### Changed
- Code split out of a single `main.go` into `internal/{ui,engine,script,vars,storage,cli}`.
- Module path is now `github.com/AbyAbyss/cli-rest-client`, so `go install` works.
- Tabs: Builder is gone; the response pane is always visible next to the active tab.
  Tab 7 is Variables.
- Send is `Ctrl+R` / `F5` / `Enter` in the URL field, save is `Ctrl+S`, save as is `Alt+S`.

### Fixed
- cURL output for HEAD requests uses `-I`, since `-X HEAD` makes curl wait for a body.
- Typing digits in the URL, body or any text field switched tabs instead of inserting text.
- `Esc` quit the application, including from dialogs and dropdowns.
- `Ctrl+S`, `Ctrl+Q` and `Ctrl+Shift+S` never fired in real terminals; saving was unreachable.
- Rename and delete dialogs replaced the whole screen, and Esc in them quit the app.
- Response text containing `[...]` (common in JSON) was eaten as colour tags.
- Status line printed the code twice ("200 200 OK").
- Collections and edits were lost on exit.
- The method dropdown could not be reached with `Tab`.
- Dialog backgrounds let the screen underneath show through between fields.
- Makefile `build` target mixed Windows `cmd` syntax into a shell recipe.

## [0.1.0] - 2024-XX-XX

### Added
- Initial release

---

[Unreleased]: https://github.com/AbyAbyss/cli-rest-client/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/AbyAbyss/cli-rest-client/releases/tag/v0.2.0
[0.1.0]: https://github.com/AbyAbyss/cli-rest-client/releases/tag/v0.1.0


