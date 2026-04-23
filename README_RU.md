# AWG Command Builder

[![Readme EN](https://img.shields.io/badge/README-EN-blue.svg)](./README.md)
[![Readme RU](https://img.shields.io/badge/README-RU-blue.svg)](./README_RU.md)
[![Go Version](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-windows-0078D6?logo=windows)](https://www.microsoft.com/windows)

🚀 Локальное Windows-приложение для работы с RCI API Keenetic/Netcraze.

AWG Command Builder работает как нативное Windows-приложение (WebView2). Внутри используется локальный сервис на `127.0.0.1:18080`, встроенный UI, авторизация на роутере и выполнение RCI-команд.

---

## ✨ Возможности

- 🔐 Авторизация Keenetic RCI с несколькими схемами хэширования (совместимость с разными прошивками)
- 🧩 Удобный билдер команд AWG/WireGuard ASC
- 🖥 Single-file формат: `AWG-Command-Builder.exe` со встроенными UI и языками
- 🌐 Мультиязычность через `languages.json` с внешним переопределением
- 🧠 Полностью работает в нативном окне WebView2 (без внешней вкладки браузера)

---

## 📦 Требования

- Go `1.22+`
- Windows 10/11

---

## 🚀 Запуск в режиме разработки

```powershell
go run .
```

Окно приложения откроется автоматически (без внешнего браузера).

---

## 🛠 Сборка

### Обычная сборка (с консолью)

```powershell
go build -o "AWG-Command-Builder.exe" .
```

### GUI-сборка (рекомендуется, без консольного окна)

```powershell
go build -ldflags="-H=windowsgui" -o "AWG-Command-Builder.exe" .
```

### Сборка одной командой

```powershell
.\build-gui.bat
```

---

## 📖 Использование

1. Запустите `AWG-Command-Builder.exe`
2. Автоматически откроется окно приложения (WebView2)
3. Введите IP роутера, логин и пароль
4. Подключитесь, загрузите WireGuard-интерфейсы, сформируйте и отправьте команду
5. Для завершения просто закройте окно приложения

---

## 🌐 Мультиязычность

- Встроенный файл переводов: `i18n/languages.json`
- При старте приложение ищет `languages.json` рядом с `AWG-Command-Builder.exe`
- Если внешний файл найден и валиден, используется он
- В интерфейсе есть:
    - выбор языка
    - кнопка **Выгрузить languages.json из EXE**

API:

- `GET /api/i18n` - активная языковая конфигурация (встроенная или внешняя)
- `GET /api/i18n/export-exe` - выгрузка встроенного языкового файла

---

## 📁 Структура проекта

- `main.go` - backend API + окно WebView2 + жизненный цикл приложения
- `index.html` - frontend UI
- `i18n/languages.json` - встроенные переводы
- `build-gui.bat` - helper для GUI-сборки