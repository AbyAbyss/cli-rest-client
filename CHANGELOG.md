# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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

[Unreleased]: https://github.com/AbyAbyss/cli-rest-client/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/AbyAbyss/cli-rest-client/releases/tag/v0.1.0


