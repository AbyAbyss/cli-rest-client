# PowerShell build script for Windows
param(
    [string]$Platform = "windows",
    [string]$Arch = "amd64"
)

$APP_NAME = "term-rest-client"
$CMD_DIR = "cmd/term-rest-client"
$BIN_DIR = "bin"
$DIST_DIR = "dist"

function Build-Local {
    Write-Host "Building $APP_NAME..." -ForegroundColor Green
    if (-not (Test-Path $BIN_DIR)) {
        New-Item -ItemType Directory -Path $BIN_DIR | Out-Null
    }
    
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -o "$BIN_DIR\$APP_NAME.exe" "./$CMD_DIR"
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Build complete: $BIN_DIR\$APP_NAME.exe" -ForegroundColor Green
    } else {
        Write-Host "Build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

function Build-Cross {
    param([string]$os, [string]$arch, [string]$ext = "")
    
    Write-Host "Building for $os/$arch..." -ForegroundColor Cyan
    if (-not (Test-Path $DIST_DIR)) {
        New-Item -ItemType Directory -Path $DIST_DIR | Out-Null
    }
    
    $env:GOOS = $os
    $env:GOARCH = $arch
    $env:CGO_ENABLED = "0"
    $output = "$DIST_DIR\$APP_NAME-$os-$arch$ext"
    go build -o $output "./$CMD_DIR"
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "  ✓ $output" -ForegroundColor Green
    } else {
        Write-Host "  ✗ Failed to build for $os/$arch" -ForegroundColor Red
    }
}

function Build-All {
    Write-Host "Building for all platforms..." -ForegroundColor Yellow
    
    Build-Cross "linux" "amd64"
    Build-Cross "linux" "arm64"
    Build-Cross "windows" "amd64" ".exe"
    Build-Cross "darwin" "amd64"
    Build-Cross "darwin" "arm64"
    
    Write-Host "`nBuild complete for all platforms!" -ForegroundColor Green
}

# Main execution
switch ($Platform.ToLower()) {
    "all" {
        Build-All
    }
    "windows" {
        Build-Local
    }
    default {
        Write-Host "Usage: .\build.ps1 [windows|all]" -ForegroundColor Yellow
        Write-Host "  windows - Build for current Windows (default)"
        Write-Host "  all     - Build for all platforms"
    }
}


