@echo off
REM ============================================================
REM  DSC Bridge - Windows one-click installer
REM ============================================================
REM  Double-click this file. It will:
REM    1. ask for Administrator permission (needed to trust the
REM       certificate, open the firewall port, and set auto-start),
REM    2. copy the bridge into your profile,
REM    3. run --post-install (cert trust + firewall + auto-start on login),
REM    4. start the bridge now.
REM  After this you never start it by hand again.
REM ============================================================
setlocal

REM --- Self-elevate to Administrator if we aren't already ---
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Requesting Administrator permission...
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

set "INSTALL_DIR=%LOCALAPPDATA%\dsc-bridge"
echo Installing DSC Bridge to "%INSTALL_DIR%" ...
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

copy /Y "%~dp0dsc-bridge.exe" "%INSTALL_DIR%\dsc-bridge.exe" >nul
if not exist "%INSTALL_DIR%\dsc-bridge.json" (
    if exist "%~dp0dsc-bridge.json" copy /Y "%~dp0dsc-bridge.json" "%INSTALL_DIR%\dsc-bridge.json" >nul
)

echo Trusting certificate, opening firewall, enabling auto-start...
"%INSTALL_DIR%\dsc-bridge.exe" --post-install

echo Starting DSC Bridge...
start "" "%INSTALL_DIR%\dsc-bridge.exe"

echo.
echo ============================================================
echo  DSC Bridge is installed and running.
echo  It will start automatically every time you log in.
echo  Plug in your DSC token and click "Sign with DSC" in the site.
echo ============================================================
echo.
pause
