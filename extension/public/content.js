// Floating mini softphone injected into every page. It is a thin UI: it sends
// commands to the offscreen SIP endpoint (via the service worker) and renders
// the state it receives back. Styles are isolated in a shadow root and follow
// the OS light/dark preference to stay in step with the panel theme.

(() => {
  if (window.top !== window) return; // top frame only
  if (document.getElementById("santralc-miniwidget-host")) return;

  const host = document.createElement("div");
  host.id = "santralc-miniwidget-host";
  host.style.cssText = "position:fixed;right:20px;bottom:20px;z-index:2147483647;";
  document.documentElement.appendChild(host);
  const root = host.attachShadow({ mode: "open" });

  const STYLE = `
    :host{ --bg:rgba(255,255,255,0.92); --fg:#1b1e24; --muted:#6b7280; --border:rgba(0,0,0,0.10);
           --input:#f1f3f5; --accent:#2f59c4; --success:#22a06b; --danger:#e5484d; --shadow:rgba(0,0,0,0.18); }
    @media (prefers-color-scheme: dark){
      :host{ --bg:rgba(31,35,41,0.92); --fg:#f3f4f6; --muted:#9aa1ad; --border:rgba(255,255,255,0.14);
             --input:rgba(255,255,255,0.08); --accent:#5b8cff; --success:#2ecc71; --danger:#ff5e57; --shadow:rgba(0,0,0,0.5); }
    }
    *{box-sizing:border-box;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
    .card{width:360px;border:1px solid var(--border);background:var(--bg);color:var(--fg);
          border-radius:16px;box-shadow:0 12px 40px var(--shadow);backdrop-filter:blur(10px);overflow:hidden}
    .head{display:flex;align-items:center;gap:8px;padding:8px 10px;cursor:move;user-select:none}
    .dot{width:8px;height:8px;border-radius:50%;background:var(--success)}
    .title{font-size:12px;font-weight:600}
    .muted{color:var(--muted);font-size:12px}
    .body{padding:8px 10px 12px}
    .pill{display:flex;align-items:center;gap:6px;background:var(--input);border-radius:20px;height:40px;padding:0 6px}
    .sel,.inp{background:transparent;border:0;color:var(--fg);outline:none;font-size:14px}
    .inp{flex:1;min-width:0}
    .row{display:flex;align-items:center;gap:8px}
    .grow{flex:1}
    button{border:0;cursor:pointer;font-family:inherit}
    .circle{width:38px;height:38px;border-radius:50%;display:flex;align-items:center;justify-content:center;
            background:var(--input);color:var(--fg);font-size:16px}
    .call{background:var(--success);color:#fff}
    .hang{background:var(--danger);color:#fff}
    .pri{background:var(--accent);color:#fff;border-radius:10px;height:32px;padding:0 12px;font-size:12px;font-weight:600}
    .ghost{background:transparent;color:var(--muted);border-radius:8px;height:32px;padding:0 8px;font-size:12px}
    .peer{font-size:15px;font-weight:700}
    .keys{display:grid;grid-template-columns:repeat(3,1fr);gap:6px;margin-top:8px}
    .key{height:34px;border-radius:8px;background:var(--input);color:var(--fg);font-size:15px}
    .xfer{margin-top:8px;border-top:1px solid var(--border);padding-top:8px;display:grid;gap:6px}
    .xrow{display:flex;gap:6px}
    .num{flex:1;background:var(--input);border:1px solid var(--border);border-radius:8px;height:32px;color:var(--fg);padding:0 8px;font-size:12px;outline:none}
    .hide{display:none}
  `;

  let state = { status: "idle" };
  let showKeys = false;
  let showXfer = false;
  let isPanel = false;

  // Bridge to the santral-c panel running in this same tab: it pushes its SIP
  // credentials and control commands, and receives live state, so the panel
  // and this extension share one registration/session.
  function postToPage(m) {
    try {
      window.postMessage({ santralc: "ext", ...m }, "*");
    } catch {
      /* ignore */
    }
  }
  window.addEventListener("message", (e) => {
    const d = e.data;
    if (e.source !== window || !d || d.santralc !== "panel") return;
    if (d.type === "hello") {
      isPanel = true;
      host.style.display = "none"; // the panel shows its own softphone UI
      chrome.runtime.sendMessage({ to: "sw", type: "getState" }, (res) => {
        postToPage({ type: "present", state: chrome.runtime.lastError ? state : res && res.state });
      });
    } else if (d.type === "config" && d.config) {
      chrome.runtime.sendMessage({ to: "sw", type: "config", config: d.config }).catch(() => {});
    } else if (d.type === "cmd") {
      chrome.runtime.sendMessage({ to: "offscreen", cmd: d.cmd, arg: d.arg }).catch(() => {});
    }
  });

  function send(cmd, arg) {
    chrome.runtime.sendMessage({ to: "offscreen", cmd, arg }).catch(() => {});
  }

  function esc(s) {
    return String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
  }

  function render() {
    if (isPanel) {
      host.style.display = "none";
      return;
    }
    const st = state.status;
    const active = st === "in-call" || st === "held";
    const outgoing = st === "calling" || st === "ringing";
    let inner = "";

    const header = `<div class="head" id="drag">
        <span class="dot"></span>
        <span class="title">SantralC</span>
        <span class="muted" id="hstatus"></span>
        <span class="grow"></span>
        <button class="ghost" id="collapse" title="Gizle">—</button>
      </div>`;

    if (st === "unconfigured") {
      inner = `<div class="body"><div class="muted">Ayar gerekli. Eklenti simgesine tıklayıp SIP bilgilerini girin.</div></div>`;
    } else if (st === "error") {
      inner = `<div class="body"><div class="muted">Bağlantı hatası: ${esc(state.error)}</div></div>`;
    } else if (st === "incoming") {
      inner = `<div class="body">
        <div class="peer">${esc(state.peer)}</div><div class="muted">Gelen çağrı</div>
        <div class="row" style="margin-top:8px">
          <button class="circle call" id="answer" title="Cevapla">✆</button>
          <button class="circle hang" id="hangup" title="Reddet">⤫</button>
        </div></div>`;
    } else if (outgoing || active) {
      inner = `<div class="body">
        <div class="peer">${esc(state.peer)}</div>
        <div class="muted">${outgoing ? "Aranıyor..." : state.held ? "Beklemede" : "Görüşme"}</div>
        <div class="row" style="margin-top:8px">
          ${active ? `<button class="circle" id="mute" title="Sustur">${state.muted ? "🔇" : "🎙"}</button>
          <button class="circle" id="hold" title="Beklet">${state.held ? "▶" : "⏸"}</button>
          <button class="circle" id="keys" title="Tuşlar">⌨</button>
          <button class="circle" id="xfer" title="Aktar">⇄</button>` : ""}
          <span class="grow"></span>
          <button class="circle hang" id="hangup" title="Kapat">⤫</button>
        </div>
        ${active && showKeys ? `<div class="keys">${["1","2","3","4","5","6","7","8","9","*","0","#"].map((k)=>`<button class="key" data-k="${k}">${k}</button>`).join("")}</div>` : ""}
        ${active && showXfer ? `<div class="xfer">
          <div class="xrow"><input class="num" id="xnum" placeholder="Dahili / numara"><button class="pri" id="xdo">Aktar</button></div>
        </div>` : ""}
      </div>`;
    } else {
      // idle / registered
      inner = `<div class="body">
        <div class="pill">
          <select class="sel" id="prefix"><option value="+90">+90</option><option value="">Dahili</option></select>
          <input class="inp" id="dial" placeholder="Numara..." inputmode="tel">
          <button class="circle call" id="dialbtn" title="Ara">✆</button>
        </div>
      </div>`;
    }

    root.innerHTML = `<style>${STYLE}</style><div class="card">${header}${inner}</div>`;
    const hs = root.getElementById("hstatus");
    if (hs) hs.textContent = st === "registered" || st === "idle" ? "Hazır" : "";

    bind();
  }

  function bind() {
    const q = (id) => root.getElementById(id);
    q("collapse")?.addEventListener("click", () => host.classList.toggle("hide") || (host.style.display = host.style.display === "none" ? "" : "none"));

    q("dialbtn")?.addEventListener("click", () => {
      const pre = q("prefix").value;
      const n = q("dial").value.trim();
      if (n) send("call", pre + n);
    });
    q("dial")?.addEventListener("keydown", (e) => { if (e.key === "Enter") q("dialbtn").click(); });

    q("answer")?.addEventListener("click", () => send("answer"));
    q("hangup")?.addEventListener("click", () => send("hangup"));
    q("mute")?.addEventListener("click", () => send("mute"));
    q("hold")?.addEventListener("click", () => send("hold"));
    q("keys")?.addEventListener("click", () => { showKeys = !showKeys; render(); });
    q("xfer")?.addEventListener("click", () => { showXfer = !showXfer; render(); });
    q("xdo")?.addEventListener("click", () => { const v = q("xnum").value.trim(); if (v) send("transfer", v); });
    root.querySelectorAll(".key").forEach((b) => b.addEventListener("click", () => send("dtmf", b.getAttribute("data-k"))));

    const drag = q("drag");
    if (drag) {
      drag.addEventListener("mousedown", (e) => {
        const sx = e.clientX, sy = e.clientY;
        const rect = host.getBoundingClientRect();
        const move = (ev) => {
          host.style.right = "auto";
          host.style.bottom = "auto";
          host.style.left = rect.left + (ev.clientX - sx) + "px";
          host.style.top = rect.top + (ev.clientY - sy) + "px";
        };
        const up = () => { document.removeEventListener("mousemove", move); document.removeEventListener("mouseup", up); };
        document.addEventListener("mousemove", move);
        document.addEventListener("mouseup", up);
      });
    }
  }

  chrome.runtime.onMessage.addListener((msg) => {
    if (msg && msg.to === "content" && msg.type === "state") {
      state = msg.state || { status: "idle" };
      if (isPanel) postToPage({ type: "state", state });
      render();
    }
  });

  chrome.runtime.sendMessage({ to: "sw", type: "getState" }, (res) => {
    if (chrome.runtime.lastError) return;
    if (res && res.state) state = res.state;
    render();
  });

  render();
})();
