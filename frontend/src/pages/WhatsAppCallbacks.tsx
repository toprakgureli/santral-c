// WhatsAppCallbacks: customers who asked to be called back, from a chatbot
// or a rule. Call them from here and mark the request done.

import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, Check, MessageCircle, Phone, PhoneCall, RefreshCw } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, EmptyState } from "@/components/ui";
import { Toolbar } from "@/components/ui/rows";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { waApi } from "@/whatsapp/api";
import type { WACallback } from "@/whatsapp/types";
import { prettyPhone, since } from "@/whatsapp/util";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";

export function WhatsAppCallbacks() {
  const { user } = useAuth();
  const phone = useSoftphoneContext();
  const wa = useWhatsApp();
  const [rows, setRows] = useState<WACallback[]>([]);
  const [all, setAll] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const canCall = can(user, "call.originate") && phone.status === "registered";
  const now = Date.now();

  const load = () => {
    setLoading(true);
    waApi.callbacks(all).then(setRows).catch((e) => setError(e instanceof ApiError ? e.message : "Yüklenemedi.")).finally(() => setLoading(false));
  };
  useEffect(load, [all]); // eslint-disable-line react-hooks/exhaustive-deps
  // A new request arrives with a notice; refresh the list with it.
  useEffect(() => {
    if (wa.alerts.some((a) => a.text.includes("geri aranmak"))) load();
  }, [wa.alerts.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const open = useMemo(() => rows.filter((r) => r.status === "open"), [rows]);
  const done = useMemo(() => rows.filter((r) => r.status !== "open"), [rows]);

  const finish = async (id: number) => {
    setRows((cur) => cur.map((r) => (r.id === id ? { ...r, status: "done", doneBy: user?.name, doneAt: new Date().toISOString() } : r)));
    try {
      await waApi.doneCallback(id);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kapatılamadı.");
      load();
    }
  };

  const Row = ({ r }: { r: WACallback }) => (
    <div className={cn("flex flex-wrap items-center gap-3 rounded-2xl px-4 py-3 ring-1", r.status === "open" ? "bg-card ring-border/60" : "bg-muted/25 ring-border/40")}>
      <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-2xl", r.status === "open" ? "bg-teal-500/12 text-teal-600 dark:text-teal-400" : "bg-muted text-muted-foreground")}>{r.status === "open" ? <PhoneCall className="size-4" /> : <Check className="size-4" />}</span>
      <div className="min-w-0 flex-1">
        <p className="flex flex-wrap items-center gap-x-2 text-sm font-medium">
          {r.customer}
          <span className="font-mono text-xs font-normal text-muted-foreground tabular-nums">{prettyPhone(r.phone)}</span>
        </p>
        {r.note && <p className="mt-0.5 text-sm text-muted-foreground">{r.note}</p>}
        <p className="mt-0.5 text-[0.7rem] text-muted-foreground">
          {r.channelName} · {since(r.createdAt, now)} önce
          {r.status !== "open" && r.doneBy && <> · {r.doneBy} kapattı</>}
        </p>
      </div>
      <span className="flex items-center gap-1.5">
        {r.conversationId > 0 && <Link to={`/whatsapp/${r.conversationId}`} data-tip="Sohbeti aç" className="flex size-9 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground"><MessageCircle className="size-4" /></Link>}
        {canCall && <button type="button" data-tip="Ara" onClick={() => void phone.call("0" + r.phone.replace(/\D/g, "").replace(/^90/, "")).catch(() => undefined)} className="flex size-9 items-center justify-center rounded-xl bg-success/12 text-success hover:bg-success/20"><Phone className="size-4" /></button>}
        {r.status === "open" && <Button variant="secondary" className="h-9" onClick={() => void finish(r.id)}><Check /> Arandı</Button>}
      </span>
    </div>
  );

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Link to="/whatsapp" data-tip="Gelen kutusuna dön" className="flex size-9 items-center justify-center rounded-xl bg-card text-muted-foreground shadow-sm ring-1 ring-border/60 hover:text-foreground"><ArrowLeft className="size-4" /></Link>
        <div className="flex-1">
          <h1 className="text-lg font-semibold tracking-tight">Geri arama talepleri</h1>
          <p className="text-xs text-muted-foreground">{open.length ? `${open.length} müşteri aranmayı bekliyor` : "Bekleyen talep yok"}</p>
        </div>
      </div>
      <Toolbar>
        <button type="button" onClick={() => setAll(false)} className={cn("rounded-xl px-3 py-1.5 text-sm font-medium", !all ? "bg-card shadow-sm ring-1 ring-border/60" : "text-muted-foreground")}>Bekleyenler</button>
        <button type="button" onClick={() => setAll(true)} className={cn("rounded-xl px-3 py-1.5 text-sm font-medium", all ? "bg-card shadow-sm ring-1 ring-border/60" : "text-muted-foreground")}>Hepsi</button>
        <Button variant="ghost" className="ml-auto h-9" onClick={load}><RefreshCw className={cn(loading && "animate-spin")} /> Yenile</Button>
      </Toolbar>
      {error && <p className="text-sm text-destructive">{error}</p>}
      {!loading && rows.length === 0 ? (
        <div className="rounded-2xl bg-card ring-1 ring-border/60"><EmptyState icon={<PhoneCall />} title="Geri arama talebi yok" description="Chatbot'ta Geri arama kutusu kullanılınca talepler buraya düşer." /></div>
      ) : (
        <div className="space-y-2">
          {open.map((r) => <Row key={r.id} r={r} />)}
          {done.length > 0 && <p className="px-1 pt-3 text-xs font-medium text-muted-foreground">{all ? "Kapatılanlar" : "Son 24 saatte kapatılanlar"}</p>}
          {done.map((r) => <Row key={r.id} r={r} />)}
        </div>
      )}
    </div>
  );
}
