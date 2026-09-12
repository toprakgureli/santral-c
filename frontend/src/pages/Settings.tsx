// System settings. The MFA requirement card needs system.settings; login
// attempts and IP bans need system.logs.

import { useCallback, useEffect, useState } from "react";
import { ScrollText, ShieldBan, ShieldCheck, TriangleAlert } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { IPBan, LoginAttempt, Paged } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, EmptyState, Pagination, Skeleton } from "../components/ui";
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
