// TicketPanel is the contact card on the right: the customer at the top
// with a few quick actions, then short cards for the conversation (state,
// priority, subject, tags), who is on it, the customer's note and tags.
// Details and past conversations fold away; blocking sits at the bottom.

import { useEffect, useState } from "react";
import { Ban, Bell, BellOff, ChevronDown, Copy, Megaphone, MegaphoneOff, Pencil, Phone, Star, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import UserAvatar from "@/components/ui/UserAvatar";
import AdSource, { asReferral } from "@/components/whatsapp/AdSource";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import { MUTES } from "@/components/whatsapp/ConversationList";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { waApi } from "@/whatsapp/api";
import type { WAConversation, WAHistoryItem } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { PRIORITY_WORD, STATUS_WORD, listTime, prettyPhone, since } from "@/whatsapp/util";

export default function TicketPanel({ conv, canEditContact, canEditTicket, onOpen, onClose }: { conv: WAConversation; canEditContact: boolean; canEditTicket: boolean; onOpen: (id: number) => void; onClose: () => void }) {
  const t = conv.ticket;
  const c = conv.contact;
  const wa = useWhatsApp();
  const { user } = useAuth();
  const phone = useSoftphoneContext();
  const [history, setHistory] = useState<WAHistoryItem[]>([]);
  const [editName, setEditName] = useState(false);
  const [name, setName] = useState(c.name);
  const [note, setNote] = useState(c.note);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [muteOpen, setMuteOpen] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const muted = wa.muted(conv.id);
  const canCall = can(user, "call.originate") && phone.status === "registered";

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

  const past = history.filter((h) => h.ticketId !== t?.id);
  const ad = asReferral(c.source);

  return (
    <aside className="flex h-full w-full flex-col bg-muted md:w-[22rem] md:shrink-0 md:border-l md:border-border/60">
      <header className="flex h-16 shrink-0 items-center gap-3 bg-card px-4">
        <button type="button" onClick={onClose} aria-label="Kapat" className="flex size-9 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-5" /></button>
        <p className="text-[0.95rem] font-semibold">Kişi bilgisi</p>
      </header>

      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto pb-4">
        {/* the customer */}
        <section className="flex flex-col items-center bg-card px-5 pt-5 pb-4 text-center">
          <ContactAvatar name={c.display} seed={c.waId} className="size-24 text-2xl" />
          {editName ? (
            <input autoFocus value={name} onChange={(e) => setName(e.target.value)} onBlur={() => { setEditName(false); if (name !== c.name) void saveContact({ name }); }} onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()} placeholder={c.profileName || "İsim"} className="mt-3 h-9 w-full rounded-lg bg-muted/70 px-2 text-center text-base outline-none focus:ring-2 focus:ring-wa-accent/30" />
          ) : (
            <button type="button" disabled={!canEditContact} onClick={() => setEditName(true)} className="group mt-3 flex items-center gap-1.5 text-lg font-semibold" data-tip={canEditContact ? "İsmi düzenle" : undefined}>
              {c.display}
              {canEditContact && <Pencil className="size-3.5 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />}
            </button>
          )}
          <p className="mt-0.5 font-mono text-sm tabular-nums text-muted-foreground">{prettyPhone(c.waId)}</p>
          <p className="text-xs text-muted-foreground">{c.profileName && c.name && c.profileName !== c.name ? `WhatsApp adı: ${c.profileName} · ` : ""}{conv.channelName}</p>
          <div className="mt-4 flex w-full justify-center gap-2">
            {canCall && <Quick icon={Phone} label="Ara" onClick={() => void phone.call("0" + c.waId.replace(/^90/, "")).catch(() => undefined)} />}
            <Quick icon={Copy} label={copied ? "Kopyalandı" : "Kopyala"} onClick={() => { void navigator.clipboard?.writeText("0" + c.waId.replace(/^90/, "")); setCopied(true); window.setTimeout(() => setCopied(false), 1400); }} />
            <span className="relative">
              <Quick icon={muted ? BellOff : Bell} label={muted ? "Sessizde" : "Sessize al"} on={muted} onClick={() => (muted ? void wa.setConvPref(conv.id, { mute: "off" }) : setMuteOpen((v) => !v))} />
              {muteOpen && (
                <span className="absolute left-1/2 top-full z-20 mt-1 w-44 -translate-x-1/2 rounded-2xl border border-border bg-popover p-1 text-left shadow-xl">
                  {MUTES.map((m) => <button key={m.key} type="button" onClick={() => { setMuteOpen(false); void wa.setConvPref(conv.id, { mute: m.key }); }} className="block w-full rounded-xl px-3 py-1.5 text-sm hover:bg-accent">{m.label}</button>)}
                </span>
              )}
            </span>
          </div>
        </section>

        {ad && (
          <Card title="Nereden geldi">
            <AdSource r={ad} />
          </Card>
        )}

        {/* the conversation */}
        {t && (
          <Card title={`Sohbet #${t.number}`}>
            <div className="divide-y divide-border/50">
              <Field label="Durum">
                {canEditTicket && (t.status === "open" || t.status === "pending") ? (
                  <select value={t.status} onChange={(e) => void saveTicket({ status: e.target.value })} className="h-8 rounded-lg bg-muted/70 px-2 text-sm outline-none">
                    <option value="open">Açık</option>
                    <option value="pending">Müşteri bekleniyor</option>
                  </select>
                ) : <span className="text-sm font-medium">{STATUS_WORD[t.status] ?? t.status}</span>}
              </Field>
              <Field label="Öncelik">
                {canEditTicket ? (
                  <select value={t.priority} onChange={(e) => void saveTicket({ priority: e.target.value })} className={cn("h-8 rounded-lg bg-muted/70 px-2 text-sm outline-none", (t.priority === "high" || t.priority === "urgent") && "text-destructive")}>
                    {Object.entries(PRIORITY_WORD).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                  </select>
                ) : <span className="text-sm font-medium">{PRIORITY_WORD[t.priority]}</span>}
              </Field>
              <Field label="Konu">
                {canEditTicket
                  ? <input defaultValue={t.category} key={t.category} onBlur={(e) => e.target.value !== t.category && void saveTicket({ category: e.target.value })} placeholder="Örn. Fatura" className="h-8 w-36 rounded-lg bg-muted/70 px-2 text-right text-sm outline-none focus:bg-card focus:ring-2 focus:ring-wa-accent/30" />
                  : <span className="text-sm font-medium">{t.category || "—"}</span>}
              </Field>
              <div className="py-2.5">
                <p className="mb-1.5 text-xs text-muted-foreground">Etiketler</p>
                <Tags values={t.tags} editable={canEditTicket} onChange={(tags) => void saveTicket({ tags })} />
              </div>
            </div>
          </Card>
        )}

        {t && (
          <Card title="İlgilenenler">
            {t.participants.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t.status === "bot" ? "Müşteri şu an chatbot ile konuşuyor." : "Henüz kimse üstlenmedi."}</p>
            ) : (
              <div className="space-y-1">
                {t.participants.map((p) => (
                  <div key={p.id} className="flex items-center gap-3 py-1">
                    <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-9" fallbackClassName="bg-primary/10 text-xs text-primary" />
                    <span className="min-w-0 flex-1 truncate text-sm">{p.name}</span>
                    <span className={cn("rounded-full px-2 py-0.5 text-[0.65rem] font-semibold", p.role === "owner" ? "bg-wa-accent/15 text-wa-accent" : "bg-muted text-muted-foreground")}>{p.role === "owner" ? "Sorumlu" : "Yardımcı"}</span>
                  </div>
                ))}
              </div>
            )}
            {t.teamName && <p className="mt-2 text-xs text-muted-foreground">Ekip: <span className="font-medium text-foreground/80">{t.teamName}</span></p>}
          </Card>
        )}

        <Card title="Müşteri notu">
          <textarea value={note} disabled={!canEditContact} onChange={(e) => setNote(e.target.value)} onBlur={() => note !== c.note && void saveContact({ note })} rows={3} placeholder={canEditContact ? "Bu müşteriyle ilgili kalıcı bir not yazın" : "Not yok"} className="w-full resize-none rounded-lg bg-muted/60 px-3 py-2 text-sm outline-none focus:bg-card focus:ring-2 focus:ring-wa-accent/30 disabled:opacity-70" />
          <p className="mt-3 mb-1.5 text-xs text-muted-foreground">Müşteri etiketleri</p>
          <Tags values={c.tags} editable={canEditContact} onChange={(tags) => void saveContact({ tags })} />
        </Card>

        {t && (
          <Fold title="Ayrıntılar">
            <div className="divide-y divide-border/50 text-sm">
              <Detail label="Açıldı" value={`${listTime(t.createdAt)} · ${since(t.createdAt, now)} önce`} />
              {t.firstResponseAt && <Detail label="İlk cevap" value={`${Math.max(1, Math.round((Date.parse(t.firstResponseAt) - Date.parse(t.createdAt)) / 60000))} dk sonra`} />}
              {t.waitingCount > 0 && <Detail label="Cevap bekleyenlere düştü" value={`${t.waitingCount} kez`} />}
              {t.reopenCount > 0 && <Detail label="Yeniden açıldı" value={`${t.reopenCount} kez`} />}
              {t.resolvedBy && <Detail label="Çözen" value={t.resolvedBy.name} />}
              {t.rating != null && (
                <div className="flex items-center justify-between py-2">
                  <span className="text-muted-foreground">Müşteri puanı</span>
                  <span className="flex items-center gap-0.5">{Array.from({ length: 5 }).map((_, i) => <Star key={i} className={cn("size-3.5", i < (t.rating ?? 0) ? "fill-warning text-warning" : "text-muted-foreground/40")} />)}</span>
                </div>
              )}
              {t.ratingComment && <p className="py-2 text-xs italic text-muted-foreground">“{t.ratingComment}”</p>}
            </div>
          </Fold>
        )}

        <Fold title="Önceki sohbetler" count={past.length}>
          {past.length === 0 ? <p className="text-sm text-muted-foreground">Başka sohbet yok.</p> : (
            <div className="-mx-2 space-y-0.5">
              {past.map((h) => (
                <button key={h.ticketId} type="button" onClick={() => onOpen(h.conversationId)} className="flex w-full items-center gap-3 rounded-xl px-2 py-2 text-left hover:bg-accent/60">
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">#{h.number} · {h.channelName}</span>
                    <span className="block truncate text-xs text-muted-foreground">{listTime(h.createdAt)} · {h.owner || "sahipsiz"} · {h.messages} mesaj{h.rating ? ` · ${h.rating}/5` : ""}</span>
                  </span>
                  <span className="text-[0.7rem] text-muted-foreground">{STATUS_WORD[h.status]}</span>
                </button>
              ))}
            </div>
          )}
        </Fold>

        {canEditContact && (
          <section className="bg-card py-1">
            <Action icon={c.optedOut ? Megaphone : MegaphoneOff} label={c.optedOut ? "Kampanya mesajlarına yeniden izin ver" : "Kampanya mesajlarını kapat"} sub={c.optedOut ? "Şu an kampanya mesajı almıyor" : undefined} onClick={() => void saveContact({ optedOut: !c.optedOut })} />
            <Action icon={Ban} label={c.blocked ? "Engeli kaldır" : "Müşteriyi engelle"} danger={!c.blocked} onClick={() => void saveContact({ blocked: !c.blocked })} />
          </section>
        )}
        {error && <p className="px-4 text-xs text-destructive">{error}</p>}
      </div>
    </aside>
  );
}

