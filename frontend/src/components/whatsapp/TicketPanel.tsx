// TicketPanel is the right column: the customer's card (name, tags, note,
// marketing permission), the conversation's card (state, priority, who is
// on it, how long the customer waited, the score) and past conversations.

import { useEffect, useState } from "react";
import { BellOff, Ban, Clock, History, Pencil, Star, Tag, UserRound, Users, X } from "lucide-react";
import { ApiError } from "@/api/client";
import UserAvatar from "@/components/ui/UserAvatar";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAConversation, WAHistoryItem } from "@/whatsapp/types";
import { PRIORITY_WORD, STATUS_WORD, listTime, prettyPhone, since } from "@/whatsapp/util";

export default function TicketPanel({ conv, canEditContact, canEditTicket, onOpen, onClose }: { conv: WAConversation; canEditContact: boolean; canEditTicket: boolean; onOpen: (id: number) => void; onClose: () => void }) {
  const t = conv.ticket;
  const c = conv.contact;
  const [history, setHistory] = useState<WAHistoryItem[]>([]);
  const [editName, setEditName] = useState(false);
  const [name, setName] = useState(c.name);
  const [note, setNote] = useState(c.note);
  const [error, setError] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setName(c.name);
    setNote(c.note);
    setEditName(false);
  }, [c.id, c.name, c.note]);

  useEffect(() => {
    waApi.history(c.id).then(setHistory).catch(() => setHistory([]));
  }, [c.id, t?.status]);

  useEffect(() => {
    const i = window.setInterval(() => setNow(Date.now()), 30000);
    return () => window.clearInterval(i);
  }, []);

  const saveContact = async (body: Parameters<typeof waApi.updateContact>[1]) => {
    setError(null);
    try {
      await waApi.updateContact(c.id, body);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    }
  };
  const saveTicket = async (body: Parameters<typeof waApi.updateTicket>[1]) => {
    setError(null);
    try {
      await waApi.updateTicket(conv.id, body);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    }
  };

  return (
    <aside className="flex w-80 shrink-0 flex-col overflow-y-auto border-l border-border/50 bg-card/40">
      <div className="flex items-center justify-between px-4 pt-3">
        <p className="text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-muted-foreground/70">Müşteri</p>
        <button type="button" onClick={onClose} aria-label="Paneli kapat" data-tip="Paneli kapat" className="rounded-lg p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-4" /></button>
      </div>
      <div className="flex flex-col items-center gap-2 px-4 pt-3 pb-4 text-center">
        <ContactAvatar name={c.display} seed={c.waId} className="size-16 text-lg" />
        {editName ? (
          <input autoFocus value={name} onChange={(e) => setName(e.target.value)} onBlur={() => { setEditName(false); if (name !== c.name) void saveContact({ name }); }} onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()} placeholder={c.profileName || "İsim"} className="h-8 w-full rounded-lg border border-border/60 bg-card px-2 text-center text-sm outline-none focus:border-ring/50" />
        ) : (
          <button type="button" disabled={!canEditContact} onClick={() => setEditName(true)} className="group flex items-center gap-1.5 text-base font-semibold" data-tip={canEditContact ? "İsmi düzenle" : undefined}>
            {c.display}
            {canEditContact && <Pencil className="size-3.5 text-muted-foreground opacity-0 group-hover:opacity-100" />}
          </button>
        )}
        <button type="button" onClick={() => void navigator.clipboard?.writeText("0" + c.waId.replace(/^90/, ""))} className="font-mono text-xs tabular-nums text-muted-foreground hover:text-foreground" data-tip="Numarayı kopyala">{prettyPhone(c.waId)}</button>
        {c.profileName && c.name && c.profileName !== c.name && <p className="text-[0.7rem] text-muted-foreground">WhatsApp adı: {c.profileName}</p>}
        <p className="text-[0.7rem] text-muted-foreground">{conv.channelName}</p>
      </div>

      <Section icon={Tag} title="Müşteri etiketleri">
        <Tags values={c.tags} editable={canEditContact} onChange={(tags) => void saveContact({ tags })} />
      </Section>
      <Section icon={Pencil} title="Müşteri notu">
        <textarea value={note} disabled={!canEditContact} onChange={(e) => setNote(e.target.value)} onBlur={() => note !== c.note && void saveContact({ note })} rows={3} placeholder={canEditContact ? "Bu müşteriyle ilgili kalıcı bir not" : "Not yok"} className="w-full resize-none rounded-xl border border-border/60 bg-muted/30 px-3 py-2 text-xs outline-none focus:border-ring/50 disabled:opacity-70" />
        {canEditContact && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            <Toggle on={c.optedOut} icon={BellOff} label={c.optedOut ? "Kampanya mesajı almıyor" : "Kampanya mesajı alıyor"} onClick={() => void saveContact({ optedOut: !c.optedOut })} />
            <Toggle on={c.blocked} icon={Ban} label={c.blocked ? "Engellendi" : "Engelle"} danger onClick={() => void saveContact({ blocked: !c.blocked })} />
          </div>
        )}
      </Section>

      {t && (
        <>
          <div className="px-4 pt-4"><p className="text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-muted-foreground/70">Sohbet #{t.number}</p></div>
          <div className="space-y-1 px-2 pt-2">
            <Line label="Durum" value={STATUS_WORD[t.status] ?? t.status}>
              {canEditTicket && (t.status === "open" || t.status === "pending") && (
                <select value={t.status} onChange={(e) => void saveTicket({ status: e.target.value })} className="h-7 rounded-lg border border-border/60 bg-card px-2 text-xs outline-none">
                  <option value="open">Açık</option>
                  <option value="pending">Müşteri bekleniyor</option>
                </select>
              )}
            </Line>
            <Line label="Öncelik" value={PRIORITY_WORD[t.priority]}>
              {canEditTicket && (
                <select value={t.priority} onChange={(e) => void saveTicket({ priority: e.target.value })} className="h-7 rounded-lg border border-border/60 bg-card px-2 text-xs outline-none">
                  {Object.entries(PRIORITY_WORD).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                </select>
              )}
            </Line>
            <Line label="Konu" value={t.category || "—"}>
              {canEditTicket && <input defaultValue={t.category} key={t.category} onBlur={(e) => e.target.value !== t.category && void saveTicket({ category: e.target.value })} placeholder="Örn. Fatura" className="h-7 w-32 rounded-lg border border-border/60 bg-card px-2 text-xs outline-none" />}
            </Line>
            <Line label="Ekip" value={t.teamName || "—"} />
            <Line label="Açıldı" value={`${listTime(t.createdAt)} · ${since(t.createdAt, now)} önce`} />
            {t.firstResponseAt && <Line label="İlk cevap" value={`${Math.max(1, Math.round((Date.parse(t.firstResponseAt) - Date.parse(t.createdAt)) / 60000))} dk sonra`} />}
            {t.waitingCount > 0 && <Line label="Cevap Bekleyenler'e düştü" value={`${t.waitingCount} kez`} />}
            {t.reopenCount > 0 && <Line label="Yeniden açıldı" value={`${t.reopenCount} kez`} />}
            {t.rating != null && (
              <Line label="Müşteri puanı" value="">
                <span className="flex items-center gap-0.5">
                  {Array.from({ length: 5 }).map((_, i) => <Star key={i} className={cn("size-3.5", i < (t.rating ?? 0) ? "fill-warning text-warning" : "text-muted-foreground/40")} />)}
                </span>
              </Line>
            )}
            {t.ratingComment && <p className="px-2 pb-1 text-xs italic text-muted-foreground">“{t.ratingComment}”</p>}
          </div>
          <Section icon={Tag} title="Sohbet etiketleri">
            <Tags values={t.tags} editable={canEditTicket} onChange={(tags) => void saveTicket({ tags })} />
          </Section>
          <Section icon={Users} title="İlgilenenler">
            {t.participants.length === 0 && <p className="text-xs text-muted-foreground">{t.status === "bot" ? "Müşteri şu an chatbot ile konuşuyor." : "Henüz kimse üstlenmedi."}</p>}
            <div className="space-y-1">
              {t.participants.map((p) => (
                <div key={p.id} className="flex items-center gap-2.5 rounded-xl px-1 py-1">
                  <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">{p.name}</span>
                  <span className={cn("rounded-full px-2 py-0.5 text-[0.6rem] font-semibold", p.role === "owner" ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground")}>{p.role === "owner" ? "Sorumlu" : "Yardımcı"}</span>
                </div>
              ))}
            </div>
            {t.resolvedBy && <p className="mt-2 text-xs text-muted-foreground">Çözen: <span className="font-medium text-foreground/80">{t.resolvedBy.name}</span></p>}
          </Section>
        </>
      )}

      <Section icon={History} title="Önceki sohbetler">
        {history.filter((h) => h.conversationId !== conv.id || h.ticketId !== t?.id).length === 0 && <p className="text-xs text-muted-foreground">Başka sohbet yok.</p>}
        <div className="space-y-1">
          {history.filter((h) => h.ticketId !== t?.id).map((h) => (
            <button key={h.ticketId} type="button" onClick={() => onOpen(h.conversationId)} className="flex w-full items-center gap-2.5 rounded-xl px-1.5 py-1.5 text-left hover:bg-accent/60">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground"><Clock className="size-4" /></span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-xs font-medium">#{h.number} · {h.channelName}</span>
                <span className="block truncate text-[0.65rem] text-muted-foreground">{listTime(h.createdAt)} · {h.owner || "sahipsiz"} · {h.messages} mesaj{h.rating ? ` · ${h.rating}/5` : ""}</span>
              </span>
              <span className="text-[0.6rem] text-muted-foreground">{STATUS_WORD[h.status]}</span>
            </button>
          ))}
        </div>
      </Section>
      {error && <p className="px-4 pb-3 text-xs text-destructive">{error}</p>}
    </aside>
  );
}

function Section({ icon: Icon, title, children }: { icon: typeof UserRound; title: string; children: React.ReactNode }) {
  return (
    <section className="border-t border-border/40 px-4 py-3">
      <p className="mb-2 flex items-center gap-1.5 text-[0.7rem] font-semibold text-muted-foreground"><Icon className="size-3.5" /> {title}</p>
      {children}
    </section>
  );
}

function Line({ label, value, children }: { label: string; value: string; children?: React.ReactNode }) {
  return (
    <div className="flex min-h-9 items-center gap-2 rounded-xl px-2 py-1 text-xs">
      <span className="min-w-0 flex-1 text-muted-foreground">{label}</span>
      {children ?? <span className="truncate text-right font-medium">{value}</span>}
    </div>
  );
}

function Toggle({ on, icon: Icon, label, danger, onClick }: { on: boolean; icon: typeof UserRound; label: string; danger?: boolean; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[0.7rem] font-medium ring-1 transition-colors", on ? (danger ? "bg-destructive/10 text-destructive ring-destructive/30" : "bg-warning/12 text-warning ring-warning/30") : "text-muted-foreground ring-border/60 hover:bg-accent")}>
      <Icon className="size-3" /> {label}
    </button>
  );
}

function Tags({ values, editable, onChange }: { values: string[]; editable: boolean; onChange: (v: string[]) => void }) {
  const [draft, setDraft] = useState("");
  const add = () => {
    const v = draft.trim();
    if (!v) return;
    setDraft("");
    if (!values.some((x) => x.toLocaleLowerCase("tr") === v.toLocaleLowerCase("tr"))) onChange([...values, v]);
  };
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {values.map((t) => (
        <span key={t} className="flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-[0.7rem] font-medium text-primary">
          {t}
          {editable && <button type="button" onClick={() => onChange(values.filter((x) => x !== t))} aria-label={`${t} etiketini kaldır`} className="rounded-full hover:text-destructive"><X className="size-3" /></button>}
        </span>
      ))}
      {editable && (
        <input value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); add(); } }} onBlur={add} placeholder="+ etiket" className="h-6 w-20 rounded-full bg-muted/50 px-2 text-[0.7rem] outline-none focus:w-28 focus:bg-card focus:ring-2 focus:ring-ring/20" />
      )}
      {!editable && values.length === 0 && <span className="text-xs text-muted-foreground">Etiket yok</span>}
    </div>
  );
}
