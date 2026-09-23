// System settings. The MFA requirement card needs system.settings; login
// attempts and IP bans need system.logs.

import { useCallback, useEffect, useState } from "react";
import { Coffee, HardDrive, ScrollText, ShieldBan, ShieldCheck, TriangleAlert } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { DriveStatus, IPBan, LoginAttempt, Paged } from "../api/types";
import { formatSize } from "../lib/attachments";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, EmptyState, Input, Pagination, Skeleton } from "../components/ui";
import { cn, formatDateTime } from "../lib/utils";

const REASONS: Record<string, string> = {
  bad_credentials: "Hatalı bilgi",
  inactive: "Pasif hesap",
  locked: "Kilitli",
  banned_ip: "Banlı IP",
  mfa_required: "Kod bekleniyor",
};

const PER_PAGE = 25;

export function Settings() {
  const { user } = useAuth();
  const canSeeLogs = can(user, "system.logs");
  const canManage = can(user, "system.settings");
  const canBreakLimit = can(user, "agent.break_limit");
  const canDrive = can(user, "teams.admin");

  const [attempts, setAttempts] = useState<Paged<LoginAttempt> | null>(null);
  const [bans, setBans] = useState<IPBan[]>([]);
  const [page, setPage] = useState(1);
  const [onlyFailed, setOnlyFailed] = useState(false);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!canSeeLogs) {
      setLoading(false);
      return;
    }
    setLoading(true);
    const [a, b] = await Promise.all([
      api.securityAttempts({ page, perPage: PER_PAGE, success: onlyFailed ? "false" : undefined }).catch(() => null),
      api.securityBans().catch(() => [] as IPBan[]),
    ]);
    setAttempts(a);
    setBans(b);
    setLoading(false);
  }, [canSeeLogs, page, onlyFailed]);

  useEffect(() => {
    void load();
  }, [load]);

  const unban = async (id: number) => {
    await api.removeBan(id).catch(() => undefined);
    void load();
  };

  return (
    <div className="space-y-6">
      {canManage && <MfaRequiredCard />}
      {canBreakLimit && <BreakLimitCard />}
      {canDrive && <DriveCard />}

      {canSeeLogs && (
        <>
          <Card title="Banlı IP Adresleri" actions={<span className="text-xs text-muted-foreground">{bans.length} aktif</span>}>
            {!bans.length ? (
              <EmptyState icon={<ShieldBan />} title="Banlı IP yok" />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[40rem] text-sm">
                  <thead>
                    <tr className="text-left text-xs text-muted-foreground">
                      <th className="pb-2">IP</th>
                      <th className="pb-2">Sebep</th>
                      <th className="pb-2">Ban Sayısı</th>
                      <th className="pb-2">Bitiş</th>
                      <th className="pb-2"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {bans.map((b) => (
                      <tr key={b.id} className="border-t border-border/60">
                        <td className="py-2.5 font-medium">{b.ip}</td>
                        <td className="py-2.5 text-muted-foreground">{b.reason}</td>
                        <td className="py-2.5 tabular-nums">{b.attempts}</td>
                        <td className="py-2.5 whitespace-nowrap text-muted-foreground">{formatDateTime(b.until)}</td>
                        <td className="py-2.5 text-right">
                          <Button variant="secondary" className="h-8 px-3 text-xs" onClick={() => unban(b.id)}>Kaldır</Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>

          <Card
            title="Giriş Kayıtları"
            actions={
              <Button
                variant="secondary"
                className={cn("h-8 px-3 text-xs", onlyFailed && "border-primary bg-primary text-primary-foreground hover:bg-primary/90")}
                onClick={() => { setOnlyFailed((v) => !v); setPage(1); }}
              >
                Sadece başarısız
              </Button>
            }
          >
            {loading ? (
              <div className="space-y-2">
                {[0, 1, 2, 3].map((row) => (
                  <Skeleton key={row} className="h-12 w-full rounded-xl" />
                ))}
              </div>
            ) : !attempts?.items.length ? (
              <EmptyState icon={<ScrollText />} title="Kayıt bulunamadı" />
            ) : (
              <>
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[44rem] text-sm">
                    <thead>
                      <tr className="text-left text-xs text-muted-foreground">
                        <th className="pb-2">Zaman</th>
                        <th className="pb-2">E-posta</th>
                        <th className="pb-2">IP</th>
                        <th className="pb-2">Sonuç</th>
                      </tr>
                    </thead>
                    <tbody>
                      {attempts.items.map((a) => (
                        <tr key={a.id} className="border-t border-border/60">
                          <td className="py-2.5 whitespace-nowrap text-muted-foreground">{formatDateTime(a.createdAt)}</td>
                          <td className="py-2.5">
                            <span className="block font-medium">{a.email}</span>
                            <span className="block max-w-64 truncate text-xs text-muted-foreground">{a.userAgent}</span>
                          </td>
                          <td className="py-2.5 tabular-nums">{a.ip}</td>
                          <td className="py-2.5">
                            <Badge tone={a.success ? "green" : "red"}>{a.success ? "Başarılı" : (REASONS[a.reason] ?? "Başarısız")}</Badge>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <Pagination page={attempts.page} perPage={attempts.perPage} total={attempts.total} onChange={setPage} />
              </>
            )}
          </Card>
        </>
      )}

      {!canManage && !canSeeLogs && (
        <Card title="Sistem Ayarları">
          <EmptyState title="Bu sayfa için yetkin yok" />
        </Card>
      )}
    </div>
  );
}

const BREAK_PRESETS = [30, 45, 60, 90, 120];

// BreakLimitCard sets the daily break allowance. Past it the break card turns
// red and counts the excess; the number is read by every agent's panel.
function BreakLimitCard() {
  const [saved, setSaved] = useState<number | null>(null);
  const [minutes, setMinutes] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .breakLimit()
      .then((r) => {
        if (!alive) return;
        setSaved(r.minutes);
        setMinutes(String(r.minutes));
      })
      .catch((e) => {
        if (!alive) return;
        setError(e instanceof ApiError ? e.message : "Ayar okunamadı.");
        setSaved(60);
        setMinutes("60");
      });
    return () => { alive = false; };
  }, []);

  const value = Number(minutes);
  const valid = Number.isInteger(value) && value >= 5 && value <= 720;
  const dirty = saved !== null && valid && value !== saved;

  const save = async () => {
    if (!valid) return;
    setBusy(true);
    setError(null);
    try {
      const r = await api.updateBreakLimit(value);
      setSaved(r.minutes);
      setMinutes(String(r.minutes));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Ayar kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };

  const label = (m: number) => (m % 60 === 0 ? `${m / 60} saat` : m > 60 ? `${Math.floor(m / 60)} sa ${m % 60} dk` : `${m} dk`);

  return (
    <Card
      title="Günlük Mola Sınırı"
      actions={saved === null ? <Skeleton className="h-5 w-16" /> : <Badge tone="amber">{label(saved)}</Badge>}
    >
      <div className="space-y-4">
        <div className="flex items-start gap-3 rounded-xl border border-border/60 p-3.5">
          <Coffee className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div className="space-y-1">
            <p className="text-sm font-medium">Bir temsilcinin gün içinde toplam ne kadar mola kullanabileceği</p>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Sınır dolduğunda mola kartının çerçevesi kırmızıya döner ve sayaç aşılan süreyi eksi olarak saymaya başlar.
              Mola bitirilmez, yalnızca görünür hale gelir. Değişiklik bir sonraki okumada tüm panellere yansır.
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {BREAK_PRESETS.map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMinutes(String(m))}
              className={cn(
                "h-9 rounded-xl border px-3 text-sm transition-colors",
                value === m ? "border-primary bg-primary/10 text-primary" : "border-border/70 text-muted-foreground hover:border-border hover:bg-accent",
              )}
            >
              {label(m)}
            </button>
          ))}
          <div className="flex items-center gap-2">
            <Input
              type="number"
              min={5}
              max={720}
              value={minutes}
              onChange={(e) => setMinutes(e.target.value)}
              className="h-9 w-24 font-mono tabular-nums"
              title="Dakika (5 ile 720 arası)"
            />
            <span className="text-sm text-muted-foreground">dakika</span>
          </div>
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}

        {saved !== null && (
          <Button onClick={save} disabled={busy || !dirty}>
            {busy ? "Kaydediliyor..." : "Kaydet"}
          </Button>
        )}
      </div>
    </Card>
  );
}

// MfaRequiredCard toggles forced TOTP enrollment at login. The server enforces
// the rule; the card only reflects and changes it.
function MfaRequiredCard() {
  const [required, setRequired] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .systemSettings()
      .then((s) => { if (alive) setRequired(Boolean(s.mfaRequired)); })
      .catch((e) => {
        if (!alive) return;
        setError(e instanceof ApiError ? e.message : "Ayarlar okunamadı.");
        setRequired(false);
      });
    return () => { alive = false; };
  }, []);

  const toggle = async () => {
    const next = !required;
    setBusy(true);
    setError(null);
    try {
      await api.updateSystemSettings({ mfaRequired: next });
      setRequired(next);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Ayar kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card
      title="İki Adımlı Doğrulama Zorunluluğu"
      actions={required === null ? <Skeleton className="h-5 w-16" /> : <Badge tone={required ? "green" : "slate"}>{required ? "Açık" : "Kapalı"}</Badge>}
    >
      <div className="space-y-4">
        <div className="flex items-start gap-3 rounded-xl border border-border/60 p-3.5">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div className="space-y-1">
            <p className="text-sm font-medium">Kullanıcılar doğrulama uygulaması kurmadan giriş yapamasın</p>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Açıkken, TOTP kurulumu olmayan herkes bir sonraki girişinde kurulum ekranına yönlendirilir ve kurulumu tamamlamadan oturum açamaz.
              Muaf işaretli hesaplar bu kuraldan etkilenmez.
            </p>
          </div>
        </div>

        {required && (
          <div className="flex items-start gap-2 rounded-xl border border-warning/30 bg-warning/10 px-3 py-2.5 text-xs leading-relaxed text-warning">
            <TriangleAlert className="mt-0.5 size-4 shrink-0" />
            <span>Doğrulama uygulamasına erişimini kaybeden kullanıcıyı yalnızca bir yönetici (MFA sıfırlayarak) kurtarabilir.</span>
          </div>
        )}

        {error && <p className="text-xs text-destructive">{error}</p>}

        {required !== null && (
          <Button variant={required ? "danger" : "primary"} onClick={toggle} disabled={busy}>
            {busy ? "Kaydediliyor..." : required ? "Zorunluluğu kaldır" : "Zorunlu yap"}
          </Button>
        )}
      </div>
    </Card>
  );
}

// DriveCard links the Google account whose Drive keeps the chat files.
// The OAuth client lives in config.yml; the account is connected here
// once and the refresh token is kept encrypted on the server.
function DriveCard() {
  const [status, setStatus] = useState<DriveStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(() => {
    const q = new URLSearchParams(window.location.search);
    const r = q.get("drive");
    if (r === "ok") return "Google Drive bağlandı.";
    if (r === "error") return `Bağlantı kurulamadı: ${q.get("reason") ?? "bilinmeyen hata"}`;
    return null;
  });

  const load = useCallback(() => {
    api
      .driveStatus()
      .then(setStatus)
      .catch((e) => setError(e instanceof ApiError ? e.message : "Durum okunamadı."));
  }, []);

  useEffect(() => {
    load();
    if (window.location.search.includes("drive=")) window.history.replaceState(null, "", window.location.pathname);
  }, [load]);

  const disconnect = async () => {
    setBusy(true);
    setError(null);
    try {
      await api.driveDisconnect();
      setNotice("Bağlantı kesildi. Eski dosyalar Drive'da kalır, yenileri yüklenemez.");
      load();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Bağlantı kesilemedi.");
    } finally {
      setBusy(false);
    }
  };

  const pct = status && status.limit > 0 ? Math.min(100, Math.round((status.usage / status.limit) * 100)) : 0;

  return (
    <Card
      title="Teams Dosya Depolama (Google Drive)"
      actions={
        status === null ? <Skeleton className="h-5 w-20" /> : status.connected ? <Badge tone="green">Bağlı</Badge> : status.configured ? <Badge tone="amber">Bağlı değil</Badge> : <Badge tone="red">Yapılandırılmamış</Badge>
      }
    >
      <div className="space-y-4">
        <div className="flex items-start gap-3 rounded-xl border border-border/60 p-3.5">
          <HardDrive className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div className="space-y-1">
            <p className="text-sm font-medium">Sohbette paylaşılan görsel, video ve dosyalar bu Google hesabının Drive'ında saklanır</p>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Dosyalar sunucuya uğramadan doğrudan Drive'a yüklenir, indirilirken sunucu üzerinden yalnızca odadaki kişilere akar.
              Hesabın kendi Drive kotası kullanılır. Bağlantı bir kez kurulur; kesilirse eski dosyalar Drive'da kalır ama yeni yükleme yapılamaz.
            </p>
          </div>
        </div>

        {status && !status.configured && (
          <p className="rounded-xl border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
            config.yml içinde <code className="font-mono">drive.clientId</code>, <code className="font-mono">drive.clientSecret</code> ve <code className="font-mono">drive.redirectUrl</code> tanımlı değil.
          </p>
        )}

        {status?.connected && (
          <div className="space-y-2 rounded-xl border border-border/60 bg-muted/30 p-3.5 text-sm">
            <p><span className="text-muted-foreground">Hesap:</span> <span className="font-medium">{status.account || "bilinmiyor"}</span></p>
            <p><span className="text-muted-foreground">Klasör:</span> <span className="font-mono text-xs">{status.folder}</span> <span className="text-xs text-muted-foreground">(Drive kök dizininde)</span></p>
            {status.limit > 0 ? (
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">Kota: {formatSize(status.usage)} / {formatSize(status.limit)} (%{pct})</p>
                <span className="block h-1.5 w-full overflow-hidden rounded-full bg-muted">
                  <span className={cn("block h-full rounded-full", pct > 90 ? "bg-destructive" : "bg-primary")} style={{ width: `${pct}%` }} />
                </span>
              </div>
            ) : status.error ? (
              <p className="text-xs text-destructive">Kota okunamadı: {status.error}</p>
            ) : null}
          </div>
        )}

        {notice && <p className="text-xs text-success">{notice}</p>}
        {error && <p className="text-xs text-destructive">{error}</p>}

        <div className="flex flex-wrap gap-2">
          {status?.configured && (
            <Button
              onClick={() => {
                window.location.href = "/api/v1/teams/drive/connect";
              }}
              disabled={busy}
              className="h-9"
            >
              {status.connected ? "Hesabı değiştir" : "Google Drive'ı bağla"}
            </Button>
          )}
          {status?.connected && (
            <Button variant="secondary" onClick={() => void disconnect()} disabled={busy} className="h-9">Bağlantıyı kes</Button>
          )}
        </div>
      </div>
    </Card>
  );
}
