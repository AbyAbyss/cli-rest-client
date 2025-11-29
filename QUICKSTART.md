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
go build -o bin\term-rest-client.exe cmd\term-rest-client\main.go
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
go build -o bin/term-rest-client cmd/term-rest-client/main.go
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


