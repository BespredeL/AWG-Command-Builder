# AWG Command Builder

[![Readme EN](https://img.shields.io/badge/README-EN-blue.svg)](./README.md)
[![Readme RU](https://img.shields.io/badge/README-RU-blue.svg)](./README_RU.md)
[![Go Version](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-windows-0078D6?logo=windows)](https://www.microsoft.com/windows)

🚀 Local Windows app for Keenetic/Netcraze RCI API.

AWG Command Builder runs as a native Windows desktop app (WebView2). Internally it uses a local service on `127.0.0.1:18080`, serves embedded UI, authenticates to the router, and executes RCI commands.

---

## ✨ Features

- 🔐 Keenetic RCI authentication with multiple hash strategies (firmware compatibility)
- 🧩 Interface-aware command builder for AWG/WireGuard ASC parameters
- 🖥 Single-file desktop-like app (`AWG-Command-Builder.exe`) with embedded UI and i18n
- 🌐 Multi-language UI (`languages.json`) with external override support
- 🧠 Runs fully inside a native WebView2 app window (no external browser tab)

---

## 📦 Requirements

- Go `1.22+`
- Windows 10/11

---

## 🚀 Run In Development

```powershell
go run .
```

The app opens its own window automatically (no external browser).

---

## 🛠 Build

### Standard build (console)

```powershell
go build -o "AWG-Command-Builder.exe" .
```

### GUI build (recommended, no console window)

```powershell
go build -ldflags="-H=windowsgui" -o "AWG-Command-Builder.exe" .
```

### One-command GUI build

```powershell
.\build-gui.bat
```

---

## 📖 Usage

1. Start `AWG-Command-Builder.exe`
2. The app window opens automatically (WebView2)
3. Enter router IP, login, password
4. Connect, fetch WireGuard interfaces, build and send command
5. Close the application window to stop the app

---

## 🌐 Internationalization

- Embedded language file: `i18n/languages.json`
- On startup, app checks `languages.json` next to `AWG-Command-Builder.exe`
- If external file exists and is valid JSON, it overrides embedded translations
- UI includes:
  - language selector
  - **Export languages.json from EXE** button

API endpoints:

- `GET /api/i18n` - active language config (embedded or external)
- `GET /api/i18n/export-exe` - download embedded language file

---

## 📁 Project Structure

- `main.go` - backend API + WebView2 desktop window + app lifecycle logic
- `index.html` - frontend UI
- `i18n/languages.json` - default embedded translations
- `build-gui.bat` - GUI build helper

