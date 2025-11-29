@echo off
REM Build script for Windows
setlocal

set APP_NAME=term-rest-client
set CMD_DIR=cmd\term-rest-client
set BIN_DIR=bin

echo Building %APP_NAME%...
if not exist %BIN_DIR% mkdir %BIN_DIR%

go build -o %BIN_DIR%\%APP_NAME%.exe %CMD_DIR%\main.go

if %ERRORLEVEL% EQU 0 (
    echo Build complete: %BIN_DIR%\%APP_NAME%.exe
) else (
    echo Build failed!
    exit /b %ERRORLEVEL%
)

endlocal

