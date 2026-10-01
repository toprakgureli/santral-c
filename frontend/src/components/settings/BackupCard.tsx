// BackupCard sets up the database copies that go to a Google Shared Drive
// every six hours. The service account is only a Contributor in that drive,
// so it can add files and never delete them; the server checks this before
// every copy and refuses when it could delete. Each copy is then locked
// read-only, so it cannot be overwritten either.

import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, CircleAlert, DatabaseBackup, KeyRound, ListChecks } from "lucide-react";
import { api, ApiError, type BackupCheck, type BackupView } from "@/api/client";
import { Badge, Button, Card, EmptyState, Input, Skeleton } from "@/components/ui";
import { Switch } from "@/components/whatsapp/settings/parts";
import { formatSize } from "@/lib/attachments";
import { formatDateTime } from "@/lib/utils";

export default function BackupCard() {
  const [view, setView] = useState<BackupView | null>(null);
  const [folder, setFolder] = useState("");
  const [key, setKey] = useState("");
  const [changeKey, setChangeKey] = useState(false);
  const [enabled, setEnabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [check, setCheck] = useState<BackupCheck | null>(null);
  const [steps, setSteps] = useState(false);

  const apply = useCallback((v: BackupView) => {
    setView(v);
    setFolder(v.folderId);
    setEnabled(v.enabled);
  }, []);

  useEffect(() => {
    api.backupSettings().then(apply).catch((e) => setError(e instanceof ApiError ? e.message : "Yedekleme ayarları okunamadı."));
  }, [apply]);

  // While a copy runs, the list is refreshed until it ends.
  useEffect(() => {
    if (!view?.running) return;
    const t = window.setInterval(() => {
      api.backupSettings().then((v) => setView(v)).catch(() => undefined);
    }, 4000);
    return () => window.clearInterval(t);
  }, [view?.running]);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem yapılamadı.");
    } finally {
      setBusy(false);
    }
  };

  const save = () =>
    run(async () => {
      const v = await api.saveBackup({ enabled, folderId: folder, credentials: changeKey || !view?.hasKey ? key : "" });
      apply(v);
      setKey("");
      setChangeKey(false);
      setNotice("Kaydedildi.");
    });

  const verify = () =>
    run(async () => {
      setCheck(await api.checkBackup());
    });

  const now = () =>
    run(async () => {
      setView(await api.runBackup());
      setNotice("Yedekleme başladı; bitince aşağıdaki listede görünür.");
    });

  const last = view?.runs.find((r) => r.finishedAt);
  const badge =
    view === null ? <Skeleton className="h-5 w-16" /> : view.running ? <Badge tone="blue">Yedekleniyor</Badge> : !view.enabled ? <Badge tone="slate">Kapalı</Badge> : last && !last.ok ? <Badge tone="red">Son yedek başarısız</Badge> : <Badge tone="green">Açık</Badge>;

  return (
    <Card title="Veritabanı Yedeği" icon={DatabaseBackup} actions={badge}>
      <div className="space-y-4">
        <div className="flex items-start gap-3 rounded-xl border border-border/60 p-3.5">
          <DatabaseBackup className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div className="space-y-1">
            <p className="text-sm font-medium">Bütün veritabanı her {view?.everyHours ?? 6} saatte bir Google Ortak Drive'a kopyalanır</p>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Yedekler, sunucunun yüklediği ama silemediği bir klasöre gider: servis hesabı Ortak Drive'da yalnızca "Katkıda bulunan" olur. Sunucu
              ele geçirilse bile eski yedekler silinemez. Her yedekten önce bu yetki denetlenir; silme yetkisi görülürse yedek alınmaz. Yüklenen her
              yedek ayrıca kilitlenir: üzerine yazılamaz, eski hali silinemez. Kilidi sadece Ortak Drive yöneticisi kaldırabilir.
              Geri yüklemek için bir dosyayı indirip <code className="font-mono">pg_restore</code> ile açmak yeterlidir.
            </p>
            <button type="button" onClick={() => setSteps((v) => !v)} className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline">
              <ListChecks className="size-3.5" /> {steps ? "Kurulum adımlarını gizle" : "Kurulum adımları"}
            </button>
          </div>
        </div>

        {steps && (
          <ol className="list-decimal space-y-1.5 rounded-xl bg-muted/40 p-3.5 pl-8 text-xs leading-relaxed text-muted-foreground">
            <li>Google Cloud Console'da bir proje aç, Google Drive API'yi etkinleştir.</li>
            <li>IAM bölümünde bir servis hesabı oluştur ve JSON anahtarını indir.</li>
            <li>Google Workspace'te bir Ortak Drive aç; servis hesabının e-posta adresini bu Ortak Drive'a <b>"Katkıda bulunan"</b> rolüyle ekle (İçerik yöneticisi ya da Yönetici değil).</li>
            <li>Ortak Drive'da bir klasör aç ve adresini aşağıya yapıştır.</li>
            <li>
              JSON anahtarının içeriğini aşağıya yapıştırıp kaydet, sonra "Bağlantıyı denetle" de. Denetim klasöre küçük bir deneme dosyası ekleyip
              kilitler; bu dosya klasörde kalır. Her şey yeşilse yedeklemeyi aç.
            </li>
          </ol>
        )}

        {view && !view.pgDumpExists && (
          <p className="flex items-start gap-2 rounded-xl border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
            <CircleAlert className="mt-0.5 size-4 shrink-0" />
            Sunucuda pg_dump bulunamadı; yedek alınamaz. Sunucuya PostgreSQL istemcisini kur (postgresql-client).
          </p>
        )}

        <div className="grid gap-3 md:grid-cols-2">
          <label className="space-y-1.5">
            <span className="text-sm font-medium">Ortak Drive klasörü</span>
            <Input value={folder} onChange={(e) => setFolder(e.target.value)} placeholder="Klasörün adresi ya da kimliği" disabled={busy} />
          </label>
          <div className="space-y-1.5">
            <span className="text-sm font-medium">Servis hesabı anahtarı</span>
            {view?.hasKey && !changeKey ? (
              <div className="flex h-10 items-center gap-2 rounded-xl border border-border/60 px-3 text-sm">
                <KeyRound className="size-4 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{view.account || "Kayıtlı"}</span>
                <button type="button" className="text-xs font-medium text-primary hover:underline" onClick={() => setChangeKey(true)}>Değiştir</button>
              </div>
            ) : (
              <textarea
                value={key}
                onChange={(e) => setKey(e.target.value)}
                rows={3}
                spellCheck={false}
                placeholder='İndirdiğin JSON dosyasının içeriği: { "type": "service_account", ... }'
                className="w-full rounded-xl border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus:ring-2 focus:ring-ring/30"
                disabled={busy}
              />
            )}
          </div>
        </div>

        <div className="flex items-center justify-between gap-3 rounded-xl border border-border/60 px-3.5 py-2.5">
          <div>
            <p className="text-sm font-medium">Otomatik yedekleme</p>
            <p className="text-xs text-muted-foreground">Açıkken son yedek {view?.everyHours ?? 6} saatten eskiyse yenisi alınır.</p>
          </div>
          <Switch on={enabled} onChange={setEnabled} disabled={busy} label="Otomatik yedekleme" />
        </div>

        {check && (
          <div className={`space-y-1 rounded-xl p-3 text-xs ${check.problem ? "bg-destructive/10 text-destructive" : "bg-success/10 text-success"}`}>
            <p className="flex items-center gap-1.5 font-medium">
              {check.problem ? <CircleAlert className="size-4" /> : <CheckCircle2 className="size-4" />}
              {check.problem ? check.problem : `Hazır: "${check.folder.name}" klasörüne dosya eklenebilir; eklenen dosya silinemez ve kilitlenir.`}
            </p>
            <p className="text-muted-foreground">
              Ortak Drive: {check.folder.sharedDrive ? "evet" : "hayır"} · Ekleyebilir: {check.folder.canAdd ? "evet" : "hayır"} · Silebilir: {check.folder.canDelete ? "evet" : "hayır"}
              {check.folder.canLock !== undefined && <> · Kilitleyebilir: {check.folder.canLock ? "evet" : "hayır"}</>}
            </p>
          </div>
        )}
        {notice && <p className="text-xs text-success">{notice}</p>}
        {error && <p className="text-xs text-destructive">{error}</p>}

        <div className="flex flex-wrap gap-2">
          <Button onClick={() => void save()} disabled={busy} className="h-9">Kaydet</Button>
          <Button variant="secondary" onClick={() => void verify()} disabled={busy || !view?.hasKey} className="h-9">Bağlantıyı denetle</Button>
          <Button variant="secondary" onClick={() => void now()} disabled={busy || !view?.hasKey || view.running || !view.pgDumpExists} className="h-9">
            {view?.running ? "Yedekleniyor..." : "Şimdi yedekle"}
          </Button>
        </div>

        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">Son yedekler</p>
          {!view ? (
            <Skeleton className="h-16 w-full" />
          ) : view.runs.length === 0 ? (
            <EmptyState icon={<DatabaseBackup />} title="Henüz yedek alınmadı" />
          ) : (
            <ul className="divide-y divide-border/60 rounded-xl border border-border/60">
              {view.runs.map((r) => (
                <li key={r.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-xs">
                  <span className="tabular-nums text-muted-foreground">{formatDateTime(r.startedAt)}</span>
                  <span className="min-w-0 flex-1 truncate font-mono">{r.file || "(dosya yok)"}</span>
                  {r.size > 0 && <span className="tabular-nums text-muted-foreground">{formatSize(r.size)}</span>}
                  <span className="text-muted-foreground">{r.manual ? "elle" : "otomatik"}</span>
                  {!r.finishedAt ? <Badge tone="blue">Sürüyor</Badge> : r.ok ? <Badge tone="green">Tamam</Badge> : <Badge tone="red">Olmadı</Badge>}
                  {r.error && <span className="w-full text-destructive">{r.error}</span>}
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </Card>
  );
}
