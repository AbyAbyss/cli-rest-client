# Windows Build and Development Guide

This guide provides detailed instructions for building and running Term REST Client on Windows.

## Prerequisites

1. **Install Go**
   - Download from: https://golang.org/dl/
   - Version: 1.21 or higher (1.24+ recommended)
   - Add Go to your PATH during installation
   - Verify: `go version`

2. **Terminal Options**
   - **Windows Terminal** (Recommended): https://aka.ms/terminal
   - **PowerShell**: Built-in
   - **Command Prompt (CMD)**: Built-in
   - **Git Bash**: Comes with Git for Windows

## Building on Windows

### Method 1: Batch Script (CMD/Git Bash)

Open Command Prompt or Git Bash:

```cmd
# Navigate to project directory
cd term-rest-client

# Build
build.bat

# Run
run.bat
```

### Method 2: PowerShell Script

Open PowerShell:

```powershell
# Navigate to project directory
cd term-rest-client

# Build
.\build.ps1

# Run
.\bin\term-rest-client.exe
```

**Note:** If you get an execution policy error:
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### Method 3: Direct Go Command

Works in any terminal:

```cmd
# Build
go build -o bin\term-rest-client.exe cmd\term-rest-client\main.go

# Run
bin\term-rest-client.exe
```

### Method 4: Using Make

If you have Make installed:

```cmd
# Install Make (if needed)
# Using Chocolatey: choco install make
# Using Scoop: scoop install make

# Build
make build

# Run
make run
```

## Cross-Platform Building from Windows

Build for all platforms:

```powershell
.\build.ps1 all
```

This creates:
- `dist/term-rest-client-windows-amd64.exe`
- `dist/term-rest-client-linux-amd64`
- `dist/term-rest-client-darwin-amd64`
- `dist/term-rest-client-darwin-arm64`

## Development Workflow

### Quick Development Cycle

```cmd
# 1. Make changes to code

# 2. Build
build.bat

# 3. Test
bin\term-rest-client.exe

# Or use run.bat which builds automatically
run.bat
```

### Using Go Run (No Build Step)

```cmd
go run cmd/term-rest-client/main.go
```

### Format Code

```cmd
go fmt ./...
```

### Run Tests

```cmd
go test ./...
```

## Troubleshooting

### Issue: "go: command not found"

**Solution:**
1. Verify Go is installed: Check if `C:\Program Files\Go\bin` exists
2. Add to PATH:
   - Open System Properties → Environment Variables
   - Add `C:\Program Files\Go\bin` to PATH
   - Restart terminal

### Issue: PowerShell Execution Policy

**Error:** `cannot be loaded because running scripts is disabled`

**Solution:**
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### Issue: Build Script Not Found

**Solution:**
- Ensure you're in the project root directory
- Check that `build.bat` or `build.ps1` exists
- Use full path: `C:\path\to\project\build.bat`

### Issue: "cannot find package"

**Solution:**
```cmd
go mod download
go mod tidy
```

### Issue: Terminal Colors Not Working

**Solution:**
- Use Windows Terminal (recommended)
- Enable ANSI color support in your terminal
- Check terminal settings for color support

## IDE Setup

### Visual Studio Code

1. Install Go extension
2. Open project folder
3. Press F5 to debug, or use integrated terminal

### GoLand / IntelliJ IDEA

1. Open project
2. Configure Go SDK
3. Use built-in terminal or run configurations

## File Paths

Windows uses backslashes (`\`) in paths:
- Build output: `bin\term-rest-client.exe`
- Source: `cmd\term-rest-client\main.go`
- Config: `config\config.json` (if added)

## Environment Variables

Set Go-specific variables if needed:

```cmd
set GOOS=windows
set GOARCH=amd64
go build -o bin\term-rest-client.exe cmd\term-rest-client\main.go
```

## Next Steps

- See [BUILD.md](BUILD.md) for detailed build instructions
- See [CONTRIBUTING.md](../CONTRIBUTING.md) for contribution guidelines
- See [README.md](../README.md) for general usage

