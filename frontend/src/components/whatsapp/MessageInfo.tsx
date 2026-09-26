// MessageMenu is the right-click menu of a message: quick reactions on top,
// then reply, copy and info. MessageInfo shows where a message got to: when
// it was sent, delivered and read (or why it failed), and for the
// customer's messages, who in the team has seen it and when.

import { useEffect, useRef, useState } from "react";
import { AlertCircle, Check, CheckCheck, Clock3, Copy, CornerUpLeft, Eye, Info, X } from "lucide-react";
import UserAvatar from "@/components/ui/UserAvatar";
import MessageBubble from "@/components/whatsapp/MessageBubble";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAMessage, WAReadInfo } from "@/whatsapp/types";

const QUICK = ["👍", "❤️", "😂", "😮", "🙏", "✅"];

export function MessageMenu({ m, x, y, onReply, onReact, onInfo, onClose }: { m: WAMessage; x: number; y: number; onReply?: () => void; onReact?: (emoji: string) => void; onInfo: () => void; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    setPos({ left: Math.max(8, Math.min(x, window.innerWidth - r.width - 8)), top: Math.max(8, Math.min(y, window.innerHeight - r.height - 8)) });
  }, [x, y]);
  useEffect(() => {
    const close = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) onClose(); };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    const t = window.setTimeout(() => {
      window.addEventListener("mousedown", close);
      window.addEventListener("keydown", esc);
      window.addEventListener("resize", onClose);
      window.addEventListener("wheel", onClose, { passive: true });
    }, 0);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("wheel", onClose);
    };
  }, [onClose]);
  const run = (fn: () => void) => {
    onClose();
    fn();
  };
  return (
    <div ref={box} className="animate-in fade-in zoom-in-95 fixed z-[70] w-56 overflow-hidden rounded-2xl border border-border bg-popover p-1.5 text-popover-foreground shadow-xl duration-100" style={pos} onContextMenu={(e) => e.preventDefault()}>
      {onReact && (
        <div className="mb-1 flex justify-between rounded-xl bg-muted/60 px-1.5 py-1">
          {QUICK.map((e) => <button key={e} type="button" onClick={() => run(() => onReact(e))} className="rounded-full px-1 text-xl leading-none transition-transform hover:scale-125">{e}</button>)}
        </div>
      )}
      {onReply && <Item icon={CornerUpLeft} label="Yanıtla" onClick={() => run(onReply)} />}
      {m.body && <Item icon={Copy} label="Metni kopyala" onClick={() => run(() => void navigator.clipboard?.writeText(m.body))} />}
      <Item icon={Info} label="Bilgi" onClick={() => run(onInfo)} />
    </div>
  );
}

function Item({ icon: Icon, label, onClick }: { icon: typeof Info; label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-sm transition-colors hover:bg-accent">
      <Icon className="size-4 shrink-0 text-muted-foreground" /> {label}
    </button>
  );
}

function when(iso?: string) {
  if (!iso) return null;
  const d = new Date(iso);
  const today = new Date();
  const yesterday = new Date(Date.now() - 86400000);
  const day = d.toDateString() === today.toDateString() ? "Bugün" : d.toDateString() === yesterday.toDateString() ? "Dün" : d.toLocaleDateString("tr-TR", { day: "numeric", month: "long", year: d.getFullYear() === today.getFullYear() ? undefined : "numeric" });
  return `${day} ${d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`;
}

