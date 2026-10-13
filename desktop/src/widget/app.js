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

let state = { status: "disconnected" };
let showKeys = false;
let showXfer = false;
let timer = null;

const LABELS = {
  disconnected: "Panel kapalı",
  disabled: "Mesai kapalı",
  connecting: "Bağlanıyor...",
  registered: "Hazır",
  calling: "Aranıyor...",
  ringing: "Çalıyor...",
  incoming: "Gelen çağrı",
  "in-call": "Görüşmede",
  held: "Beklemede",
  error: "Hata",
  unconfigured: "Ayarlanmadı",
};

function clock(sec) {
  const s = Math.max(0, Math.floor(sec || 0));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${m}:${r < 10 ? "0" : ""}${r}`;
}

function normalizeDial(raw) {
  const t = String(raw ?? "").trim();
  if (/[*#]/.test(t)) return t.replace(/[^\d*#]/g, "");
  const d = t.replace(/\D/g, "");
  if (!d) return "";
  if (d.length <= 5) return d;
  if (d.startsWith("90") && d.length === 12) return "0" + d.slice(2);
  if (d.startsWith("0") && d.length === 11) return d;
  if (d.length === 10) return "0" + d;
  return d;
}

function render() {
  const dotEl = document.getElementById("dot");
  const stEl = document.getElementById("st");
  const bodyEl = document.getElementById("body");

  const st = state.status;
  dotEl.className = "dot";
  if (st === "registered" || st === "in-call") dotEl.classList.add("on");
  else if (st === "calling") dotEl.classList.add("calling");
  else if (st === "incoming" || st === "ringing") dotEl.classList.add("ringing");

  let statusText = LABELS[st] || st;
  if (st === "registered" && state.extension) {
    statusText = `Dah: ${state.extension}`;
  }
  stEl.textContent = statusText;

  let html = "";
  const isLive = ["calling", "ringing", "incoming", "in-call", "held"].includes(st);

  if (st === "incoming") {
    html = `
      <div class="peer" id="peer">${state.peer || "Bilinmeyen numara"}</div>
      <div class="sub">Gelen çağrı</div>
      <div class="ctrls">
        <button class="btn call" id="answer" title="Cevapla">${svg(I.call)}</button>
        <button class="btn hang" id="hangup" title="Reddet">${svg(I.hang)}</button>
      </div>
    `;
  } else if (isLive) {
    const isTalk = st === "in-call" || st === "held";
    html = `
      <div class="peer" id="peer">${state.peer || "Çağrı"}</div>
      <div class="dur" id="dur">${clock(state.answeredAt ? (Date.now() - state.answeredAt) / 1000 : 0)}</div>
      <div class="ctrls">
        ${isTalk ? `
          <button class="btn ${state.muted ? "on" : ""}" id="mute" title="${state.muted ? "Sesi aç" : "Sustur"}">${svg(state.muted ? I.micoff : I.mic)}</button>
          <button class="btn ${state.held ? "on" : ""}" id="hold" title="${state.held ? "Görüşmeye dön" : "Beklet"}">${svg(state.held ? I.play : I.pause)}</button>
          <button class="btn ${showKeys ? "on" : ""}" id="keys" title="Tuşlar">${svg(I.keys)}</button>
          <button class="btn ${showXfer ? "on" : ""}" id="xfer" title="Aktar">${svg(I.xfer)}</button>
        ` : ""}
        <button class="btn hang" id="hangup" title="Kapat">${svg(I.hang)}</button>
      </div>
      ${isTalk && showKeys ? `
        <div class="keys">
          ${["1","2","3","4","5","6","7","8","9","*","0","#"].map((k) => `<button class="key" data-k="${k}">${k}</button>`).join("")}
        </div>
      ` : ""}
      ${isTalk && showXfer ? `
        <div class="xfer">
          <input class="num" id="xnum" placeholder="Dahili / numara">
          <button class="go" id="xdo">Aktar</button>
        </div>
      ` : ""}
    `;
  } else if (st === "disconnected") {
    html = `
      <div class="peer" style="cursor:default">Panel açık değil</div>
      <div class="sub">Tarayıcıda Santral-C web panelini açtığınızda telefon hazır olacaktır.</div>
    `;
  } else if (st === "disabled") {
    html = `
      <div class="peer" style="cursor:default">Mesai başlatılmadı</div>
      <div class="sub">Çağrı almak ve arama yapmak için panelden mesaiyi başlatın.</div>
    `;
  } else if (st === "connecting") {
    html = `
      <div class="peer" style="cursor:default">Bağlanıyor...</div>
      <div class="sub">Santral hattına bağlanıyor. Hazır olunca buradan arayabilirsiniz.</div>
    `;
  } else if (st !== "registered") {
    html = `
      <div class="peer" style="cursor:default">Telefon hazır değil</div>
      <div class="sub">Paneldeki softphone durumunu kontrol edin.</div>
    `;
  } else if (state.canCall === false) {
    html = `
      <div class="peer" style="cursor:default">Hazır</div>
      <div class="sub">Giden arama yetkiniz yok. Gelen çağrıları karşılayabilirsiniz.</div>
    `;
  } else {
    html = `
      <div class="pill">
        <select class="sel" id="prefix">
          <option value="+90">+90</option>
          <option value="">Dahili</option>
        </select>
        <input class="inp" id="dial" placeholder="Numara" inputmode="tel">
        <button class="btn call" id="dialbtn" title="Ara">${svg(I.call)}</button>
      </div>
    `;
  }

  bodyEl.innerHTML = html;
  bindEvents();
  adjustHeight();
}

function bindEvents() {
  const dial = () => {
    const input = document.getElementById("dial");
    if (!input) return;
    const raw = input.value.trim();
    if (!raw) return;
    const prefix = document.getElementById("prefix")?.value ?? "+90";
    const n = prefix === "" ? normalizeDial(raw) : normalizeDial(raw.length <= 5 ? raw : raw.replace(/^\+?90/, "").replace(/^0/, ""));
    if (n) {
      window.santralDesktop?.sendCommand("call", n);
    }
  };

  document.getElementById("dialbtn")?.addEventListener("click", dial);
  document.getElementById("dial")?.addEventListener("keydown", (e) => {
    if (e.key === "Enter") dial();
  });

  document.getElementById("answer")?.addEventListener("click", () => {
    window.santralDesktop?.sendCommand("answer");
  });

  document.getElementById("hangup")?.addEventListener("click", () => {
    window.santralDesktop?.sendCommand("hangup");
  });

  document.getElementById("mute")?.addEventListener("click", () => {
    window.santralDesktop?.sendCommand("mute");
  });

  document.getElementById("hold")?.addEventListener("click", () => {
    window.santralDesktop?.sendCommand("hold");
  });

  document.getElementById("keys")?.addEventListener("click", () => {
    showKeys = !showKeys;
    if (showKeys) showXfer = false;
    render();
  });

  document.getElementById("xfer")?.addEventListener("click", () => {
    showXfer = !showXfer;
    if (showXfer) showKeys = false;
    render();
  });

  document.getElementById("xdo")?.addEventListener("click", () => {
    const v = normalizeDial(document.getElementById("xnum")?.value);
    if (v) window.santralDesktop?.sendCommand("transfer", v);
  });

  document.querySelectorAll(".key").forEach((b) => {
    b.addEventListener("click", () => {
      const k = b.getAttribute("data-k");
      if (k) window.santralDesktop?.sendCommand("dtmf", k);
    });
  });

  document.getElementById("peer")?.addEventListener("click", () => {
    const p = state.peer;
    if (p && navigator.clipboard) {
      navigator.clipboard.writeText(p);
      const el = document.getElementById("peer");
      if (el) el.innerHTML = `${p} <span class="ok">Kopyalandı</span>`;
    }
  });

  if (timer) {
    clearInterval(timer);
    timer = null;
  }
  if (state.answeredAt && (state.status === "in-call" || state.status === "held")) {
    timer = setInterval(() => {
      const durEl = document.getElementById("dur");
      if (!durEl) {
        clearInterval(timer);
        timer = null;
        return;
      }
      durEl.textContent = clock((Date.now() - state.answeredAt) / 1000);
    }, 1000);
  }
}

function adjustHeight() {
  requestAnimationFrame(() => {
    const card = document.getElementById("card");
    if (card && window.santralDesktop?.setHeight) {
      const h = card.offsetHeight + 10;
      window.santralDesktop.setHeight(h);
    }
  });
}

document.getElementById("hidebtn")?.addEventListener("click", () => {
  window.santralDesktop?.hide();
});

document.getElementById("panelbtn")?.addEventListener("click", () => {
  window.santralDesktop?.togglePanel?.();
});

if (window.santralDesktop) {
  window.santralDesktop.onState((newState) => {
    const prev = state.status;
    state = newState || { status: "disconnected" };
    if (prev !== state.status) {
      showKeys = false;
      showXfer = false;
    }
    render();
  });
}

render();
