// RatingLinks: the time-limited links that open the ratings without signing
// in. Whoever holds whatsapp.rating_link makes one for someone outside the
// panel, copies it, and can cancel any link at once. Whoever opens a link
// sees the ratings and can filter them, with customers' names cut to the
// first name and numbers to their last four digits.

import { useEffect, useState } from "react";
import { Info, Link2, Loader2, Plus, Slash } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, ConfirmDialog, Field, Input, Modal, Notice, Select } from "@/components/ui";
import CopyButton from "@/components/ui/CopyButton";
import { clockTime, shortMonthDate } from "@/lib/time";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WARatingLink } from "@/whatsapp/types";

const DURATIONS = [
  { hours: 1, label: "1 saat" },
  { hours: 24, label: "1 gün" },
  { hours: 72, label: "3 gün" },
  { hours: 168, label: "7 gün" },
  { hours: 720, label: "30 gün" },
];

// sharedUrl is the address a link opens.
export function sharedUrl(token: string): string {
  return `${window.location.origin}/shared/ratings/${token}`;
}

function at(iso: string): string {
  return `${shortMonthDate(iso)} ${clockTime(iso)}`;
}

export default function RatingLinks({ onClose }: { onClose: () => void }) {
  const [links, setLinks] = useState<WARatingLink[] | null>(null);
  const [label, setLabel] = useState("");
  const [hours, setHours] = useState(24);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fresh, setFresh] = useState<number | null>(null);
  const [cancel, setCancel] = useState<WARatingLink | null>(null);
  const [cancelling, setCancelling] = useState(false);
  const [cancelError, setCancelError] = useState<string | null>(null);

  const load = () => waApi.ratingLinks().then(setLinks).catch((e) => setError(e instanceof ApiError ? e.message : "Linkler alınamadı."));
  useEffect(() => {
    void load();
  }, []);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const l = await waApi.createRatingLink(label.trim(), hours);
      setLinks((cur) => [l, ...(cur ?? [])]);
      setFresh(l.id);
      setLabel("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Link oluşturulamadı.");
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    if (!cancel) return;
    setCancelling(true);
    setCancelError(null);
    try {
      await waApi.revokeRatingLink(cancel.id);
      setCancel(null);
      await load();
    } catch (err) {
      setCancelError(err instanceof ApiError ? err.message : "Link iptal edilemedi.");
    } finally {
      setCancelling(false);
    }
  };

  const active = (links ?? []).filter((l) => l.active);
  const ended = (links ?? []).filter((l) => !l.active);

  return (
    <Modal open onClose={onClose} size="lg" title="Paylaşım linkleri" description="Puanlamaları giriş yapmadan açan, süresi dolunca kendiliğinden kapanan linkler.">
      <div className="space-y-5">
        <Notice icon={<Info />}>
          Linki alan herkes, süresi bitene kadar puanlamaları görür ve filtreler; hiçbir şeyi değiştiremez. Müşteri adları kısaltılır (Zeynep A.), numaraların yalnızca son dört hanesi görünür, sohbetlere gidilemez. Linki kimin oluşturduğu ve iptal ettiği denetim kaydına yazılır.
        </Notice>

        <form onSubmit={create} className="grid gap-3 rounded-2xl bg-muted/30 p-4 ring-1 ring-border/50 sm:grid-cols-[1fr_9rem_auto] sm:items-end">
          <Field label="Kime ya da ne için">
            <Input value={label} maxLength={120} onChange={(e) => { setLabel(e.target.value); setError(null); }} placeholder="Örn: Bölge müdürü, ekim değerlendirmesi" />
          </Field>
          <Field label="Süre">
            <Select value={hours} onChange={(e) => setHours(Number(e.target.value))}>
              {DURATIONS.map((d) => <option key={d.hours} value={d.hours}>{d.label}</option>)}
            </Select>
          </Field>
          <Button type="submit" disabled={busy || label.trim().length < 2}>
            {busy ? <Loader2 className="animate-spin" /> : <Plus />} Link oluştur
          </Button>
        </form>
        {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}

        <section className="space-y-2">
          <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Çalışan linkler</h3>
          {links === null ? (
            <p className="text-sm text-muted-foreground">Yükleniyor...</p>
          ) : active.length === 0 ? (
            <p className="rounded-xl bg-muted/30 px-4 py-5 text-center text-sm text-muted-foreground">Çalışan link yok.</p>
          ) : (
            active.map((l) => (
              <div key={l.id} className={cn("space-y-2.5 rounded-2xl bg-card p-3.5 ring-1", fresh === l.id ? "ring-primary/50" : "ring-border/60")}>
                <div className="flex flex-wrap items-start gap-2">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary"><Link2 className="size-4" /></span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold">{l.label}</p>
                    <p className="text-xs text-muted-foreground">{l.createdBy.name} oluşturdu, {at(l.createdAt)} · <b className="font-medium text-foreground">{at(l.expiresAt)}</b> tarihine kadar geçerli</p>
                    <p className="text-xs text-muted-foreground">{l.openCount > 0 ? `${l.openCount} kez açıldı, en son ${at(l.lastOpenedAt!)}` : "Henüz açılmadı"}</p>
                  </div>
                </div>
                {l.token && (
                  <div className="flex flex-wrap items-center gap-2">
                    <code className="min-w-0 flex-1 truncate rounded-lg bg-muted/60 px-2.5 py-2 font-mono text-xs text-muted-foreground">{sharedUrl(l.token)}</code>
                    <CopyButton text={sharedUrl(l.token)} label="Linki kopyala" variant={fresh === l.id ? "primary" : "secondary"} />
                    <Button variant="ghost" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => { setCancelError(null); setCancel(l); }}>
                      <Slash /> İptal et
                    </Button>
                  </div>
                )}
              </div>
            ))
          )}
        </section>

        {ended.length > 0 && (
          <section className="space-y-2">
            <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Son 30 günde kapananlar</h3>
            <div className="divide-y divide-border/50 rounded-2xl bg-muted/20 px-3.5 ring-1 ring-border/40">
              {ended.map((l) => (
                <div key={l.id} className="py-2.5 text-sm">
                  <p className="truncate font-medium text-muted-foreground">{l.label}</p>
                  <p className="text-xs text-muted-foreground">
                    {l.revokedAt ? `${l.revokedBy?.name ?? "Biri"} iptal etti, ${at(l.revokedAt)}` : `Süresi doldu, ${at(l.expiresAt)}`} · {l.openCount} kez açıldı · {l.createdBy.name} oluşturmuştu
                  </p>
                </div>
              ))}
            </div>
          </section>
        )}
      </div>

      <ConfirmDialog
        open={cancel !== null}
        title="Link iptal edilsin mi?"
        description={`"${cancel?.label ?? ""}" linki hemen çalışmaz olur; açık olan sayfa da bir sonraki yenilemede kapanır. Gerekirse yeni bir link oluşturabilirsin.`}
        confirmLabel="İptal et"
        cancelLabel="Vazgeç"
        busy={cancelling}
        error={cancelError}
        onConfirm={() => void revoke()}
        onCancel={() => setCancel(null)}
      />
    </Modal>
  );
}
