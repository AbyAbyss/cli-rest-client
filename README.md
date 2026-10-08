# Term REST Client

A terminal REST API client written in Go with [tview](https://github.com/rivo/tview). Build requests, organise them into collections, use variables, script simple pre-request steps and response tests, all without leaving the terminal. Saved requests can also be run from the command line, which makes them usable in CI.

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue.svg?style=for-the-badge)

![Term REST Client: request builder, JSON body and response with passing tests](assets/screenshots/main.png)

## Features

- **Request builder**: GET, POST, PUT, PATCH, DELETE, HEAD and OPTIONS, with a URL bar that autocompletes `{{variables}}`.
- **Params, Headers, Auth and Body tabs**: query parameters, headers, Basic / Bearer / API key auth, and JSON, text, XML or form-urlencoded bodies. Any line can be disabled by starting it with `#`.
- **Variables**: `{{name}}` works in the URL, params, headers, auth fields and body. Built-ins: `{{$uuid}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$randomInt}}`.
- **Pre-request scripts**: `set name = value` / `unset name` before a request is sent.
- **Tests**: one assertion per line (`status == 200`, `json.items[0].id exists`, `time < 500` ...), plus `set token = json.token` to capture values for the next request.
- **Collections and folders, like Postman**: group requests into collections, folders inside collections, and folders inside folders to any depth. Requests can also live at the top level. Create, rename, duplicate, reorder, move and delete anything. Everything is saved to a JSON file automatically.
- **Unsaved changes are never lost**: edits you haven't saved are kept when you quit and restored on the next start. Opening another request asks before discarding.
- **Response viewer**: status, timing, size, sorted headers, pretty-printed and colour-highlighted JSON, test results. Press `s` to save the body to a file.
- **Copy as cURL** (`Ctrl+G`), JSON formatter (`Ctrl+P`), request cancel (`Esc`).
- **Settings**: four themes (Catppuccin Mocha, Original, Gruvbox Dark, Light), timeout, redirect following, TLS verification.
- **Mouse support**: click any field or tab, scroll the response.
- **Headless mode**: `term-rest-client run "Collection/Folder/Request"` (or a whole folder) sends saved requests, prints results and exits non-zero when a test fails.

## Screenshots

| | |
|---|---|
| ![Query params with variables](assets/screenshots/params.png) | ![Bearer token auth](assets/screenshots/auth.png) |
| **Query params and headers** with `{{variables}}` and a generated `{{$uuid}}` | **Auth tab**: Basic, Bearer or API key |
| ![Pre-request script and tests](assets/screenshots/tests.png) | ![Save request dialog](assets/screenshots/save.png) |
| **Tests**: each line passes or fails with the actual value | **Save as**: pick a name and collection |
| ![Keyboard shortcuts](assets/screenshots/help.png) | ![Settings in the Gruvbox Dark theme](assets/screenshots/settings.png) |
| **F1** shows every shortcut | **Settings** with the Gruvbox Dark theme |
| ![Light theme](assets/screenshots/light.png) | ![Command-line mode](assets/screenshots/cli.png) |
| **Light** theme | **`run` from the command line**, usable in CI |
| ![Moving a request into another folder](assets/screenshots/move.png) | |
| **Folders**: nest them as deep as you like, and press `m` to move a request or folder | |

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
| `make build-linux` | Linux binaries for amd64 and arm64, in `dist/` |

Want a shorter command? Add an alias to `~/.zshrc`:

```bash
echo "alias rest='term-rest-client'" >> ~/.zshrc && source ~/.zshrc
rest
```

### Linux: install as a command you can run from any terminal

**1. Install the tools.** You need `git`, `make` and Go 1.21 or newer (Go downloads the 1.24 toolchain this project uses by itself). Distro Go packages are often older than that, so check with `go version` after installing.

| Distro | Command |
|--------|---------|
| Ubuntu / Debian | `sudo apt install git make` then `sudo snap install go --classic` |
| Fedora | `sudo dnf install git make golang` |
| Arch / Manjaro | `sudo pacman -S git make go` |
| openSUSE | `sudo zypper install git make go` |

No snap, or the packaged Go is too old? Use the official release (swap `amd64` for `arm64` on ARM machines such as a Raspberry Pi):

