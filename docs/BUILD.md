# Build Instructions

This document provides detailed build instructions for all supported platforms.

## Prerequisites

- Go 1.24 or higher
- Git (for cloning the repository)

## Quick Build

### Windows

**Using Batch Script:**
```cmd
build.bat
```

**Using PowerShell:**
```powershell
.\build.ps1
```

**Using Go directly:**
```cmd
go build -o bin\term-rest-client.exe .\cmd\term-rest-client
```

### Linux / macOS

**Using Shell Script:**
```bash
chmod +x build.sh
./build.sh
```

**Using Go directly:**
```bash
go build -o bin/term-rest-client ./cmd/term-rest-client
```

## Cross-Platform Building

### Build for All Platforms

**Windows (PowerShell):**
```powershell
.\build.ps1 all
```

**Linux/macOS:**
```bash
./build.sh all
```

**Using Make:**
```bash
make build-all
```

### Build for Specific Platform

**Windows:**
```bash
make build-windows
# Output: dist/term-rest-client-windows-amd64.exe
```

**Linux:**
```bash
make build-linux
# Output: dist/term-rest-client-linux-amd64
```

**macOS (Intel):**
```bash
make build-darwin
# Output: dist/term-rest-client-darwin-amd64
#         dist/term-rest-client-darwin-arm64
```

## Manual Cross-Compilation

You can manually build for any platform using Go's cross-compilation:

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o dist/term-rest-client-windows-amd64.exe ./cmd/term-rest-client

# Linux
GOOS=linux GOARCH=amd64 go build -o dist/term-rest-client-linux-amd64 ./cmd/term-rest-client

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -o dist/term-rest-client-darwin-amd64 ./cmd/term-rest-client

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o dist/term-rest-client-darwin-arm64 ./cmd/term-rest-client
```

## Using Make

The Makefile provides convenient targets for common operations:

```bash
make help          # Show all available targets
make build         # Build for current platform
make run           # Build and run
make test          # Run tests
make fmt           # Format code
make lint          # Run linter
make clean         # Clean build artifacts
make build-all     # Build for all platforms
```

**Note:** On Windows, you may need to install Make:
- Using Chocolatey: `choco install make`
- Using Scoop: `scoop install make`
- Or download from: https://www.gnu.org/software/make/

## Troubleshooting

### Windows Issues

**Issue: "go: command not found"**
- Ensure Go is installed and added to PATH
- Verify installation: `go version`

**Issue: "make: command not found"**
- Install Make (see above) or use build scripts instead
- Use `build.bat` or `build.ps1` instead of Make

**Issue: Script execution policy (PowerShell)**
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### Linux/macOS Issues

**Issue: Permission denied on scripts**
```bash
chmod +x build.sh
chmod +x run.sh
```

**Issue: Build fails with "cannot find package"**
```bash
go mod download
go mod tidy
```

## Development Build

For development, you can use:

```bash
# Watch mode (requires additional tooling)
go run ./cmd/term-rest-client

# Or build and run
make build && make run
```

## Release Build

For release builds with version information:

```bash
# Set version and build time
VERSION=$(git describe --tags --always --dirty)
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')

# Build with version info
go build -ldflags "-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME" \
  -o bin/term-rest-client ./cmd/term-rest-client
```

The Makefile handles this automatically when you use `make build`.


