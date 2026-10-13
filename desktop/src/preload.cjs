const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("santralDesktop", {
  onState: (callback) => {
    const handler = (_event, state) => callback(state);
    ipcRenderer.on("call-state", handler);
    return () => ipcRenderer.removeListener("call-state", handler);
  },
  sendCommand: (cmd, arg) => {
    ipcRenderer.send("widget-cmd", { cmd, arg });
  },
  hide: () => {
    ipcRenderer.send("window-hide");
  },
  setHeight: (height) => {
    ipcRenderer.send("window-set-height", height);
  },
  togglePanel: () => {
    ipcRenderer.send("toggle-panel");
  },
  showPanel: () => {
    ipcRenderer.send("show-panel");
  },
  showExitPrompt: () => {
    ipcRenderer.send("show-exit-prompt");
  },
  quit: () => {
    ipcRenderer.send("app-quit");
  },
});
