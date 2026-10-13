# Santral-C Windows Desktop Mini App

English | [Türkçe](README.tr.md)

A lightweight, standalone Windows desktop mini phone widget that mirrors and controls the active softphone session in the Santral-C web panel in real time. It runs frameless, translucent, draggable, and always-on-top, keeping call controls accessible anywhere on Windows without switching browser tabs.

---

## Architecture & Integration

- **Zero-overhead SIP session sharing:** The browser panel owns the SIP registration, microphone access, and WebRTC audio streams. The desktop app acts as an ultra-lightweight remote control with zero server load and no duplicate SIP logins.
- **Local WebSocket Bridge (`127.0.0.1:48123`):** Bidirectional real-time bridge connecting the desktop widget and the active panel tab.
- **Configurable Origins & Security:**
  - Binds strictly to `127.0.0.1` (loopback only).
  - Validates `Origin` on HTTP Upgrade. Origins can be configured dynamically via `widget-config.json` (`panelUrl`), environment variables (`SANTRAL_PANEL_ORIGINS` or `SANTRAL_PANEL_URL`), and defaults to HTTPS / localhost.
  - Context isolation enabled in Electron (`nodeIntegration: false`, `contextIsolation: true`).

---

## Features

- **Always-on-top & Frameless:** Floats seamlessly over all windows and full-screen workspaces.
- **Position Persistence:** Remembers its last dragged coordinates across app restarts.
- **Dynamic Window Resizing:** Automatically expands and contracts when toggling the DTMF dial pad or call transfer drawer.
- **Windows System Tray & Menu:** Minimizes cleanly to the Windows notification tray with toggle, open panel, auto-start, and exit options.
- **Global Hotkeys (System-wide):**
  - <kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>A</kbd>: Answer call
  - <kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>H</kbd>: Hangup / Reject call
  - <kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>M</kbd>: Toggle microphone mute
- **Native Windows Toast Notifications:** Alerts the user on incoming calls even when minimized or unfocused.
- **Quick Controls:** Dial pad with prefix toggle (+90 / Extension), Mute, Hold, DTMF keypad, Transfer, live call timer, and one-click peer number copy.

---

## Getting Started

### Development
```bash
cd desktop
npm install
npm start
```

### Run Tests
```bash
cd desktop
npm test
```

### Build & Package (.exe)
Produce standalone Windows executables:

```bash
cd desktop

# Builds both Portable and Installer (.exe) into desktop/dist/
npm run dist

# Or build specifically:
npm run dist:portable    # Outputs: Santral-C-Widget-Portable.exe
npm run dist:installer   # Outputs: Santral-C Mini Widget Setup 1.0.0.exe
```

### Configuration
Optionally configure the web panel URL or allowed origins via environment variable or in `widget-config.json` inside Electron's `userData` directory:

```bash
# Environment variable:
set SANTRAL_PANEL_URL=https://santral.example.com
npm start
```
