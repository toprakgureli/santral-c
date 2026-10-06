import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronDown, Copy, History, Phone, PhoneIncoming, PhoneOutgoing, Search } from "lucide-react";
import { api, ApiError } from "../../api/client";
import type { Call } from "../../api/types";
import { useSoftphoneContext } from "../../softphone/SoftphoneContext";
import WhatsAppIcon from "../icons/WhatsAppIcon";
import { whatsappNumber } from "../../lib/whatsapp";
import { useWhatsAppWrite } from "../../whatsapp/useWhatsAppWrite";
import { displayNumber, normalizeDial } from "../../softphone/dial";
import { Button, Card } from "../ui";
import { cn } from "../../lib/utils";
import { ContextMenu, type MenuItem } from "../ContextMenu";
import { callQuality, formatDuration, formatStamp } from "../../pages/callFormat";
import { useRealCallSeconds } from "../../lib/realCall";

export function CallHistory({ canCall }: { canCall: boolean }) {
  const realSeconds = useRealCallSeconds();
  const phone = useSoftphoneContext();
  const { write: writeWhatsApp } = useWhatsAppWrite();
  const [calls, setCalls] = useState<Call[]>([]);
  const [counts, setCounts] = useState({ short: 0, long: 0, unanswered: 0, inbound: 0, outbound: 0, inboundMissed: 0, outboundMissed: 0, inboundReal: 0, outboundReal: 0 });
  const [showMissed, setShowMissed] = useState(false);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const canDial = phone.status === "registered" && canCall;
  const inCall = phone.status === "in-call" || phone.status === "held";
  const copyTimer = useRef<number | null>(null);

  useEffect(() => {
    let live = true;
    const load = () => {
      api
        .recentCalls()
        .then((r) => {
          if (!live) return;
          setCalls(r.items);
          setCounts({ short: r.short, long: r.long, unanswered: r.unanswered, inbound: r.inbound ?? 0, outbound: r.outbound ?? 0, inboundMissed: r.inboundMissed ?? 0, outboundMissed: r.outboundMissed ?? 0, inboundReal: r.inboundReal ?? 0, outboundReal: r.outboundReal ?? 0 });
          setError(null);
          setLoading(false);
        })
        .catch((e) => {
          if (!live) return;
          setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi.");
          setLoading(false);
        });
    };
    load();
    // Our own store is authoritative and cheap; refresh often so a just-ended
    // call appears right away. It also resets at local midnight (server-side).
    const timer = window.setInterval(load, 10000);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, []);

  const copy = useCallback((raw: string) => {
    const val = displayNumber(raw);
    navigator.clipboard?.writeText(val).catch(() => undefined);
    setCopied(val);
    if (copyTimer.current) window.clearTimeout(copyTimer.current);
    copyTimer.current = window.setTimeout(() => setCopied(null), 1200);
  }, []);

  function rowMenu(e: React.MouseEvent, number: string) {
    e.preventDefault();
    if (!number) return;
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: `${displayNumber(number)} ara`, onClick: () => phone.call(normalizeDial(number)).catch(() => undefined), disabled: !canDial },
        { label: "Numarayı kopyala", onClick: () => copy(number) },
        ...(whatsappNumber(number) ? [{ label: "WhatsApp'tan yaz", onClick: () => writeWhatsApp(number) }] : []),
        { label: "Görüşmeye aktar", onClick: () => phone.transfer(normalizeDial(number)).catch(() => undefined), disabled: !inCall },
      ],
    });
  }


  const term = query.trim();
  const filtered = term ? calls.filter((c) => displayNumber(c.direction === "outbound" ? c.toNumber : c.fromNumber).includes(displayNumber(term))) : calls;

  return (
    <Card title="Çağrı Geçmişi" icon={History}>
      {loading ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : error ? (
        <p className="text-sm text-destructive">{error}</p>
      ) : (
        <>
          {/* Today's breakdown (resets at 00:00) */}
          {/* Reached: real conversations (the set threshold and up), split by direction. */}
          <div className="mb-2 rounded-2xl bg-success/[0.07] p-2 ring-1 ring-success/15">
            <div className="mb-1.5 px-1 text-xs font-semibold text-success">Ulaşılanlar</div>
            <div className="grid grid-cols-3 gap-2">
              <CountBox label="Gerçek çağrı" sub={`${realSeconds} saniye ve üstü`} value={counts.long} tone="green" />
              <CountBox label="Gelen" sub="gerçek çağrı" value={counts.inboundReal} tone="green" />
              <CountBox label="Giden" sub="gerçek çağrı" value={counts.outboundReal} tone="green" />
            </div>
          </div>
          {/* Not reached: unanswered plus too-short calls, folded away by default. */}
          <div className="mb-3 rounded-2xl bg-muted/30 p-2 ring-1 ring-border/40">
            <button
              type="button"
              onClick={() => setShowMissed((v) => !v)}
              className="flex w-full items-center justify-between px-1 text-xs font-semibold text-muted-foreground hover:text-foreground"
            >
              <span>
                Ulaşılamayanlar · {counts.unanswered + counts.short}
                <span className="ml-1 font-normal text-muted-foreground/80">({counts.unanswered} cevapsız, {counts.short} geçersiz)</span>
              </span>
              <ChevronDown className={cn("size-4 transition-transform", showMissed && "rotate-180")} />
            </button>
            {showMissed && (
              <div className="mt-1.5 space-y-2">
                <div className="grid grid-cols-3 gap-2">
                  <CountBox label="Cevapsız" sub="hiç bağlanmadı" value={counts.unanswered} tone="slate" />
                  <CountBox label="Gelen" sub="arayan, cevaplanmadı" value={counts.inboundMissed} tone="slate" />
                  <CountBox label="Giden" sub="aradın, açılmadı" value={counts.outboundMissed} tone="slate" />
                </div>
                <div className="grid grid-cols-3 gap-2">
                  <CountBox label="Geçersiz çağrı" sub={`bağlandı, ${realSeconds} saniye dolmadı`} value={counts.short} tone="amber" />
                </div>
              </div>
            )}
          </div>

          <div className="relative mb-2">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Numaraya göre ara"
              inputMode="tel"
              className="h-9 w-full rounded-full border border-transparent bg-muted/60 pl-9 pr-3 text-sm outline-none transition-[background-color,box-shadow] placeholder:text-muted-foreground/60 focus-visible:border-ring/40 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/15"
            />
          </div>

          {filtered.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">{calls.length === 0 ? "Bugün çağrı kaydı yok." : "Eşleşen çağrı yok."}</p>
          ) : (
          <ul className="max-h-[28rem] space-y-1 overflow-y-auto">
            {filtered.map((c) => {
              const counterpart = c.direction === "outbound" ? c.toNumber : c.fromNumber;
              const isOpen = open === c.uuid;
              const q = callQuality(c.disposition, c.durationSeconds);
              const Arrow = c.direction === "inbound" ? PhoneIncoming : PhoneOutgoing;
              return (
                <li key={c.uuid} className={cn("overflow-hidden rounded-2xl transition-[background-color,box-shadow]", isOpen && "bg-card shadow-sm ring-1 ring-border/60")}>
                  <button
                    onClick={() => setOpen(isOpen ? null : c.uuid)}
                    onContextMenu={(e) => rowMenu(e, counterpart)}
                    className="flex w-full items-center gap-3 rounded-2xl px-2.5 py-2 text-left transition-colors hover:bg-accent/60"
                  >
                    <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-xl", q.tone === "green" ? "bg-success/10 text-success" : q.tone === "amber" ? "bg-warning/12 text-warning" : q.tone === "blue" ? "bg-primary/10 text-primary" : "bg-muted/70 text-muted-foreground")}>
                      <Arrow className="size-4" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold tabular-nums">{displayNumber(counterpart) || "—"}</span>
                      <span className={cn("text-xs", q.text)}>{q.label}{c.durationSeconds > 0 && <span className="text-muted-foreground"> · {formatDuration(c.durationSeconds)}</span>}</span>
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">{formatStamp(c.startedAt)}</span>
                    <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", isOpen && "rotate-180")} />
                  </button>

                  {isOpen && (
                    <div className="space-y-2 px-3 pb-3 pt-1 text-sm">
                      <CopyRow label="Arayan" number={c.fromNumber} copied={copied} onCopy={copy} />
                      <CopyRow label="Aranan" number={c.toNumber} copied={copied} onCopy={copy} />
                      <div className="flex items-center gap-4 text-xs text-muted-foreground">
                        <span>Süre: {formatDuration(c.durationSeconds)}</span>
                        <span>{formatStamp(c.startedAt)}</span>
                      </div>
                      <div className="flex gap-2 pt-1">
                        <Button variant="secondary" className="h-8 px-3" onClick={() => phone.call(normalizeDial(counterpart)).catch(() => undefined)} disabled={!canDial || !counterpart}>
                          <Phone className="size-3.5" /> Ara
                        </Button>
                        {whatsappNumber(counterpart) && (
                          <Button variant="secondary" className="h-8 px-3" onClick={() => writeWhatsApp(counterpart)}>
                            <WhatsAppIcon className="size-3.5 text-emerald-600 dark:text-emerald-400" /> WhatsApp'tan yaz
                          </Button>
                        )}
                      </div>
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
          )}
        </>
      )}
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
    </Card>
  );
}

// minus is the unanswered share of the count, shown small and red beside it.
function CountBox({ label, sub, value, minus, tone }: { label: string; sub?: string; value: number; minus?: number; tone: "green" | "amber" | "slate" | "blue" }) {
  const toneClass: Record<string, string> = {
    green: "text-success",
    amber: "text-warning",
    slate: "text-muted-foreground",
    blue: "text-primary",
  };
  return (
    <div className="rounded-xl bg-card/80 px-3 py-2 text-center ring-1 ring-border/40">
      <div className={cn("text-xl font-bold tabular-nums leading-none", toneClass[tone])}>
        {value}
        {minus ? <span className="ml-1 align-middle text-xs font-semibold text-destructive" data-tip="Bağlanmayan">-{minus}</span> : null}
      </div>
      <div className="mt-1 text-[0.7rem] font-medium text-foreground/80">{label}</div>
      {sub && <div className="text-[0.65rem] text-muted-foreground">{sub}</div>}
    </div>
  );
}

function CopyRow({ label, number, copied, onCopy }: { label: string; number: string; copied: string | null; onCopy: (n: string) => void }) {
  const shown = displayNumber(number);
  const isCopied = copied === shown && !!shown;
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <button
        onClick={() => number && onCopy(number)}
        data-tip="Kopyala"
        className="flex items-center gap-1.5 rounded-md px-2 py-1 font-medium tabular-nums transition hover:bg-accent"
      >
        {shown || "—"}
        {number && <Copy className="size-3.5 text-muted-foreground" />}
        {isCopied && <span className="text-xs text-success">kopyalandı</span>}
      </button>
    </div>
  );
}