```bash
curl -LO https://go.dev/dl/go1.24.7.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.7.linux-amd64.tar.gz
echo 'export PATH="$PATH:/usr/local/go/bin"' >> ~/.bashrc && source ~/.bashrc
go version
```

**2. Build and install.**

```bash
git clone https://github.com/AbyAbyss/cli-rest-client.git
cd cli-rest-client
make install            # same as ./install.sh
```

On most Linux systems `/usr/local/bin` needs root, so the binary goes to `~/.local/bin` without asking for a password. Ubuntu, Debian and Fedora already put `~/.local/bin` on your `PATH` (you may need to open a new terminal, or log out and back in, the first time it's created). If it isn't on your `PATH`, the installer prints the line for your shell: `~/.bashrc` for bash, `~/.zshrc` for zsh, or `fish_add_path` for fish. For example:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

To install system-wide for all users instead (asks for your password once, for the copy):

```bash
INSTALL_DIR=/usr/local/bin make install
```

**3. Run it** from any terminal: `term-rest-client`. `make uninstall`, `git pull && make install` and the `rest` alias work exactly as on macOS (put the alias in `~/.bashrc` instead of `~/.zshrc` if you use bash).

**Servers without Go.** Build on any machine and copy the binary over. It's a single static file with no dependencies:

```bash
make build-linux        # dist/term-rest-client-linux-amd64 and -linux-arm64
scp dist/term-rest-client-linux-amd64 user@server:~/.local/bin/term-rest-client
```

Over SSH the UI works in any terminal; `term-rest-client run ...` is handy for scripted checks on servers.

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
| `Enter`, `Space` | Open a request, or expand / collapse a collection or folder |
| `Left` / `Right` | Collapse / expand. `Left` on a request jumps to its folder |
| `n` | New collection (top level) |
| `f` | New folder inside the selected collection or folder (or the folder of the selected request) |
| `a` (or `r`) | New request in the selected collection or folder. With nothing selected it goes to the top level |
| `m` | Move the selected request or folder to another collection, folder, or the top level |
| `e` (or `F2`) | Rename |
| `c` | Duplicate. Folders are copied with everything inside |
| `d` (or `Delete`) | Delete, after confirmation. Deleting a collection or folder deletes everything inside it |
| `Shift+Up` / `Shift+Down`, `K` / `J` | Move up / down within the same folder |

### Organising requests

The tree works like Postman's sidebar:

```
▾ User Service             collection
  ▾ Users                  folder
    ▾ Admin                folder inside a folder
        DELETE Delete User
      POST   Create User
      PUT    Update User
  ▾ Lookup
      GET    Get JSON
    POST   Form Login      request directly in the collection
▾ Payment Gateway
  ▾ Charges
      POST   Charge
  GET    Health Check      top-level request, not in any collection
```

- Inside each collection or folder, sub-folders are listed first, then requests.
- The request title above the URL shows the full path, for example `User Service / Users / Create User`, so you always know which API you're editing.
- Counts next to a collection or folder include everything in its sub-folders.
- **Save As** (`Alt+S`, or `Ctrl+S` on a new request) lets you pick any collection or folder, the top level, or a new collection.
- A folder moved to the top level with `m` becomes a collection, and a collection moved into another one becomes a folder.

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
term-rest-client run "User Service/Lookup/Get JSON"     # one request, by its full path
term-rest-client run "User Service/Users"               # everything in a folder, sub-folders included
term-rest-client run "Auth API" "Payment Gateway"       # whole collections, in order
term-rest-client run "Health Check"                     # a top-level request
term-rest-client run -v -set baseUrl=http://localhost:8080 "User Service"
```

Paths are `Collection/Folder/.../Request`, matched case-insensitively; `list` prints them. `run` executes pre-request scripts and tests just like the UI, prints a line per test, and exits with status 1 if a request fails or any test fails. `-v` also prints response headers and bodies. `-set name=value` overrides a variable for this run only. Values captured with `set` in tests are saved to the workspace.

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
make screenshots  # regenerate assets/screenshots (needs Node + Playwright)
```

The screenshots are generated, not hand-made: `make screenshots` drives the real UI on a simulated terminal against a local httpbin clone, writes each frame as HTML with the exact colors the app drew, and Playwright turns those into PNGs. Rerun it after UI changes.

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
