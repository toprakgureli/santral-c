// WhatsAppMissedCalls: missed calls that were held for representative approval
// or tracked after automated dispatch. Send the template with one click or call back.

import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, Check, Phone, PhoneMissed, RefreshCw, Send, Settings2, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, EmptyState } from "@/components/ui";
import { Toolbar } from "@/components/ui/rows";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { waApi } from "@/whatsapp/api";
import type { WAMissedCallRecoveryItem } from "@/whatsapp/types";
import { prettyPhone, since } from "@/whatsapp/util";

export function WhatsAppMissedCalls() {
  const { user } = useAuth();
  const phone = useSoftphoneContext();
  const [rows, setRows] = useState<WAMissedCallRecoveryItem[]>([]);
  const [all, setAll] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const canCall = can(user, "call.originate") && phone.status === "registered";
  const canConfig = can(user, "whatsapp.missed_call_recovery");
  const now = Date.now();

  const latest = useRef(0);
  const load = () => {
    const id = ++latest.current;
    setLoading(true);
    setError(null);
    waApi
      .missedCallItems(all)
      .then((r) => id === latest.current && setRows(r))
      .catch((e) => id === latest.current && setError(e instanceof ApiError ? e.message : "Yüklenemedi."))
      .finally(() => id === latest.current && setLoading(false));
  };

  useEffect(load, [all]);

  const pending = useMemo(() => rows.filter((r) => r.status === "pending"), [rows]);

  const send = async (id: number) => {
    setBusyId(id);
    setError(null);
    try {
      const updated = await waApi.sendMissedCall(id);
      setRows((cur) => cur.map((r) => (r.id === id ? updated : r)));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Şablon gönderilemedi.");
      load();
    } finally {
      setBusyId(null);
    }
  };

  const dismiss = async (id: number) => {
    setBusyId(id);
    setError(null);
    try {
      const updated = await waApi.dismissMissedCall(id);
      setRows((cur) => cur.map((r) => (r.id === id ? updated : r)));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem yapılamadı.");
      load();
    } finally {
      setBusyId(null);
    }
  };

  const statusBadge = (s: WAMissedCallRecoveryItem["status"]) => {
    switch (s) {
      case "pending":
        return <span className="rounded-full bg-amber-500/12 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-400">Onay Bekliyor</span>;
      case "sent":
        return <span className="rounded-full bg-emerald-500/12 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:text-emerald-400">Gönderildi</span>;
      case "dismissed":
        return <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">İptal Edildi</span>;
      case "skipped":
        return <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">Atlandı</span>;
      case "failed":
        return <span className="rounded-full bg-destructive/12 px-2 py-0.5 text-[11px] font-medium text-destructive">Başarısız</span>;
    }
  };

  const Row = ({ r }: { r: WAMissedCallRecoveryItem }) => (
    <div
      className={cn(
        "flex flex-wrap items-center gap-3 rounded-2xl px-4 py-3 ring-1 transition-colors",
        r.status === "pending" ? "bg-card ring-border/70" : "bg-muted/25 ring-border/40",
      )}
    >
      <span
        className={cn(
          "flex size-10 shrink-0 items-center justify-center rounded-2xl",
          r.status === "pending"
            ? "bg-amber-500/12 text-amber-600 dark:text-amber-400"
            : r.status === "sent"
              ? "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400"
              : "bg-muted text-muted-foreground",
        )}
      >
        {r.status === "sent" ? <Check className="size-4" /> : <PhoneMissed className="size-4" />}
      </span>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <p className="text-sm font-semibold">
            {r.customer || prettyPhone(r.peerNumber)}
          </p>
          {r.customer && (
            <span className="font-mono text-xs text-muted-foreground tabular-nums">
              {prettyPhone(r.peerNumber)}
            </span>
          )}
          {statusBadge(r.status)}
        </div>

        {r.note && <p className="mt-0.5 text-xs text-muted-foreground">{r.note}</p>}

        <p className="mt-0.5 text-[0.7rem] text-muted-foreground">
          {r.channelName ? `${r.channelName} · ` : ""}
          {r.template ? `Şablon: ${r.template} · ` : ""}
          {since(r.createdAt, now)} önce
        </p>
      </div>

      <span className="flex items-center gap-2">
        {canCall && (
          <button
            type="button"
            aria-label="Santralden ara"
            data-tip="Santralden ara"
            onClick={() => void phone.call("0" + r.peerNumber.replace(/\D/g, "").replace(/^90/, "")).catch(() => undefined)}
            className="flex size-9 items-center justify-center rounded-xl bg-success/12 text-success hover:bg-success/20"
          >
            <Phone className="size-4" />
          </button>
        )}

        {r.status === "pending" && (
          <>
            <Button
              className="h-9 gap-1.5"
              disabled={busyId === r.id}
              onClick={() => void send(r.id)}
            >
              <Send className="size-3.5" />
              {busyId === r.id ? "Gönderiliyor..." : "Şablonu Gönder"}
            </Button>
            <Button
              variant="secondary"
              className="h-9 gap-1.5"
              disabled={busyId === r.id}
              onClick={() => void dismiss(r.id)}
            >
              <X className="size-3.5" />
              İptal
            </Button>
          </>
        )}
      </span>
    </div>
  );

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Link
          to="/whatsapp"
          aria-label="Gelen kutusuna dön"
          data-tip="Gelen kutusuna dön"
          className="flex size-9 items-center justify-center rounded-xl bg-card text-muted-foreground shadow-sm ring-1 ring-border/60 hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
        </Link>
        <div className="flex-1">
          <h1 className="text-lg font-semibold tracking-tight">Kaçan çağrı kurtarma</h1>
          <p className="text-xs text-muted-foreground">
            {pending.length ? `${pending.length} çağrı onay bekliyor` : "Onay bekleyen kaçan çağrı yok"}
          </p>
        </div>
        {canConfig && (
          <Link
            to="/whatsapp/settings?tab=missed-call"
            className="flex items-center gap-1.5 rounded-xl border border-border/60 bg-card px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <Settings2 className="size-3.5" /> Ayarlar
          </Link>
        )}
      </div>

      <Toolbar>
        <button
          type="button"
          onClick={() => setAll(false)}
          className={cn(
            "rounded-xl px-3 py-1.5 text-sm font-medium transition-colors",
            !all ? "bg-card shadow-sm ring-1 ring-border/60" : "text-muted-foreground",
          )}
        >
          Bekleyenler {pending.length > 0 && `(${pending.length})`}
        </button>
        <button
          type="button"
          onClick={() => setAll(true)}
          className={cn(
            "rounded-xl px-3 py-1.5 text-sm font-medium transition-colors",
            all ? "bg-card shadow-sm ring-1 ring-border/60" : "text-muted-foreground",
          )}
        >
          Hepsi
        </button>
        <Button variant="ghost" className="ml-auto h-9" onClick={load}>
          <RefreshCw className={cn(loading && "animate-spin")} /> Yenile
        </Button>
      </Toolbar>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {!loading && rows.length === 0 ? (
        <div className="rounded-2xl bg-card ring-1 ring-border/60">
          <EmptyState
            icon={<PhoneMissed />}
            title="Kaçan çağrı yok"
            description={
              all
                ? "Henüz kaydedilmiş bir kaçan çağrı bulunmuyor."
                : "Temsilci onayı bekleyen kaçan çağrı yok. Santralde cevapsız kalan aramalar onaylı modda buraya düşer."
            }
          />
        </div>
      ) : (
        <div className="space-y-2">
          {rows.map((r) => (
            <Row key={r.id} r={r} />
          ))}
        </div>
      )}
    </div>
  );
}