export function MessageInfo({ m: given, onClose }: { m: WAMessage; onClose: () => void }) {
  const [m, setM] = useState(given);
  const [reads, setReads] = useState<WAReadInfo[] | null>(null);

  // The latest word from the server: statuses may have moved on.
  useEffect(() => {
    let live = true;
    if (given.id > 0) {
      waApi.messages(given.conversationId, { around: given.id }).then((page) => {
        const fresh = page.find((x) => x.id === given.id);
        if (live && fresh) setM(fresh);
      }).catch(() => undefined);
    }
    if (given.direction !== "out") waApi.reads(given.conversationId).then((r) => live && setReads(r)).catch(() => live && setReads([]));
    return () => {
      live = false;
    };
  }, [given]);

  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [onClose]);

  const out = m.direction === "out";
  const seen = (reads ?? []).filter((r) => r.messageId >= m.id).sort((a, b) => Date.parse(a.readAt) - Date.parse(b.readAt));

  return (
    <div className="fixed inset-0 z-[75] flex justify-end bg-black/30" onClick={onClose}>
      <aside onClick={(e) => e.stopPropagation()} className="animate-in slide-in-from-right-4 fade-in flex h-full w-full max-w-md flex-col bg-muted shadow-2xl duration-200">
        <header className="flex h-16 shrink-0 items-center gap-3 bg-card px-4">
          <button type="button" onClick={onClose} aria-label="Kapat" className="flex size-9 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-5" /></button>
          <p className="text-[0.95rem] font-semibold">Mesaj bilgisi</p>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="wa-wall py-6">
            <MessageBubble m={m} head />
          </div>
          <div className="space-y-2 pb-6">
            {out ? (
              <section className="bg-card px-5 py-2">
                {m.status === "failed" ? (
                  <Step icon={<AlertCircle className="size-5 text-destructive" />} label="Gönderilemedi" value={m.errorText || "WhatsApp mesajı kabul etmedi."} danger />
                ) : (
                  <>
                    <Step icon={<CheckCheck className="size-5 text-[#53bdeb]" />} label="Okundu" value={when(m.readAt) ?? (m.status === "read" ? "Okundu, saati bildirilmedi" : "Henüz okunmadı")} muted={!m.readAt && m.status !== "read"} />
                    <Step icon={<CheckCheck className="size-5 text-wa-meta" />} label="Teslim edildi" value={when(m.deliveredAt) ?? (m.status === "delivered" || m.status === "read" ? "Teslim edildi, saati bildirilmedi" : "Henüz telefona ulaşmadı")} muted={!m.deliveredAt && m.status !== "delivered" && m.status !== "read"} />
                    <Step icon={m.status === "queued" ? <Clock3 className="size-5 text-wa-meta" /> : <Check className="size-5 text-wa-meta" />} label={m.status === "queued" ? "Sırada" : "Gönderildi"} value={when(m.sentAt) ?? (m.status === "queued" ? "WhatsApp'a gönderilmeyi bekliyor" : "—")} muted={m.status === "queued"} />
                  </>
                )}
              </section>
            ) : (
              <section className="bg-card px-5 py-2">
                <Step icon={<Check className="size-5 text-wa-meta" />} label={m.direction === "note" ? "Yazıldı" : "Geldi"} value={when(m.createdAt) ?? "—"} />
              </section>
            )}
            <section className="bg-card px-5 py-3 text-sm">
              <Row label="Yazan" value={m.direction === "in" ? "Müşteri" : m.sender.kind === "agent" || m.direction === "note" ? m.sender.name ?? "—" : m.sender.kind === "bot" ? `Chatbot · ${m.sender.label}` : `Otomatik mesaj · ${m.sender.label}`} />
              {m.kind === "template" && <Row label="Şablon" value={m.sender.label ?? "—"} />}
              {m.media?.name && <Row label="Dosya" value={m.media.name} />}
              <Row label="Oluşturuldu" value={when(m.createdAt) ?? "—"} />
            </section>
            {m.direction !== "out" && (
              <section className="bg-card px-5 py-3">
                <p className="mb-2 flex items-center gap-2 text-[0.8rem] font-medium text-muted-foreground"><Eye className="size-4" /> Ekipte görenler</p>
                {reads === null ? <p className="text-sm text-muted-foreground">Yükleniyor...</p> : seen.length === 0 ? <p className="text-sm text-muted-foreground">Henüz ekipten gören yok.</p> : (
                  <div className="space-y-1">
                    {seen.map((r) => (
                      <div key={r.user.id} className="flex items-center gap-3 py-1">
                        <UserAvatar userId={r.user.id} name={r.user.name} hasAvatar={r.user.hasAvatar} version={r.user.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
                        <span className="min-w-0 flex-1 truncate text-sm">{r.user.name}</span>
                        <span className="text-xs tabular-nums text-muted-foreground">{when(r.readAt)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </section>
            )}
          </div>
        </div>
      </aside>
    </div>
  );
}

function Step({ icon, label, value, muted, danger }: { icon: React.ReactNode; label: string; value: string; muted?: boolean; danger?: boolean }) {
  return (
    <div className={cn("flex items-start gap-4 border-b border-border/50 py-3 last:border-0", muted && "opacity-60")}>
      <span className="mt-0.5 shrink-0">{icon}</span>
      <span className="min-w-0 flex-1">
        <span className={cn("block text-sm font-medium", danger && "text-destructive")}>{label}</span>
        <span className="block text-[0.8rem] text-muted-foreground">{value}</span>
      </span>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 py-1.5">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate text-right font-medium">{value}</span>
    </div>
  );
}
