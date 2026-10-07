# Term REST Client

A terminal REST API client written in Go with [tview](https://github.com/rivo/tview). Build requests, organise them into collections, use variables, script simple pre-request steps and response tests, all without leaving the terminal. Saved requests can also be run from the command line, which makes them usable in CI.

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue.svg?style=for-the-badge)

```
┌────────── Collections ─────────┐┌─ Method ──┐╔═══════════ User Service / Create User ════════════╗
│ ▾ Auth API (2)                 ││POST       │║ {{baseUrl}}/post                                  ║   SEND
│ ├── GET    Login               │└───────────┘╚═══════════════════════════════════════════════════╝
│ └── GET    Bearer Token        │ 1 Params   2 Auth   3 Headers   4 Body •   5 Pre-request   6 Tests •  ...
│ ▾ User Service (6)             │Body type JSON                    ┌──────── Response · 200 ─────────┐
│ ├── GET    Get JSON            │┌────────── Body · JSON ─────────┐│ POST https://httpbin.org/post   │
│ ├── GET    Query Params        ││ {                              ││                                 │
│ ├──●POST   Create User         ││   "name": "Aby",               ││ 200 OK   Time 412 ms   Size 1 KB│
│ ├── PUT    Update User         ││   "role": "admin",             ││                                 │
│ ├── POST   Form Login          ││   "createdAt": {{$timestamp}}  ││ Tests 2/2 passed                │
│ └── DELETE Delete User         ││ }                              ││   ✓ status == 200               │
│ ▾ Payment Gateway (1)          ││                                ││   ✓ json.json.name == Aby       │
│ └── POST   Charge              ││                                ││   → lastUser = "Aby"            │
```

## Features

- **Request builder**: GET, POST, PUT, PATCH, DELETE, HEAD and OPTIONS, with a URL bar that autocompletes `{{variables}}`.
- **Params, Headers, Auth and Body tabs**: query parameters, headers, Basic / Bearer / API key auth, and JSON, text, XML or form-urlencoded bodies. Any line can be disabled by starting it with `#`.
- **Variables**: `{{name}}` works in the URL, params, headers, auth fields and body. Built-ins: `{{$uuid}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$randomInt}}`.
- **Pre-request scripts**: `set name = value` / `unset name` before a request is sent.
- **Tests**: one assertion per line (`status == 200`, `json.items[0].id exists`, `time < 500` ...), plus `set token = json.token` to capture values for the next request.
- **Collections**: create, rename, duplicate, reorder and delete collections and requests. Everything is saved to a JSON file automatically.
- **Unsaved changes are never lost**: edits you haven't saved are kept when you quit and restored on the next start. Opening another request asks before discarding.
- **Response viewer**: status, timing, size, sorted headers, pretty-printed and colour-highlighted JSON, test results. Press `s` to save the body to a file.
- **Copy as cURL** (`Ctrl+G`), JSON formatter (`Ctrl+P`), request cancel (`Esc`).
- **Settings**: four themes (Catppuccin Mocha, Original, Gruvbox Dark, Light), timeout, redirect following, TLS verification.
- **Mouse support**: click any field or tab, scroll the response.
- **Headless mode**: `term-rest-client run "Collection/Request"` sends saved requests, prints results and exits non-zero when a test fails.

## Quick Start

You need Go 1.24 or newer and a terminal with 256-colour (ideally true-colour) support.

### macOS: install as a command you can run from any terminal

```bash
# 1. Install Go (skip if `go version` already works)
brew install go

# 2. Get the code
git clone https://github.com/AbyAbyss/cli-rest-client.git
cd cli-rest-client

# 3. Build the binary and put it on your PATH
make install            # same as ./install.sh
```

Then open any terminal window and run:

```bash
term-rest-client                          # launch the REST client
term-rest-client list                     # list saved requests
term-rest-client run "Auth API/Login"     # send a saved request without the UI
term-rest-client -version
```

`make install` picks the first of these it can write to: `/usr/local/bin`, `/opt/homebrew/bin` (Apple Silicon Homebrew), or `~/.local/bin`. If it ends up in a folder that isn't on your `PATH`, it prints the exact line to add to `~/.zshrc` (the default shell on macOS). For example:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Other install commands:

