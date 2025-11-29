@echo off
REM Run script for Windows
setlocal

set APP_NAME=term-rest-client
set BIN_DIR=bin

if not exist %BIN_DIR%\%APP_NAME%.exe (
    echo Building %APP_NAME% first...
    call build.bat
    if %ERRORLEVEL% NEQ 0 (
        echo Build failed, cannot run.
        exit /b %ERRORLEVEL%
    )
)

echo Running %APP_NAME%...
%BIN_DIR%\%APP_NAME%.exe

endlocal


