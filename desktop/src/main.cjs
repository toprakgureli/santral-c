const { app, BrowserWindow, Tray, Menu, ipcMain, screen, Notification, globalShortcut, session, nativeTheme, dialog } = require("electron");
const path = require("node:path");
const fs = require("node:fs");

const THEME_MAP = {
  light: { bg: "#f8f9fb", isDark: false },
  dark: { bg: "#1e2128", isDark: true },
  midnight: { bg: "#151a2b", isDark: true },
  forest: { bg: "#171f1b", isDark: true },
  sand: { bg: "#f6f1e8", isDark: false },
  crimson: { bg: "#1c1114", isDark: true },
  matte: { bg: "#000000", isDark: true },
};

function getInitialTheme() {
  const cfg = loadConfig();
  if (cfg.lastTheme && THEME_MAP[cfg.lastTheme]) {
    return { theme: cfg.lastTheme, ...THEME_MAP[cfg.lastTheme] };
  }
  const isDark = nativeTheme.shouldUseDarkColors;
  return {
    theme: isDark ? "dark" : "light",
    bg: isDark ? "#1e2128" : "#f8f9fb",
    isDark,
  };
}

let BridgeServerClass;
async function loadBridge() {
  const mod = await import("./bridgeServer.js");
  BridgeServerClass = mod.BridgeServer;
}

const DEFAULT_PANEL_URL = "https://cm.toprakgureli.com";

let panelWindow = null;
let widgetWindow = null;
let tray = null;
let bridgeServer = null;
let isQuitting = false;
let currentCallState = { status: "disconnected" };

const CONFIG_FILE = path.join(app.getPath("userData"), "widget-config.json");

function loadConfig() {
  try {
    if (fs.existsSync(CONFIG_FILE)) {
      return JSON.parse(fs.readFileSync(CONFIG_FILE, "utf8"));
    }
  } catch {}
  return {};
}

function saveConfig(cfg) {
  try {
    const existing = loadConfig();
    fs.writeFileSync(CONFIG_FILE, JSON.stringify({ ...existing, ...cfg }, null, 2), "utf8");
  } catch {}
}

function loadBounds() {
  return loadConfig().bounds || null;
}

function saveBounds(bounds) {
  saveConfig({ bounds });
}

function getPanelUrl() {
  return process.env.SANTRAL_PANEL_URL || loadConfig().panelUrl || DEFAULT_PANEL_URL;
}

const gotTheLock = app.requestSingleInstanceLock();
if (!gotTheLock) {
  app.quit();
} else {
  if (process.platform === "win32") {
    app.setAppUserModelId("com.santralc.desktop");
  }

  app.on("second-instance", () => {
    if (panelWindow) {
      if (panelWindow.isMinimized()) panelWindow.restore();
      panelWindow.show();
      panelWindow.focus();
    }
    if (widgetWindow) {
      widgetWindow.show();
      widgetWindow.focus();
    }
  });

  app.whenReady().then(async () => {
    // Setup automatic WebRTC microphone / media permissions
    session.defaultSession.setPermissionRequestHandler((webContents, permission, callback) => {
      if (permission === "media" || permission === "notifications") {
        callback(true);
      } else {
        callback(true);
      }
    });

    await loadBridge();
    createWidgetWindow();
    createPanelWindow();
    createTray();
    registerShortcuts();
    startServer();
    initAutoUpdater();
  });
}

function getDefaultPosition(width, height) {
  const primary = screen.getPrimaryDisplay();
  const workArea = primary.workArea;
  const x = Math.round(workArea.x + workArea.width - width - 24);
  const y = Math.round(workArea.y + workArea.height - height - 24);
  return { x, y };
}

function getAppIcon() {
  return process.platform === "win32"
    ? path.join(__dirname, "..", "assets", "icon.ico")
    : path.join(__dirname, "..", "assets", "icon.png");
}

