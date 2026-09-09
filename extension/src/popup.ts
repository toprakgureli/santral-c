// Popup: stores the SIP credentials the offscreen endpoint registers with, and
// lets the user grant microphone access to the extension (needed once so the
// offscreen document can capture audio for calls).

const $ = (id: string) => document.getElementById(id) as HTMLInputElement;
const msg = document.getElementById("msg") as HTMLDivElement;

const DEFAULTS = {
  wss: "wss://api.bulutsantralim.com:7443",
  domain: "ersinhacioglu.bulutsantralim.com",
  stun: "stun:194.49.126.36:7443",
};

async function load() {
  const { sipConfig } = await chrome.storage.local.get("sipConfig");
  $("ext").value = sipConfig?.ext ?? "";
  $("password").value = sipConfig?.password ?? "";
  $("wss").value = sipConfig?.wss ?? DEFAULTS.wss;
  $("domain").value = sipConfig?.domain ?? DEFAULTS.domain;
  $("stun").value = sipConfig?.stun ?? DEFAULTS.stun;
}

document.getElementById("save")!.addEventListener("click", async () => {
  const sipConfig = {
    ext: $("ext").value.trim(),
    password: $("password").value,
    wss: $("wss").value.trim(),
    domain: $("domain").value.trim(),
    stun: $("stun").value.trim(),
  };
  if (!sipConfig.ext || !sipConfig.password || !sipConfig.wss || !sipConfig.domain) {
    msg.textContent = "Dahili, parola, WSS ve domain zorunlu.";
    return;
  }
  await chrome.storage.local.set({ sipConfig });
  chrome.runtime.sendMessage({ to: "offscreen", cmd: "reconfigure" }).catch(() => undefined);
  msg.textContent = "Kaydedildi. Bağlanılıyor...";
});

document.getElementById("mic")!.addEventListener("click", async () => {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    stream.getTracks().forEach((t) => t.stop());
    msg.textContent = "Mikrofon izni verildi.";
  } catch {
    msg.textContent = "Mikrofon izni reddedildi.";
  }
});

void load();
