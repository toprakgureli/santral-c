// AgentCallsDialog lists one agent's calls in the chosen days: reached
// ones first, the unreached shown on request, with a number search and
// the day's totals on top. Opened from the team card, so the card itself
// never grows.

import { useEffect, useMemo, useState } from "react";
import { PhoneIncoming, PhoneMissed, PhoneOutgoing, Search, X } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { AgentCalls, TeamRow } from "@/api/types";
import { Modal, Skeleton } from "@/components/ui";
import { ListRow } from "@/components/ui/rows";
import UserAvatar from "@/components/ui/UserAvatar";
import { rangeLabel } from "@/lib/dateRange";
import { cn } from "@/lib/utils";
import { formatClock } from "@/pages/callFormat";
import { displayNumber } from "@/softphone/dial";

function stamp(iso: string, withDay: boolean) {
  const d = new Date(iso);
  const t = d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
  return withDay ? `${d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit" })} ${t}` : t;
}

const REASON: Record<string, string> = {
  no_answer: "Cevap vermedi",
  missed: "Kaçırıldı",
  busy: "Meşgul",
  canceled: "Çalarken kapatıldı",
  failed: "Başarısız",
};

export default function AgentCallsDialog({ row, from, to, onClose }: { row: TeamRow; from: string; to: string; onClose: () => void }) {
  const [data, setData] = useState<AgentCalls | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const [showUnreached, setShowUnreached] = useState(false);
  const multiDay = from !== to;

  useEffect(() => {
    let live = true;
    api
      .agentCalls({ userId: row.userId, from, to })
      .then((d) => live && setData(d))
      .catch((e) => live && setError(e instanceof ApiError ? e.message : "Çağrılar alınamadı."));
    return () => {
      live = false;
    };
  }, [row.userId, from, to]);

  const items = useMemo(() => {
    if (!data) return [];
    const digits = q.replace(/\D/g, "");
    return data.items.filter((c) => {
      const answered = c.disposition === "answered";
      if (!showUnreached && !answered) return false;
      if (digits && !c.peer.replace(/\D/g, "").includes(digits) && !(c.peerName ?? "").toLocaleLowerCase("tr").includes(q.trim().toLocaleLowerCase("tr"))) return false;
      return true;
    });
  }, [data, q, showUnreached]);

  const answered = data?.items.filter((c) => c.disposition === "answered").length ?? 0;
  const unreached = (data?.items.length ?? 0) - answered;

  return (
    <Modal open onClose={onClose} title={`${row.name} · görüşmeler`} description={`${rangeLabel(from, to)} · ${answered} görüşme, ${unreached} ulaşılamayan`} size="lg">
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2 rounded-2xl bg-muted/40 p-2">
          <UserAvatar userId={row.userId} name={row.name} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
          <div className="relative min-w-[12rem] flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Numara ya da isim ara" inputMode="tel" className="h-9 w-full rounded-xl border border-transparent bg-card pl-9 pr-8 text-sm outline-none transition focus:border-ring/40 focus:ring-4 focus:ring-ring/15" />
            {q && (
              <button type="button" onClick={() => setQ("")} aria-label="Temizle" className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground hover:text-foreground"><X className="size-3.5" /></button>
            )}
          </div>
          <button
            type="button"
            onClick={() => setShowUnreached((v) => !v)}
            aria-pressed={showUnreached}
            className={cn("flex h-9 items-center gap-2 rounded-xl px-3 text-xs font-medium transition-colors", showUnreached ? "bg-destructive/10 text-destructive ring-1 ring-destructive/30" : "bg-card text-muted-foreground ring-1 ring-border/60 hover:text-foreground")}
          >
            <PhoneMissed className="size-3.5" /> {showUnreached ? "Ulaşılamayanlar gösteriliyor" : `Ulaşılamayanları gör${unreached ? ` (${unreached})` : ""}`}
          </button>
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}
        {!data && !error && (
          <div className="space-y-1">
            {[0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-12 w-full rounded-2xl" />)}
          </div>
        )}
        {data && items.length === 0 && (
          <p className="py-8 text-center text-sm text-muted-foreground">{data.items.length === 0 ? "Bu tarihlerde çağrı yok." : q ? "Eşleşen çağrı yok." : "Görüşme yok; ulaşılamayanları görmek için düğmeye bas."}</p>
        )}
        {items.length > 0 && (
          <ul className="max-h-[26rem] space-y-0.5 overflow-y-auto pr-1">
            {items.map((c) => {
              const ok = c.disposition === "answered";
              const real = ok && c.durationSeconds >= 30;
              return (
                <li key={c.uuid}>
                  <ListRow
                    icon={c.direction === "inbound" ? PhoneIncoming : c.direction === "outbound" ? PhoneOutgoing : PhoneIncoming}
                    tone={!ok ? "destructive" : real ? "success" : "warning"}
                    title={<span className="font-mono tabular-nums">{displayNumber(c.peer) || c.peer}{c.peerName && <span className="ml-2 font-sans font-normal text-muted-foreground">{c.peerName}</span>}</span>}
                    sub={<span>{c.direction === "inbound" ? "Gelen" : "Giden"} · {ok ? (real ? "Gerçek çağrı" : "Geçersiz, 30 sn altı") : (REASON[c.disposition] ?? "Ulaşılamadı")}</span>}
                    trailing={
                      <>
                        <span className="text-xs tabular-nums text-muted-foreground">{stamp(c.startedAt, multiDay)}</span>
                        <span className={cn("w-14 text-right font-mono text-sm tabular-nums", !ok ? "text-destructive" : real ? "text-success" : "text-warning")}>{ok ? formatClock(c.durationSeconds) : "—"}</span>
                      </>
                    }
                  />
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </Modal>
  );
}
