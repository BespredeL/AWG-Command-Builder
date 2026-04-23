@echo off
setlocal

echo Building AWG Command Builder (Windows GUI mode)...
go build -ldflags="-H=windowsgui" -o "AWG-Command-Builder.exe" .
if errorlevel 1 (
  echo.
  echo Build failed.
  exit /b 1
)

echo.
echo Done: AWG-Command-Builder.exe
exit /b 0

