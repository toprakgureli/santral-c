// The panel's address decides which tab may drive the widgets; every other
// page is treated as an ordinary website.
const DEFAULT_PANEL = "https://cm.toprakgureli.com";
const input = document.getElementById("origin");
const note = document.getElementById("note");

chrome.storage.local.get("panelOrigin").then(({ panelOrigin }) => {
  input.value = typeof panelOrigin === "string" && panelOrigin ? panelOrigin : DEFAULT_PANEL;
});

function say(text, tone) {
  note.textContent = text;
  note.className = "note" + (tone ? " " + tone : "");
}

document.getElementById("save").addEventListener("click", async () => {
  let origin;
  try {
    const url = new URL(input.value.trim());
    const local = url.hostname === "localhost" || url.hostname === "127.0.0.1";
    if (url.protocol !== "https:" && !(local && url.protocol === "http:")) throw new Error("https");
    origin = url.origin;
  } catch {
    say("Geçerli bir https adresi yazın (örneğin https://cm.ornek.com).", "bad");
    return;
  }
  await chrome.storage.local.set({ panelOrigin: origin });
  input.value = origin;
  say("Kaydedildi. Panel sekmesini ve diğer sekmeleri yenileyin.", "good");
});
