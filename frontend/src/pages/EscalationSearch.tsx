import { useEffect, useState } from "react";
import { Search } from "lucide-react";
import { api } from "../api/client";
import type { EscalationRecord } from "../api/types";
import { displayNumber } from "../softphone/dial";
import { Badge, Card } from "../components/ui";

export function EscalationSearch() {
  const [number, setNumber] = useState("");
  const [records, setRecords] = useState<EscalationRecord[]>([]);
  const [searched, setSearched] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const key = number.trim();
    if (!key) { setRecords([]); setSearched(false); return; }
    setLoading(true);
    const t = window.setTimeout(() => {
      api
        .escalationHistory(key)
        .then((r) => { setRecords(r); setSearched(true); })
        .catch(() => setRecords([]))
        .finally(() => setLoading(false));
    }, 350);
    return () => window.clearTimeout(t);
  }, [number]);

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <Card title="Müşteriye Göre Eskalasyon Ara">
        <div className="space-y-4">
          <div className="relative">
            <Search className="pointer-events-none absolute left-4 top-1/2 size-5 -translate-y-1/2 text-muted-foreground" />
            <input
              value={number}
              onChange={(e) => setNumber(e.target.value)}
              placeholder="Müşteri numarası girin (örn. 05304230113)"
              inputMode="tel"
              autoFocus
              className="h-14 w-full rounded-2xl border border-border/70 bg-muted/40 pl-12 pr-4 text-lg outline-none focus-visible:border-ring/60 focus-visible:bg-card"
            />
          </div>

          {loading ? (
            <p className="py-8 text-center text-sm text-muted-foreground">Aranıyor...</p>
          ) : !number.trim() ? (
            <p className="py-10 text-center text-sm text-muted-foreground">Bir müşteri numarası girerek geçmiş eskalasyonları görüntüleyin.</p>
          ) : searched && records.length === 0 ? (
            <p className="py-10 text-center text-sm text-muted-foreground">
              {displayNumber(number)} için eskalasyon kaydı bulunamadı.
            </p>
          ) : (
            <div className="space-y-2.5">
              <p className="text-sm text-muted-foreground">
                {displayNumber(number)} · <span className="font-medium text-foreground">{records.length}</span> kayıt
              </p>
              <ul className="space-y-2.5">
                {records.map((r) => (
                  <li key={r.id} className="rounded-xl bg-muted/30 p-4 ring-1 ring-border/50">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-semibold">{r.agentName} görüştü</span>
                      <span className="text-xs text-muted-foreground">{r.createdAt}</span>
                    </div>
                    <div className="mt-1.5 flex flex-wrap items-center gap-2">
                      <Badge tone="amber">{r.categoryName}</Badge>
                      <span className="text-sm text-muted-foreground">{r.reasonName}</span>
                    </div>
                    {r.note && <p className="mt-2 text-sm text-foreground/80">{r.note}</p>}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
