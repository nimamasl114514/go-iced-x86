@echo off
setlocal
cd /d %~dp0
echo [1/4] cargo build --release
cd rust-iced
cargo build --release
if errorlevel 1 exit /b 1
cd ..
echo [2/4] copy DLL
copy /y rust-iced\target\release\iced_go_ffi.dll .\iced_go_ffi.dll >nul
echo [3/4] go mod tidy ^& build
set CGO_ENABLED=0
go mod tidy
if errorlevel 1 exit /b 1
go build ./...
if errorlevel 1 exit /b 1
echo [4/4] go test
go test -v .
if errorlevel 1 exit /b 1
echo BUILD OK

