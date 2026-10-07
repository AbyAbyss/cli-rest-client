# Quick Start Guide

## Windows

### Option 1: Batch Script (Easiest)
```cmd
build.bat
run.bat
```

### Option 2: PowerShell
```powershell
.\build.ps1
.\bin\term-rest-client.exe
```

### Option 3: Go Directly
```cmd
go build -o bin\term-rest-client.exe .\cmd\term-rest-client
bin\term-rest-client.exe
```

### Option 4: Make (if installed)
```cmd
make build
make run
```

## Linux / macOS

### Option 1: Shell Script (Easiest)
```bash
chmod +x build.sh run.sh
./build.sh
./run.sh
```

### Option 2: Go Directly
```bash
go build -o bin/term-rest-client ./cmd/term-rest-client
./bin/term-rest-client
```

### Option 3: Make
```bash
make build
make run
```

## Cross-Platform Build

Build for all platforms:
```bash
# Windows PowerShell
.\build.ps1 all

# Linux/macOS
./build.sh all

# Using Make
make build-all
```

## Troubleshooting

**Windows: "go: command not found"**
- Install Go from https://golang.org/dl/
- Add Go to your PATH

**Windows: PowerShell execution policy**
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

**Linux/macOS: Permission denied**
```bash
chmod +x build.sh run.sh
```

For more details, see [docs/BUILD.md](docs/BUILD.md)



## First Request

1. Start the app. The sample workspace opens `Auth API / Login`.
2. Press `Ctrl+R` (or `F5`) to send it. The response and test results appear on the right.
3. Pick another request in the Collections tree with the arrow keys and `Enter`.
4. Edit the URL or a tab, then press `Ctrl+S` to save. `Ctrl+N` starts a new request.
5. Press `F1` at any time for the full list of shortcuts, `Ctrl+Q` to quit.

Your collections are saved automatically. Run a saved request without the UI:

```bash
./bin/term-rest-client run "User Service/Get JSON"
```
