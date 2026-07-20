@echo off
REM PathWeaver Production Build Script (Windows)
REM Builds frontend + backend for release

echo === Building frontend ===
cd /d "%~dp0web"
call npm install
call npm run build

echo === Building Rust backend ===
cd /d "%~dp0"
cargo build --release -p pathweaver-controller

echo === Copying output ===
mkdir target\release\web-dist 2>nul
xcopy /E /Y web\dist\* target\release\web-dist\

echo === Build complete ===
echo Binary: target\release\pathweaver-controller.exe
echo Run:   set PW_STATIC_DIR=target\release\web-dist ^&^& target\release\pathweaver-controller.exe
pause
