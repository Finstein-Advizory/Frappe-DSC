@echo off
REM ============================================================
REM  DSC Bridge - Windows uninstaller
REM ============================================================
REM  Removes auto-start, firewall rule, and the trusted certificate,
REM  stops the bridge, and deletes the installed files.
REM ============================================================
setlocal

REM --- Self-elevate to Administrator ---
net session >nul 2>&1
if %errorlevel% neq 0 (
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

set "INSTALL_DIR=%LOCALAPPDATA%\dsc-bridge"

echo Stopping DSC Bridge...
taskkill /IM dsc-bridge.exe /F >nul 2>&1

if exist "%INSTALL_DIR%\dsc-bridge.exe" (
    echo Removing auto-start, firewall rule, trusted certificate...
    "%INSTALL_DIR%\dsc-bridge.exe" --pre-uninstall
)

echo Deleting files...
del /Q "%INSTALL_DIR%\dsc-bridge.exe" >nul 2>&1

echo.
echo DSC Bridge has been removed. (Config/certs kept in "%INSTALL_DIR%".)
echo.
pause