function Quick({ icon: Icon, label, onClick, on }: { icon: typeof Phone; label: string; onClick: () => void; on?: boolean }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex w-24 flex-col items-center gap-1.5 rounded-2xl border border-border/60 py-2.5 text-xs font-medium transition-colors hover:bg-accent", on ? "text-wa-accent" : "text-foreground/80")}>
      <Icon className="size-5 text-wa-accent" />
      {label}
    </button>
  );
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="bg-card px-5 py-3.5">
      <p className="mb-2 text-[0.8rem] font-medium text-muted-foreground">{title}</p>
      {children}
    </section>
  );
}

function Fold({ title, count, children }: { title: string; count?: number; children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <section className="bg-card">
      <button type="button" onClick={() => setOpen((v) => !v)} className="flex w-full items-center gap-2 px-5 py-3.5 text-left">
        <span className="flex-1 text-[0.8rem] font-medium text-muted-foreground">{title}</span>
        {count != null && count > 0 && <span className="text-xs tabular-nums text-muted-foreground">{count}</span>}
        <ChevronDown className={cn("size-4 text-muted-foreground transition-transform", open && "rotate-180")} />
      </button>
      {open && <div className="px-5 pb-3.5">{children}</div>}
    </section>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-3 py-1.5">
      <span className="text-sm text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 py-2">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate text-right font-medium">{value}</span>
    </div>
  );
}