| Command | What it does |
|---------|--------------|
| `make install` | Build with version info and install to your `PATH` |
| `INSTALL_DIR=/usr/local/bin make install` | Install to a specific folder (asks for your password if it needs `sudo`) |
| `make uninstall` | Remove the installed binary. Your collections are kept |
| `git pull && make install` | Update to the latest version |
| `make build` | Just build `./bin/term-rest-client` without installing |
| `make build-mac-universal` | One binary for both Intel and Apple Silicon Macs, in `dist/` |

Want a shorter command? Add an alias to `~/.zshrc`:

```bash
echo "alias rest='term-rest-client'" >> ~/.zshrc && source ~/.zshrc
rest
```

### Linux

The same `make install` / `./install.sh` works on Linux.

### Windows

```cmd
build.bat
bin\term-rest-client.exe
```

See [docs/WINDOWS.md](docs/WINDOWS.md) for PowerShell and other options.

### With `go install`

```bash
go install github.com/AbyAbyss/cli-rest-client/cmd/term-rest-client@latest
```

This puts the binary in `$(go env GOPATH)/bin` (usually `~/go/bin`), which needs to be on your `PATH`.

### First run

On first start the workspace contains sample collections that call [httpbin.org](https://httpbin.org). Open one, press `Ctrl+R`, and you'll see the response and test results.

See [QUICKSTART.md](QUICKSTART.md) and [docs/BUILD.md](docs/BUILD.md) for more build options.

## Using the app

The screen has four areas: the **Collections tree** on the left, the **request bar** (method, URL, SEND) at the top, the **request tabs** below it, and the **Response** pane on the right.

| Tab | What it holds |
|-----|---------------|
| 1 Params | Query parameters, `key=value` per line. They are appended to whatever query string the URL already has. |
| 2 Auth | No Auth, Basic Auth, Bearer Token, or API Key (sent as a header or a query parameter). |
| 3 Headers | `Name: value` per line. |
| 4 Body | Body type (None, JSON, Text, XML, Form) and the body text. `Content-Type` is set from the type unless you set it in Headers. Form bodies are `key=value` per line. |
| 5 Pre-request | Script run before sending (see below). |
| 6 Tests | Assertions run on the response (see below). |
| 7 Variables | Workspace variables, `name=value` per line. Shared by all requests. |
| 8 Settings | Theme, timeout, redirects, TLS verification, version, data file location and the keyboard reference. |

Tabs that contain something show a marker, for example `Headers (2)` or `Tests •`. A `●` next to the request title means it has unsaved changes.

### Keyboard shortcuts

| Keys | Action |
|------|--------|
| `Ctrl+R`, `F5`, `Enter` in the URL field | Send the request (`Ctrl+Enter` also works in terminals that report it) |
| `Esc` | Cancel a running request, otherwise jump to the Collections tree |
| `Ctrl+S` | Save. The first save of a new request asks for a name and collection |
| `Alt+S` | Save as a new request |
| `Ctrl+N` | New empty request |
| `Ctrl+P` | Pretty-print the JSON body |
| `Ctrl+G`, `F4` | Show the request as a cURL command |
| `Tab` / `Shift+Tab` | Move between fields |
| `Alt+1` ... `Alt+8` | Switch tab. Plain `1`-`8` also works when you're not typing in a text field |
| `F1`, `?` | Help |
| `Ctrl+Q`, `Ctrl+C` | Quit |

In the **Collections tree**:

| Keys | Action |
|------|--------|
| `Enter`, `Space` | Open a request, or expand / collapse a collection |
| `Left` / `Right` | Collapse / expand a collection |
| `n` | New collection |
| `a` (or `r`) | New request in the selected collection |
| `e` (or `F2`) | Rename |
| `c` | Duplicate |
| `d` (or `Delete`) | Delete, after confirmation. Deleting a collection deletes its requests |
| `Shift+Up` / `Shift+Down`, `K` / `J` | Move up / down |

In the **Response** pane: arrow keys, `PgUp`/`PgDn` and `g`/`G` scroll, `s` saves the body to a file.

In dialogs: `Enter` confirms, `Tab` moves between fields, `Esc` cancels.

### Variables

Define variables in tab 7 as `name=value`, then use `{{name}}` anywhere in a request. The sample workspace defines `baseUrl` and `token`. Variables that can't be resolved are left as-is and listed as a warning above the response.

### Pre-request scripts

```
# lines starting with # are comments
set orderId = order-{{$randomInt}}
set auth = Bearer {{token}}
unset oldValue
```

`set` writes to the workspace variables, so the values show up in the Variables tab and stay available for later requests.

### Tests

One assertion per line. Each line is reported as passed (`✓`) or failed (`✗`) with the actual value.

```
status == 200
status < 300
time < 1000                       # milliseconds
size > 0                          # body size in bytes
header Content-Type contains json
header X-Rate-Limit exists
body contains "hello"
json.data.items[0].id == 42
json.data.items.length == 3
json.user.name matches ^Ab
json.error !exists
set token = json.token            # capture a value into a variable
```

Subjects: `status`, `time`, `size`, `body`, `header <Name>`, `json.<path>`. Operators: `==`, `!=`, `<`, `<=`, `>`, `>=`, `contains`, `matches` (regular expression), `exists`, and the negations `!contains`, `!matches`, `!exists`. Numbers compare numerically (`1.50 == 1.5`), everything else as text. The expected value may use `{{variables}}`.

## Command-line mode

```bash
term-rest-client list                                   # list saved requests
term-rest-client run "User Service/Get JSON"            # one request
term-rest-client run "Auth API" "Payment Gateway"       # whole collections, in order
term-rest-client run -v -set baseUrl=http://localhost:8080 "User Service"
```

`run` executes pre-request scripts and tests just like the UI, prints a line per test, and exits with status 1 if a request fails or any test fails. `-v` also prints response headers and bodies. `-set name=value` overrides a variable for this run only. Values captured with `set` in tests are saved to the workspace.

## Where data is stored

Collections, variables, settings and unsaved edits live in one JSON file:

| OS | Default location |
|----|------------------|
| Linux | `~/.config/term-rest-client/workspace.json` |
| macOS | `~/Library/Application Support/term-rest-client/workspace.json` |
| Windows | `%AppData%\term-rest-client\workspace.json` |

Use `-data path/to/file.json` or the `TERM_REST_CLIENT_DATA` environment variable to pick another file, for example one per project. The file is plain JSON, so it can be committed, shared or edited by hand. Settings shows the path in use.

## Project structure

```
cli-rest-client/
├── cmd/term-rest-client/   # entry point, flags, list/run commands dispatch
├── internal/
│   ├── cli/                # headless "list" and "run" commands
│   ├── engine/             # builds http.Request from a saved request + variables, cURL export
│   ├── models/             # workspace, collection and request types
│   ├── script/             # pre-request and test script interpreter
│   ├── storage/            # workspace file load/save, sample workspace
│   ├── testutil/           # local httpbin clone used by tests
│   ├── ui/                 # terminal UI (tview)
│   └── vars/               # {{variable}} substitution
├── pkg/httpclient/         # HTTP client with timing, redirects, TLS and cancel support
├── install.sh              # build and install to your PATH (macOS/Linux)
├── Makefile                # make build / install / uninstall / test
└── docs/
```

More detail in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Development

```bash
make install  # build and install to your PATH
make test     # go test ./...
make vet
make fmt
make lint     # needs golangci-lint
```

The UI tests drive the real application through tcell's simulation screen (typing, tab switching, saving, sending to a local test server), so `go test ./...` covers the interface as well as the HTTP and scripting code.

## Roadmap

- Request history
- Multiple named environments
- Import from Postman collections and cURL
- OAuth 2.0 helpers
- Proxy settings in the UI (the `HTTPS_PROXY` / `HTTP_PROXY` environment variables already work)

## Known limitations

- `Ctrl+Enter` is only distinguishable from `Enter` in terminals with extended keyboard reporting. Use `Ctrl+R` or `F5` elsewhere.
- Copying text uses your terminal's selection (usually `Shift` + drag while mouse support is on).
- Response bodies over 50 MB are truncated; display is capped at 2 MB.

## License

MIT, see [LICENSE](LICENSE).

## Acknowledgments

- [tview](https://github.com/rivo/tview), terminal UI library
- [tcell](https://github.com/gdamore/tcell), terminal cell library
