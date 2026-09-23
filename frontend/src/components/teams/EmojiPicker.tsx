// EmojiPicker: a small grid of emojis by category, with the ones used most
// recently at the top. Picking one calls back with the character.

import { useEffect, useMemo, useState } from "react";
import { cn } from "@/lib/utils";

const GROUPS: { key: string; label: string; items: string }[] = [
  { key: "faces", label: "Yüzler", items: "😀 😃 😄 😁 😆 😅 😂 🤣 😊 😇 🙂 🙃 😉 😌 😍 🥰 😘 😗 😙 😚 😋 😛 😝 😜 🤪 🤨 🧐 🤓 😎 🥸 🤩 🥳 😏 😒 😞 😔 😟 😕 🙁 ☹️ 😣 😖 😫 😩 🥺 😢 😭 😤 😠 😡 🤬 🤯 😳 🥵 🥶 😱 😨 😰 😥 😓 🤗 🤔 🤭 🤫 🤥 😶 😐 😑 😬 🙄 😯 😦 😧 😮 😲 🥱 😴 🤤 😪 😵 🤐 🥴 🤢 🤮 🤧 😷 🤒 🤕 🤑 🤠 😈 👿 💀 ☠️ 💩 🤡 👻 👽 🤖" },
  { key: "hands", label: "Eller", items: "👍 👎 👌 🤌 🤏 ✌️ 🤞 🤟 🤘 🤙 👈 👉 👆 👇 ☝️ 👋 🤚 🖐️ ✋ 🖖 👏 🙌 🤲 🤝 🙏 ✍️ 💪 🦾 🫶 🫡 🫰 👀 👁️ 🧠" },
  { key: "hearts", label: "Kalpler", items: "❤️ 🧡 💛 💚 💙 💜 🖤 🤍 🤎 💔 ❤️‍🔥 💕 💞 💓 💗 💖 💘 💝 💟 ♥️ 🔥 ✨ ⭐ 🌟 💫 💥 💯 ❗ ❓ ✅ ❌ ⚠️ 🚀 🎉 🎊 🎈 🏆 🥇 🎯 💡 🔔 🔕" },
  { key: "objects", label: "Nesneler", items: "📞 ☎️ 📱 💻 🖥️ ⌨️ 🖱️ 📧 📨 📩 📝 📋 📁 📂 🗂️ 📊 📈 📉 🗓️ 📅 ⏰ ⏳ ⌛ 🔒 🔓 🔑 🛠️ 🔧 🔨 ⚙️ 🧰 🧲 💰 💳 🧾 📦 🚚 ✈️ 🚗 🏠 🏢 ☕ 🍵 🍕 🍔 🍩 🍪 🎂 🍺 🥤" },
  { key: "symbols", label: "Semboller", items: "✔️ ☑️ 🔴 🟠 🟡 🟢 🔵 🟣 ⚫ ⚪ 🔺 🔻 ▶️ ⏸️ ⏹️ 🔁 🔀 ➕ ➖ ➗ ✖️ 🆗 🆕 🆙 🆓 🔝 🔙 🔜 ℹ️ 🅿️ 🚫 ⛔ 🔞 💤 🏳️ 🏁 🇹🇷" },
];

const RECENT_KEY = "teams.emoji.recent";

function loadRecent(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(RECENT_KEY) ?? "[]");
    return Array.isArray(v) ? v.slice(0, 24) : [];
  } catch {
    return [];
  }
}

export function rememberEmoji(e: string) {
  try {
    const next = [e, ...loadRecent().filter((x) => x !== e)].slice(0, 24);
    localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // storage unavailable
  }
}

export default function EmojiPicker({ onPick, onClose, className }: { onPick: (emoji: string) => void; onClose: () => void; className?: string }) {
  const [tab, setTab] = useState(GROUPS[0].key);
  const [recent, setRecent] = useState<string[]>(loadRecent);
  const [q, setQ] = useState("");

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const all = useMemo(() => GROUPS.flatMap((g) => g.items.split(" ")), []);
  const shown = q.trim() ? all : (GROUPS.find((g) => g.key === tab)?.items.split(" ") ?? []);

  const pick = (e: string) => {
    rememberEmoji(e);
    setRecent(loadRecent());
    onPick(e);
  };

  return (
    <div className={cn("w-72 overflow-hidden rounded-xl border border-border bg-popover shadow-lg", className)} onMouseDown={(e) => e.preventDefault()}>
      <div className="flex gap-0.5 border-b border-border/60 p-1">
        {GROUPS.map((g) => (
          <button key={g.key} type="button" onClick={() => { setTab(g.key); setQ(""); }} className={cn("flex-1 rounded-lg px-1 py-1 text-[0.65rem] font-medium", tab === g.key && !q ? "bg-accent text-foreground" : "text-muted-foreground hover:bg-accent/60")}>
            {g.label}
          </button>
        ))}
      </div>
      <div className="max-h-56 overflow-y-auto p-1.5">
        {!q && recent.length > 0 && (
          <>
            <p className="px-1 pb-0.5 text-[0.6rem] font-semibold uppercase tracking-wide text-muted-foreground">Son kullanılan</p>
            <div className="mb-1.5 grid grid-cols-8 gap-0.5">
              {recent.map((e) => (
                <button key={`r-${e}`} type="button" onClick={() => pick(e)} className="rounded-lg py-0.5 text-xl leading-none hover:bg-accent">{e}</button>
              ))}
            </div>
          </>
        )}
        <div className="grid grid-cols-8 gap-0.5">
          {shown.map((e) => (
            <button key={e} type="button" onClick={() => pick(e)} className="rounded-lg py-0.5 text-xl leading-none hover:bg-accent">{e}</button>
          ))}
        </div>
      </div>
    </div>
  );
}