function Action({ icon: Icon, label, sub, danger, onClick }: { icon: typeof Ban; label: string; sub?: string; danger?: boolean; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex w-full items-center gap-4 px-5 py-3 text-left text-sm transition-colors hover:bg-accent/60", danger ? "text-destructive" : "text-foreground/85")}>
      <Icon className="size-5 shrink-0" />
      <span className="min-w-0 flex-1">
        <span className="block">{label}</span>
        {sub && <span className="block text-xs text-muted-foreground">{sub}</span>}
      </span>
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
        <span key={t} className="flex items-center gap-1 rounded-full bg-wa-accent/12 px-2.5 py-0.5 text-xs font-medium text-wa-accent">
          {t}
          {editable && <button type="button" onClick={() => onChange(values.filter((x) => x !== t))} aria-label={`${t} etiketini kaldır`} className="rounded-full hover:text-destructive"><X className="size-3" /></button>}
        </span>
      ))}
      {editable && <input value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); add(); } }} onBlur={add} placeholder="+ etiket" className="h-7 w-20 rounded-full bg-muted/70 px-2.5 text-xs outline-none focus:w-28 focus:bg-card focus:ring-2 focus:ring-wa-accent/30" />}
      {!editable && values.length === 0 && <span className="text-sm text-muted-foreground">Etiket yok</span>}
    </div>
  );
}