function createWidgetWindow() {
  const width = 292;
  const height = 180;
  const saved = loadBounds();
  const def = getDefaultPosition(width, height);

  const x = saved && saved.x != null ? saved.x : def.x;
  const y = saved && saved.y != null ? saved.y : def.y;

  widgetWindow = new BrowserWindow({
    width,
    height,
    x,
    y,
    icon: getAppIcon(),
    frame: false,
    transparent: true,
    alwaysOnTop: true,
    resizable: false,
    skipTaskbar: true,
    backgroundColor: "#00000000",
    hasShadow: true,
    webPreferences: {
      preload: path.join(__dirname, "preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  widgetWindow.setSkipTaskbar(true);
  widgetWindow.on("show", () => {
    widgetWindow?.setSkipTaskbar(true);
  });

  widgetWindow.loadFile(path.join(__dirname, "widget", "index.html"));

  widgetWindow.on("moved", () => {
    if (widgetWindow) {
      saveBounds(widgetWindow.getBounds());
    }
  });

  widgetWindow.on("close", (e) => {
    if (!isQuitting) {
      e.preventDefault();
      widgetWindow.hide();
    }
  });
}

function createPanelWindow() {
  const initTheme = getInitialTheme();
  nativeTheme.themeSource = initTheme.isDark ? "dark" : "light";

  panelWindow = new BrowserWindow({
    width: 1366,
    height: 840,
    minWidth: 960,
    minHeight: 640,
    icon: getAppIcon(),
    title: "Santral-C",
    show: true,
    autoHideMenuBar: true,
    backgroundColor: initTheme.bg,
    webPreferences: {
      preload: path.join(__dirname, "panelPreload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  const url = getPanelUrl();
  panelWindow.loadURL(url).catch((err) => {
    console.error("Failed to load panel URL:", url, err);
  });

  panelWindow.on("close", (e) => {
    if (!isQuitting) {
      e.preventDefault();
      panelWindow.hide();
    }
  });
}

function showPanelWindow() {
  if (!panelWindow) return;
  if (panelWindow.isMinimized()) panelWindow.restore();
  panelWindow.show();
  panelWindow.focus();
}

function togglePanelWindow() {
  if (!panelWindow) return;
  if (panelWindow.isVisible() && !panelWindow.isMinimized()) {
    panelWindow.hide();
  } else {
    showPanelWindow();
  }
}

function broadcastCallState(state) {
  currentCallState = state;
  if (widgetWindow && !widgetWindow.isDestroyed()) {
    widgetWindow.webContents.send("call-state", state);
  }

  // On incoming call: ensure widget is visible and show Windows notification
  if (state.status === "incoming") {
    if (widgetWindow && !widgetWindow.isVisible()) {
      widgetWindow.showInactive();
    }
    if (Notification.isSupported()) {
      const notif = new Notification({
        title: "Gelen Çağrı",
        body: `${state.peer || "Bilinmeyen numara"} arıyor...`,
        silent: false,
      });
      notif.on("click", () => {
        showPanelWindow();
      });
      notif.show();
    }
  }
}

function sendCommandToPanel(cmd, arg) {
  // 1. Send directly to embedded panel window
  if (panelWindow && !panelWindow.isDestroyed()) {
    panelWindow.webContents.send("panel-command", { cmd, arg });
  }
  // 2. Also send to bridgeServer (for external browser connections)
  bridgeServer?.sendCommand(cmd, arg);
}

function quitApplication() {
  isQuitting = true;
  try {
    globalShortcut.unregisterAll();
  } catch {}
  try {
    bridgeServer?.stop();
  } catch {}
  if (widgetWindow && !widgetWindow.isDestroyed()) {
    try {
      widgetWindow.destroy();
    } catch {}
    widgetWindow = null;
  }
  if (panelWindow && !panelWindow.isDestroyed()) {
    try {
      panelWindow.destroy();
    } catch {}
    panelWindow = null;
  }
  if (tray && !tray.isDestroyed()) {
    try {
      tray.destroy();
    } catch {}
    tray = null;
  }
  app.exit(0);
}

let isPromptOpen = false;

async function showExitPrompt() {
  if (isPromptOpen) {
    return;
  }
  isPromptOpen = true;

  try {
    const options = {
      type: "question",
      buttons: [
        "Tümünü Kapat (Tam Çıkış)",
        "Sadece Mini Widget'ı Kapat",
        "Sadece Ana Paneli Kapat",
        "İptal",
      ],
      defaultId: 0,
      cancelId: 3,
      title: "Santral-C Kapatma Seçenekleri",
      message: "Santral-C Kapatma Seçenekleri",
      detail:
        "Hangi bileşeni kapatmak istiyorsunuz?\n\n" +
        "• Tümünü Kapat: Hem ana paneli hem mini widget'ı tamamen kapatır.\n" +
        "• Sadece Mini Widget'ı Kapat: Widget ekrandan kaldırılır, ana panel açık kalır.\n" +
        "• Sadece Ana Paneli Kapat: Ana panel kapatılır, mini widget çağrılar için ekranda kalır.",
      noLink: true,
    };

    // Do NOT pass parent window: passing a frameless transparent window or non-foreground window
    // causes modal lockups and disables the window on Windows.
    const result = await dialog.showMessageBox(options);

    if (result.response === 0) {
      // Tümünü Kapat (Tam Çıkış)
      quitApplication();
    } else if (result.response === 1) {
      // Sadece Mini Widget'ı Kapat
      if (widgetWindow && !widgetWindow.isDestroyed()) {
        widgetWindow.hide();
      }
      showPanelWindow();
    } else if (result.response === 2) {
      // Sadece Ana Paneli Kapat
      if (panelWindow && !panelWindow.isDestroyed()) {
        panelWindow.hide();
      }
      if (widgetWindow && !widgetWindow.isDestroyed() && !widgetWindow.isVisible()) {
        widgetWindow.show();
        widgetWindow.focus();
      }
    }
  } catch (err) {
    console.error("showExitPrompt error:", err);
  } finally {
    isPromptOpen = false;
  }
}

let autoUpdater = null;
function getAutoUpdater() {
  if (!autoUpdater) {
    try {
      const mod = require("electron-updater");
      autoUpdater = mod.autoUpdater;
    } catch (e) {
      console.warn("electron-updater not available:", e);
    }
  }
  return autoUpdater;
}

let updateCheckInProgress = false;

function initAutoUpdater() {
  if (!app.isPackaged) return;

  const updater = getAutoUpdater();
  if (!updater) return;

  updater.autoDownload = false;
  updater.autoInstallOnAppQuit = true;

  updater.on("update-available", (info) => {
    updateCheckInProgress = false;
    const version = info?.version || "Yeni Sürüm";
    dialog.showMessageBox({
      type: "info",
      title: "Santral-C Güncelleme Mevcut",
      message: `Yeni bir sürüm mevcut: v${version}`,
      detail: "Güncelleme paketini arka planda indirmek istiyor musunuz?",
      buttons: ["Şimdi İndir", "Daha Sonra"],
      defaultId: 0,
      cancelId: 1,
      noLink: true,
    }).then((res) => {
      if (res.response === 0) {
        updater.downloadUpdate().catch((err) => {
          console.error("downloadUpdate error:", err);
        });
        if (Notification.isSupported()) {
          new Notification({
            title: "Santral-C Güncellemesi",
            body: `v${version} arka planda indiriliyor...`,
          }).show();
        }
      }
    });
  });

  updater.on("update-not-available", () => {
    if (updateCheckInProgress) {
      updateCheckInProgress = false;
      dialog.showMessageBox({
        type: "info",
        title: "Santral-C Güncel",
        message: "Uygulamanız güncel",
        detail: `En son sürümü (v${app.getVersion()}) kullanıyorsunuz.`,
        buttons: ["Tamam"],
        noLink: true,
      });
    }
  });

  updater.on("error", (err) => {
    if (updateCheckInProgress) {
      updateCheckInProgress = false;
      dialog.showMessageBox({
        type: "warning",
        title: "Santral-C Güncelleme Denetimi",
        message: "Güncelleme denetlenirken bir sorun oluştu",
        detail: err ? (err.message || String(err)) : "GitHub sunucusuna ulaşılamadı.",
        buttons: ["Tamam"],
        noLink: true,
      });
    }
  });

  updater.on("update-downloaded", (info) => {
    const version = info?.version || "";
    dialog.showMessageBox({
      type: "question",
      title: "Santral-C Güncelleme Hazır",
      message: `Santral-C v${version} hazır!`,
      detail: "Güncellemenin tamamlanması için uygulamanın yeniden başlatılması gerekiyor. Şimdi yeniden başlatmak ister misiniz?",
      buttons: ["Şimdi Yeniden Başlat", "Daha Sonra"],
      defaultId: 0,
      cancelId: 1,
      noLink: true,
    }).then((res) => {
      if (res.response === 0) {
        isQuitting = true;
        updater.quitAndInstall();
      }
    });
  });

  // Check 5 seconds after startup
  setTimeout(() => {
    updater.checkForUpdates().catch(() => {});
  }, 5000);
}

function checkForUpdatesManually() {
  if (!app.isPackaged) {
    dialog.showMessageBox({
      type: "info",
      title: "Santral-C",
      message: "Geliştirici Modu",
      detail: "Otomatik güncelleme denetimi paketlenmiş (.exe) sürümde çalışır.",
      buttons: ["Tamam"],
      noLink: true,
    });
    return;
  }
  const updater = getAutoUpdater();
  if (!updater) return;

  updateCheckInProgress = true;
  updater.checkForUpdates().catch((err) => {
    updateCheckInProgress = false;
    dialog.showMessageBox({
      type: "warning",
      title: "Santral-C",
      message: "Güncelleme denetlenemedi.",
      detail: err ? (err.message || String(err)) : "Sunucuya bağlanılamadı.",
      buttons: ["Tamam"],
      noLink: true,
    });
  });
}

function createTray() {
  const trayIcon = process.platform === "win32"
    ? path.join(__dirname, "..", "assets", "icon.ico")
    : path.join(__dirname, "..", "assets", "icon32.png");
  try {
    tray = new Tray(trayIcon);
  } catch {
    try {
      tray = new Tray(path.join(__dirname, "..", "assets", "icon32.png"));
    } catch {
      return;
    }
  }

  const contextMenu = Menu.buildFromTemplate([
    {
      label: "Santral Panelini Aç / Göster",
      click: () => {
        showPanelWindow();
      },
    },
    {
      label: "Mini Widget'ı Göster / Gizle",
      click: () => {
        if (!widgetWindow) return;
        if (widgetWindow.isVisible()) {
          widgetWindow.hide();
        } else {
          widgetWindow.show();
          widgetWindow.focus();
        }
      },
    },
    { type: "separator" },
    {
      label: "Sadece Mini Widget'ı Kapat",
      click: () => {
        if (widgetWindow && !widgetWindow.isDestroyed()) {
          widgetWindow.hide();
        }
        showPanelWindow();
      },
    },
    {
      label: "Sadece Ana Paneli Kapat",
      click: () => {
        if (panelWindow && !panelWindow.isDestroyed()) {
          panelWindow.hide();
        }
        if (widgetWindow && !widgetWindow.isDestroyed() && !widgetWindow.isVisible()) {
          widgetWindow.show();
          widgetWindow.focus();
        }
      },
    },
    { type: "separator" },
    {
      label: "Windows ile Birlikte Başlat",
      type: "checkbox",
      checked: app.getLoginItemSettings().openAtLogin,
      click: (item) => {
        app.setLoginItemSettings({
          openAtLogin: item.checked,
          path: process.execPath,
        });
      },
    },
    {
      label: "Güncellemeleri Denetle...",
      click: () => {
        checkForUpdatesManually();
      },
    },
    {
      label: `Sürüm: v${app.getVersion()}`,
      enabled: false,
    },
    { type: "separator" },
    {
      label: "Çıkış Yap...",
      click: () => {
        showExitPrompt();
      },
    },
    {
      label: "Tümünü Kapat (Tam Çıkış)",
      click: () => {
        quitApplication();
      },
    },
  ]);

  tray.setToolTip("Santral-C Desktop Softphone");
  tray.setContextMenu(contextMenu);
  tray.on("click", () => {
    showPanelWindow();
  });
  tray.on("double-click", () => {
    showPanelWindow();
  });
}

function registerShortcuts() {
  // Ctrl+Alt+A: Answer call
  globalShortcut.register("CommandOrControl+Alt+A", () => {
    sendCommandToPanel("answer");
  });
  // Ctrl+Alt+H: Hangup call
  globalShortcut.register("CommandOrControl+Alt+H", () => {
    sendCommandToPanel("hangup");
  });
  // Ctrl+Alt+M: Toggle mute
  globalShortcut.register("CommandOrControl+Alt+M", () => {
    sendCommandToPanel("mute");
  });
  // Ctrl+Alt+P: Toggle full panel window
  globalShortcut.register("CommandOrControl+Alt+P", () => {
    togglePanelWindow();
  });
}

async function startServer() {
  const cfg = loadConfig();
  const origins = [];
  const panelUrl = getPanelUrl();
  if (panelUrl) {
    try {
      origins.push(new URL(panelUrl).origin);
    } catch {}
  }
  bridgeServer = new BridgeServerClass({ allowedOrigins: origins });
  await bridgeServer.start();

  bridgeServer.onState((state) => {
    broadcastCallState(state);
  });
}

// IPC Handlers
ipcMain.on("panel-state-update", (_event, state) => {
  broadcastCallState(state);
});

ipcMain.on("panel-heartbeat", () => {
  // Panel is alive
});

ipcMain.on("panel-theme-change", (_event, { theme, bg, isDark }) => {
  if (theme) {
    saveConfig({ lastTheme: theme });
  }
  const resolvedDark = typeof isDark === "boolean" ? isDark : true;
  nativeTheme.themeSource = resolvedDark ? "dark" : "light";

  if (panelWindow && !panelWindow.isDestroyed()) {
    if (bg) {
      panelWindow.setBackgroundColor(bg);
    }
  }
});

nativeTheme.on("updated", () => {
  const cfg = loadConfig();
  if (!cfg.lastTheme) {
    const isDark = nativeTheme.shouldUseDarkColors;
    nativeTheme.themeSource = isDark ? "dark" : "light";
    if (panelWindow && !panelWindow.isDestroyed()) {
      panelWindow.setBackgroundColor(isDark ? "#1e2128" : "#f8f9fb");
    }
  }
});

ipcMain.on("widget-cmd", (_event, { cmd, arg }) => {
  sendCommandToPanel(cmd, arg);
});

ipcMain.on("window-hide", () => {
  widgetWindow?.hide();
});

ipcMain.on("toggle-panel", () => {
  togglePanelWindow();
});

ipcMain.on("show-panel", () => {
  showPanelWindow();
});

ipcMain.on("window-set-height", (_event, newHeight) => {
  if (widgetWindow && typeof newHeight === "number" && Number.isFinite(newHeight)) {
    const [w] = widgetWindow.getSize();
    widgetWindow.setSize(w, Math.round(newHeight));
  }
});

ipcMain.on("show-exit-prompt", () => {
  showExitPrompt();
});

ipcMain.on("app-quit", () => {
  quitApplication();
});

ipcMain.on("get-app-version", (event) => {
  event.returnValue = app.getVersion();
});

app.on("before-quit", () => {
  isQuitting = true;
});

app.on("will-quit", () => {
  globalShortcut.unregisterAll();
  bridgeServer?.stop();
});

app.on("window-all-closed", (e) => {
  if (!isQuitting) {
    e.preventDefault(); // Keep running in background / tray
  }
});
