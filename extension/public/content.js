// Injected into every page. On the santral-c panel tab it is a silent bridge
// (relays the panel's live call state to the worker, and worker commands back
// to the panel). On every other tab it draws the floating mini softphone that
// remote-controls the panel's session. Styles are isolated in a shadow root and
// follow the OS light/dark preference.

(() => {
  if (window.top !== window) return;
  if (document.getElementById("santralc-miniwidget-host")) return;

  const host = document.createElement("div");
  host.id = "santralc-miniwidget-host";
  host.style.cssText = "position:fixed;right:20px;bottom:20px;z-index:2147483647;";
  document.documentElement.appendChild(host);
  const root = host.attachShadow({ mode: "open" });

  const I = {
    call: '<path d="M6.62 10.79c1.44 2.83 3.76 5.15 6.59 6.59l2.2-2.2a1 1 0 0 1 1.02-.24c1.12.37 2.33.57 3.57.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1A17 17 0 0 1 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.24.2 2.45.57 3.57a1 1 0 0 1-.24 1.02l-2.2 2.2z"/>',
    hang: '<path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .4-.23.74-.56.9-.98.49-1.87 1.12-2.67 1.85-.18.18-.43.29-.68.29-.3 0-.55-.12-.73-.3L.29 13.08C.11 12.9 0 12.65 0 12.38c0-.28.11-.53.29-.71C3.34 8.77 7.46 7 12 7s8.66 1.77 11.71 4.67c.18.18.29.43.29.71 0 .27-.11.52-.29.7l-2.48 2.48c-.18.18-.43.3-.73.3-.25 0-.5-.11-.68-.29-.79-.73-1.68-1.36-2.66-1.85-.33-.16-.56-.5-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"/>',
    mic: '<path d="M12 2a3 3 0 0 1 3 3v6a3 3 0 0 1-6 0V5a3 3 0 0 1 3-3zM19 11a7 7 0 0 1-6 6.92V21h-2v-3.08A7 7 0 0 1 5 11h2a5 5 0 0 0 10 0h2z"/>',
    micoff: '<path d="M3 4.27 4.27 3 21 19.73 19.73 21l-3.7-3.7A7 7 0 0 1 13 17.92V21h-2v-3.08A7 7 0 0 1 5 11h2a5 5 0 0 0 7.31 4.43l-1.5-1.5A3 3 0 0 1 9 11V9.27L3 4.27zM15 9.8 9.2 4H9a3 3 0 0 1 6 0v4c0 .28-.03.55-.1.8z"/>',
    pause: '<path d="M8 5h3v14H8zM13 5h3v14h-3z"/>',
    play: '<path d="M8 5v14l11-7z"/>',
    keys: '<path d="M7 4a2 2 0 1 1 0 4 2 2 0 0 1 0-4zm5 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4zm5 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM7 10a2 2 0 1 1 0 4 2 2 0 0 1 0-4zm5 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4zm5 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM7 16a2 2 0 1 1 0 4 2 2 0 0 1 0-4zm5 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4zm5 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4z"/>',
    xfer: '<path d="M14 4l5 5-5 5v-3H8a3 3 0 0 0-3 3v4H3v-4a5 5 0 0 1 5-5h6V4z"/>',
  };
  const svg = (p) => `<svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">${p}</svg>`;

  const STYLE = `
    :host{ --bg:rgba(255,255,255,0.96); --fg:#1b1e24; --muted:#6b7280; --border:rgba(0,0,0,0.10);
           --input:#eef0f3; --accent:#2f59c4; --success:#22a06b; --danger:#e5484d; --shadow:rgba(0,0,0,0.20); }
    @media (prefers-color-scheme: dark){
      :host{ --bg:rgba(30,34,40,0.97); --fg:#f3f4f6; --muted:#9aa1ad; --border:rgba(255,255,255,0.13);
             --input:rgba(255,255,255,0.09); --accent:#5b8cff; --success:#2ecc71; --danger:#ff5e57; --shadow:rgba(0,0,0,0.55); }
    }
    *{box-sizing:border-box;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
    .card{width:288px;border:1px solid var(--border);background:var(--bg);color:var(--fg);
          border-radius:18px;box-shadow:0 14px 44px var(--shadow);backdrop-filter:blur(12px);overflow:hidden}
    .head{display:flex;align-items:center;gap:8px;padding:11px 14px 6px;cursor:move;user-select:none}
    .dot{width:8px;height:8px;border-radius:50%;background:var(--muted)}
    .dot.on{background:var(--success)}
    .title{font-size:12.5px;font-weight:700;letter-spacing:.2px}
    .st{margin-left:auto;font-size:11px;color:var(--muted)}
    .body{padding:6px 14px 14px}
    .peer{font-size:16px;font-weight:700;line-height:1.2;word-break:break-all}
    .sub{font-size:12px;color:var(--muted);margin-top:2px}
    button{border:0;cursor:pointer;font-family:inherit;color:inherit}
    .pill{display:flex;align-items:center;gap:8px;background:var(--input);border-radius:22px;height:44px;padding:0 6px 0 12px}
    .sel{background:transparent;border:0;color:var(--fg);outline:none;font-size:13px;font-weight:600}
    .inp{flex:1;min-width:0;background:transparent;border:0;color:var(--fg);outline:none;font-size:15px}
    .btn{width:44px;height:44px;border-radius:50%;display:flex;align-items:center;justify-content:center;background:var(--input);color:var(--fg);flex:0 0 auto;transition:transform .12s,background .15s}
    .btn:active{transform:scale(.92)}
    .btn.on{background:var(--accent);color:#fff}
    .btn.call{background:var(--success);color:#fff}
    .btn.hang{background:var(--danger);color:#fff}
    .ctrls{display:flex;align-items:center;justify-content:center;gap:10px;margin-top:12px}
    .keys{display:grid;grid-template-columns:repeat(3,1fr);gap:8px;margin-top:12px}
    .key{height:42px;border-radius:12px;background:var(--input);color:var(--fg);font-size:17px;font-weight:600}
    .key:active{transform:scale(.95)}
    .xfer{display:flex;gap:8px;margin-top:12px}
    .num{flex:1;background:var(--input);border:1px solid var(--border);border-radius:12px;height:40px;color:var(--fg);padding:0 12px;font-size:13px;outline:none}
    .go{background:var(--accent);color:#fff;border-radius:12px;height:40px;padding:0 16px;font-size:13px;font-weight:600}
  `;

  let state = { status: "idle" };
  let showKeys = false;
  let showXfer = false;
  let isPanel = false;

  function postToPage(m) { try { window.postMessage({ santralc: "ext", ...m }, "*"); } catch { /* ignore */ } }

  window.addEventListener("message", (e) => {
    const d = e.data;
    if (e.source !== window || !d || d.santralc !== "panel") return;
    if (d.type === "hello") { isPanel = true; host.style.display = "none"; }
    else if (d.type === "state") chrome.runtime.sendMessage({ to: "sw", type: "state", state: d.state }).catch(() => {});
  });

  function send(cmd, arg) { chrome.runtime.sendMessage({ to: "sw", type: "cmd", cmd, arg }).catch(() => {}); }
  function esc(s) { return String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]); }

  function render() {
    if (isPanel) { host.style.display = "none"; return; }
    const st = state.status;
    const active = st === "in-call" || st === "held";
    const outgoing = st === "calling" || st === "ringing";
    const stLabel = st === "registered" || st === "idle" ? "Hazır" : st === "unconfigured" ? "Panel kapalı" : "";
    const on = st !== "unconfigured" && st !== "error";

    const head = `<div class="head" id="drag"><span class="dot ${on ? "on" : ""}"></span><span class="title">SantralC</span><span class="st">${stLabel}</span></div>`;
    let body;

    if (st === "incoming") {
      body = `<div class="peer">${esc(state.peer) || "Bilinmeyen"}</div><div class="sub">Gelen çağrı</div>
        <div class="ctrls"><button class="btn call" id="answer" title="Cevapla">${svg(I.call)}</button>
        <button class="btn hang" id="hangup" title="Reddet">${svg(I.hang)}</button></div>`;
    } else if (outgoing || active) {
      body = `<div class="peer">${esc(state.peer)}</div><div class="sub">${outgoing ? "Aranıyor..." : state.held ? "Beklemede" : "Görüşme"}</div>
        <div class="ctrls">
          ${active ? `<button class="btn ${state.muted ? "on" : ""}" id="mute" title="Sustur">${svg(state.muted ? I.micoff : I.mic)}</button>
          <button class="btn ${state.held ? "on" : ""}" id="hold" title="Beklet">${svg(state.held ? I.play : I.pause)}</button>
          <button class="btn ${showKeys ? "on" : ""}" id="keys" title="Tuş takımı">${svg(I.keys)}</button>
          <button class="btn ${showXfer ? "on" : ""}" id="xfer" title="Aktar">${svg(I.xfer)}</button>` : ""}
          <button class="btn hang" id="hangup" title="Kapat">${svg(I.hang)}</button>
        </div>
        ${active && showKeys ? `<div class="keys">${["1","2","3","4","5","6","7","8","9","*","0","#"].map((k) => `<button class="key" data-k="${k}">${k}</button>`).join("")}</div>` : ""}
        ${active && showXfer ? `<div class="xfer"><input class="num" id="xnum" placeholder="Dahili / kuyruk / numara"><button class="go" id="xdo">Aktar</button></div>` : ""}`;
    } else {
      body = `<div class="pill">
        <select class="sel" id="prefix"><option value="+90">+90</option><option value="">Dahili</option></select>
        <input class="inp" id="dial" placeholder="Numara" inputmode="tel">
        <button class="btn call" id="dialbtn" title="Ara">${svg(I.call)}</button></div>`;
    }

    root.innerHTML = `<style>${STYLE}</style><div class="card">${head}<div class="body">${body}</div></div>`;
    bind();
  }

  function bind() {
    const q = (id) => root.getElementById(id);
    q("dialbtn")?.addEventListener("click", () => { const n = q("dial").value.trim(); if (n) send("call", q("prefix").value + n); });
    q("dial")?.addEventListener("keydown", (e) => { if (e.key === "Enter") q("dialbtn").click(); });
    q("answer")?.addEventListener("click", () => send("answer"));
    q("hangup")?.addEventListener("click", () => send("hangup"));
    q("mute")?.addEventListener("click", () => send("mute"));
    q("hold")?.addEventListener("click", () => send("hold"));
    q("keys")?.addEventListener("click", () => { showKeys = !showKeys; if (showKeys) showXfer = false; render(); });
    q("xfer")?.addEventListener("click", () => { showXfer = !showXfer; if (showXfer) showKeys = false; render(); });
    q("xdo")?.addEventListener("click", () => { const v = q("xnum").value.trim(); if (v) send("transfer", v); });
    root.querySelectorAll(".key").forEach((b) => b.addEventListener("click", () => send("dtmf", b.getAttribute("data-k"))));

    const drag = q("drag");
    if (drag) drag.addEventListener("mousedown", (e) => {
      const sx = e.clientX, sy = e.clientY; const rect = host.getBoundingClientRect();
      const move = (ev) => { host.style.right = "auto"; host.style.bottom = "auto"; host.style.left = rect.left + (ev.clientX - sx) + "px"; host.style.top = rect.top + (ev.clientY - sy) + "px"; };
      const up = () => { document.removeEventListener("mousemove", move); document.removeEventListener("mouseup", up); };
      document.addEventListener("mousemove", move); document.addEventListener("mouseup", up);
    });
  }

  chrome.runtime.onMessage.addListener((msg) => {
    if (!msg || msg.to !== "content") return;
    if (msg.type === "state") {
      const prev = state.status;
      state = msg.state || { status: "idle" };
      if (prev !== state.status) { showKeys = false; showXfer = false; }
      if (!isPanel) render();
    } else if (msg.type === "panelcmd" && isPanel) {
      postToPage({ type: "cmd", cmd: msg.cmd, arg: msg.arg });
    }
  });

  chrome.runtime.sendMessage({ to: "sw", type: "getState" }, (res) => {
    if (!chrome.runtime.lastError && res && res.state) state = res.state;
    render();
  });

  render();
})();
