// ShareDialog shows the team picture and hands it over as a file or to the
// clipboard, with a prominent break usage and limit compliance summary.

import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, Check, CheckCircle2, Clock, Coffee, Copy, Download } from "lucide-react";
import { api } from "@/api/client";
import type { TeamRow } from "@/api/types";
import { Button, Modal } from "@/components/ui";
import { renderTeamImage } from "@/pages/teamShare";
import { useRealCallSeconds } from "@/lib/realCall";
import { cn } from "@/lib/utils";

function formatSec(seconds: number): string {
  const m = Math.round(seconds / 60);
  if (m < 60) return `${m} dk`;
  const hrs = Math.floor(m / 60);
  const rem = m % 60;
  if (rem === 0) return `${hrs} sa`;
  return `${hrs} sa ${rem} dk`;
}

export default function ShareDialog({
  open,
  rows,
  rangeLabel,
  scopeLabel,
  from,
  to,
  onClose,
}: {
  open: boolean;
  rows: TeamRow[];
  rangeLabel: string;
  scopeLabel: string;
  from?: string;
  to?: string;
  onClose: () => void;
}) {
  const [canvas, setCanvas] = useState<HTMLCanvasElement | null>(null);
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [breakLimitMinutes, setBreakLimitMinutes] = useState(60);
  const realSeconds = useRealCallSeconds();

  const dayCount = useMemo(() => {
    if (!from || !to) return 1;
    const d1 = Date.parse(from);
    const d2 = Date.parse(to);
    if (isNaN(d1) || isNaN(d2) || d2 < d1) return 1;
    return Math.max(1, Math.round((d2 - d1) / 86400000) + 1);
  }, [from, to]);

  // Fetch the active break limit configuration
  useEffect(() => {
    if (!open) return;
    let live = true;
    if (typeof api?.breakLimit === "function") {
      api
        .breakLimit()
        .then((r) => {
          if (live && r && typeof r.minutes === "number" && r.minutes > 0) {
            setBreakLimitMinutes(r.minutes);
          }
        })
        .catch(() => {
          // keep default 60 minutes
        });
    }
    return () => {
      live = false;
    };
  }, [open]);

  const limitSeconds = breakLimitMinutes * 60 * dayCount;

  const { totalBreakSeconds, exceededAgents, withinLimitAgents, noBreakAgents } = useMemo(() => {
    let total = 0;
    const exceeded: TeamRow[] = [];
    const within: TeamRow[] = [];
    const noBreak: TeamRow[] = [];
    for (const r of rows) {
      const b = r.breakSeconds || 0;
      total += b;
      if (b > limitSeconds) {
        exceeded.push(r);
      } else if (b > 0) {
        within.push(r);
      } else {
        noBreak.push(r);
      }
    }
    return {
      totalBreakSeconds: total,
      exceededAgents: exceeded,
      withinLimitAgents: within,
      noBreakAgents: noBreak,
    };
  }, [rows, limitSeconds]);

  useEffect(() => {
    if (!open) return;
    let live = true;
    setCanvas(null);
    setUrl(null);
    setError(null);
    renderTeamImage(rows, rangeLabel, scopeLabel, realSeconds, breakLimitMinutes, dayCount)
      .then((c) => {
        if (!live) return;
        setCanvas(c);
        setUrl(c.toDataURL("image/png"));
      })
      .catch(() => live && setError("Görsel oluşturulamadı."));
    return () => {
      live = false;
    };
  }, [open, rows, rangeLabel, scopeLabel, realSeconds, breakLimitMinutes, dayCount]);

  const file = `ekip-performansi-${rangeLabel.replace(/[^\d]+/g, "-").replace(/^-|-$/g, "") || "bugun"}.png`;

  const download = () => {
    if (!url) return;
    const a = document.createElement("a");
    a.href = url;
    a.download = file;
    a.click();
  };

  const copy = async () => {
    if (!canvas) return;
    try {
      const blob = await new Promise<Blob | null>((r) => canvas.toBlob(r, "image/png"));
      if (!blob) throw new Error();
      await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setError("Panoya kopyalanamadı, indirmeyi kullan.");
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Performans Görseli & Mola Durumu"
      description="Sıralama, çağrı rakamları ve mola uyum analizi. İndir ya da panoya kopyalayıp sohbete yapıştır."
      size="lg"
      footer={
        <>
          {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
          <Button variant="secondary" onClick={copy} disabled={!canvas}>
            {copied ? <Check className="size-4" /> : <Copy className="size-4" />}{" "}
            {copied ? "Kopyalandı" : "Panoya kopyala"}
          </Button>
          <Button onClick={download} disabled={!url}>
            <Download className="size-4" /> PNG indir
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {/* Dikkat çekici mola uyum & aşım durum kartı */}
        <div className="space-y-2.5">
          {exceededAgents.length > 0 ? (
            <div className="flex items-start gap-3 rounded-2xl border border-destructive/40 bg-destructive/10 p-3.5 text-xs text-destructive shadow-sm">
              <AlertTriangle className="mt-0.5 size-5 shrink-0 animate-pulse text-destructive" />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-bold text-destructive">
                    Mola Sınırı Aşıldı: {exceededAgents.length} Temsilci
                  </span>
                  <span className="rounded-full bg-destructive/20 px-2.5 py-0.5 text-[11px] font-bold">
                    Sınır: {formatSec(limitSeconds)}
                  </span>
                </div>
                <p className="mt-1 font-medium text-foreground/90">
                  {exceededAgents
                    .map(
                      (a) =>
                        `${a.name}: ${formatSec(a.breakSeconds)} (+${formatSec(a.breakSeconds - limitSeconds)} aşıldı)`,
                    )
                    .join(" · ")}
                </p>
              </div>
            </div>
          ) : (
            <div className="flex items-center gap-3 rounded-2xl border border-emerald-500/40 bg-emerald-500/10 p-3.5 text-xs text-emerald-600 shadow-sm dark:text-emerald-400">
              <CheckCircle2 className="size-5 shrink-0 text-emerald-500" />
              <div className="flex min-w-0 flex-1 flex-wrap items-center justify-between gap-2">
                <div>
                  <span className="text-sm font-bold">Mola Sınırına Tam Uyum</span>
                  <p className="mt-0.5 text-muted-foreground">
                    Tüm temsilciler {dayCount > 1 ? `${dayCount} günlük toplam ` : ""}
                    {formatSec(limitSeconds)} mola sınırına uydu. Aşım yapılmadı.
                  </p>
                </div>
                <span className="rounded-full bg-emerald-500/20 px-2.5 py-0.5 text-[11px] font-bold text-emerald-600 dark:text-emerald-400">
                  Aşım Yok
                </span>
              </div>
            </div>
          )}

          {/* 3 Ana Metrik Kartı */}
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <div className="rounded-xl border border-border/60 bg-muted/30 p-2.5">
              <div className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
                <Coffee className="size-3.5 text-amber-500" />
                Toplam Ekip Molası
              </div>
              <div className="mt-1 text-lg font-bold tabular-nums text-foreground">
                {formatSec(totalBreakSeconds)}
              </div>
              <div className="text-[10px] text-muted-foreground">
                {rows.filter((r) => (r.breakSeconds || 0) > 0).length} kişi mola kullandı
              </div>
            </div>

            <div className="rounded-xl border border-border/60 bg-muted/30 p-2.5">
              <div className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
                <Clock className="size-3.5 text-blue-500" />
                Kişi Başı Sınır
              </div>
              <div className="mt-1 text-lg font-bold tabular-nums text-foreground">
                {formatSec(limitSeconds)}
              </div>
              <div className="text-[10px] text-muted-foreground">
                {dayCount > 1 ? `Günde ${breakLimitMinutes} dk (${dayCount} gün)` : "Günlük mola hakkı"}
              </div>
            </div>

            <div
              className={cn(
                "rounded-xl border p-2.5",
                exceededAgents.length > 0
                  ? "border-destructive/40 bg-destructive/5"
                  : "border-emerald-500/40 bg-emerald-500/5",
              )}
            >
              <div className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground">
                {exceededAgents.length > 0 ? (
                  <AlertTriangle className="size-3.5 text-destructive" />
                ) : (
                  <CheckCircle2 className="size-3.5 text-emerald-500" />
                )}
                Mola Aşım Durumu
              </div>
              <div
                className={cn(
                  "mt-1 text-lg font-bold tabular-nums",
                  exceededAgents.length > 0 ? "text-destructive" : "text-emerald-500",
                )}
              >
                {exceededAgents.length > 0 ? `${exceededAgents.length} Kişi Aştı` : "Aşım Yok"}
              </div>
              <div className="text-[10px] text-muted-foreground">
                {exceededAgents.length > 0
                  ? `${withinLimitAgents.length + noBreakAgents.length} kişi sınır içinde`
                  : "Tüm ekip kurallara uydu"}
              </div>
            </div>
          </div>

          {/* Kişi bazlı mola dağılım rozetleri */}
          {rows.length > 0 && (
            <div className="flex max-h-24 flex-wrap items-center gap-1.5 overflow-y-auto rounded-xl border border-border/50 bg-background/50 p-2 text-xs">
              {rows.map((r) => {
                const bSec = r.breakSeconds || 0;
                const isExceeded = bSec > limitSeconds;
                return (
                  <div
                    key={r.userId}
                    className={cn(
                      "flex items-center gap-1.5 rounded-lg px-2 py-1 text-[11px] font-medium ring-1 transition-colors",
                      isExceeded
                        ? "bg-destructive/15 font-bold text-destructive ring-destructive/40"
                        : bSec > 0
                          ? "bg-muted/70 text-foreground ring-border/50"
                          : "bg-muted/30 text-muted-foreground opacity-70 ring-border/30",
                    )}
                  >
                    <span>{r.name}</span>
                    <span className="font-mono">({formatSec(bSec)})</span>
                    {isExceeded ? (
                      <span className="rounded bg-destructive px-1 py-0.5 text-[9.5px] text-destructive-foreground">
                        +{formatSec(bSec - limitSeconds)} aşıldı
                      </span>
                    ) : bSec > 0 ? (
                      <span className="text-[10px] text-emerald-500">uygun</span>
                    ) : null}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Görsel Önizleme */}
        {url ? (
          <img
            src={url}
            alt="Ekip performansı"
            className="w-full rounded-xl border border-border/60 shadow-sm"
          />
        ) : (
          <p className="py-10 text-center text-sm text-muted-foreground">
            {error ?? "Görsel hazırlanıyor..."}
          </p>
        )}
      </div>
    </Modal>
  );
}
