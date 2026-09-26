// EventsTab lists the notices from Meta that could not be processed after
// every retry, so nothing is lost silently. Each can be tried again.

import { useEffect, useState } from "react";
import { CheckCircle2, RefreshCw, ShieldAlert } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card, EmptyState } from "@/components/ui";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAEventRow } from "@/whatsapp/types";
import { since } from "@/whatsapp/util";

export default function EventsTab({ channels }: { channels: WAChannel[] }) {
  const [rows, setRows] = useState<WAEventRow[]>([]);
  const [busy, setBusy] = useState<number | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const now = Date.now();
  const load = () => waApi.events().then(setRows).catch(() => setRows([]));
  useEffect(() => { void load(); }, []);
  const retry = async (id: number) => {
    setBusy(id);
    setMsg(null);
    try {
      await waApi.retryEvent(id);
      window.setTimeout(() => void load(), 1500);
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Olmadı.");
    } finally {
      setBusy(null);
    }
  };
  return (
    <Card title="İşlenemeyen bildirimler" icon={ShieldAlert} actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw /> Yenile</Button>}>
      <p className="mb-4 text-sm text-muted-foreground">Meta'dan gelen her bildirim önce kaydedilir, sonra işlenir. İşlenemeyenler birkaç kez yeniden denenir. Hâlâ olmayanlar burada kalır ve yöneticilere haber verilir; hiçbir mesaj sessizce kaybolmaz.</p>
      {msg && <p className="mb-3 text-sm text-destructive">{msg}</p>}
      {rows.length === 0 ? (
        <EmptyState icon={<CheckCircle2 />} title="Her şey yolunda" description="İşlenemeyen bildirim yok." />
      ) : (
        <div className="divide-y divide-border/50 overflow-hidden rounded-2xl ring-1 ring-border/60">
          {rows.map((r) => (
            <div key={r.id} className="flex items-start gap-3 px-4 py-3">
              <span className={cn("mt-1 size-2 shrink-0 rounded-full", r.status === "failed" ? "bg-destructive" : r.status === "done" ? "bg-success" : "bg-warning")} />
              <div className="min-w-0 flex-1 space-y-0.5">
                <p className="text-sm font-medium">{r.summary || "Bildirim"} <span className="font-normal text-muted-foreground">· {channels.find((c) => c.id === r.channelId)?.name ?? "cihaz bilinmiyor"}</span></p>
                <p className="text-xs text-destructive/90">{r.lastError}</p>
                <p className="text-[0.68rem] text-muted-foreground">{since(r.receivedAt, now)} önce geldi · {r.attempts} deneme</p>
              </div>
              <Button variant="secondary" className="h-8 px-3 text-xs" disabled={busy === r.id} onClick={() => void retry(r.id)}><RefreshCw className={cn(busy === r.id && "animate-spin")} /> Yeniden dene</Button>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}
