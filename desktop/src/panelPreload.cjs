const { ipcRenderer } = require("electron");

const THEME_MAP = {
  light: { bg: "#f8f9fb", isDark: false },
  dark: { bg: "#1e2128", isDark: true },
  midnight: { bg: "#151a2b", isDark: true },
  forest: { bg: "#171f1b", isDark: true },
  sand: { bg: "#f6f1e8", isDark: false },
  crimson: { bg: "#1c1114", isDark: true },
  matte: { bg: "#000000", isDark: true },
};

let lastSentTheme = null;

function detectAndSendTheme() {
  try {
    const root = document.documentElement;
    let theme = root ? root.getAttribute("data-theme") : null;
    if (!theme) {
      try {
        theme = window.localStorage ? window.localStorage.getItem("santral.theme") : null;
      } catch {}
    }

    if (theme && THEME_MAP[theme]) {
      if (lastSentTheme !== theme) {
        lastSentTheme = theme;
        ipcRenderer.send("panel-theme-change", {
          theme,
          bg: THEME_MAP[theme].bg,
          isDark: THEME_MAP[theme].isDark,
        });
      }
      return;
    }

    if (root) {
      const isDark = root.classList.contains("dark");
      const fallbackTheme = isDark ? "dark" : "light";
      if (lastSentTheme !== fallbackTheme) {
        lastSentTheme = fallbackTheme;
        ipcRenderer.send("panel-theme-change", {
          theme: fallbackTheme,
          bg: isDark ? "#1e2128" : "#f8f9fb",
          isDark,
        });
      }
    }
  } catch {}
}

function initThemeObserver() {
  detectAndSendTheme();

  if (document.documentElement) {
    const observer = new MutationObserver(() => {
      detectAndSendTheme();
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme", "class"],
    });
  }
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", initThemeObserver);
} else {
  initThemeObserver();
}

window.addEventListener("load", detectAndSendTheme);

window.addEventListener("storage", (e) => {
  if (e.key === "santral.theme") {
    detectAndSendTheme();
  }
});

// Relay panel messages (softphone state & heartbeats) to the Electron main process
window.addEventListener("message", (event) => {
  const data = event.data;
  if (!data || data.santralc !== "panel") return;

  if (data.type === "state" && data.state) {
    ipcRenderer.send("panel-state-update", data.state);
  } else if (data.type === "alive" || data.type === "hello") {
    ipcRenderer.send("panel-heartbeat");
  }
});

// Receive commands from Electron (widget buttons, global hotkeys) and forward to the softphone
ipcRenderer.on("panel-command", (_event, { cmd, arg }) => {
  window.postMessage({ santralc: "ext", type: "cmd", cmd, arg }, window.location.origin);
});
