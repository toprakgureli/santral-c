// Mini games administration: the three switches and, per game, the
// content pool with add, edit, disable, delete and spreadsheet import.

import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowLeft, FileUp, Plus, Trash2 } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Badge, Button, Card, ConfirmDialog, EmptyState, Input } from "@/components/ui";
import { gamesApi } from "@/games/api";
import { ICONS } from "@/games/StartGameDialog";
import type { GameItem, GameMeta, GamesConfig } from "@/games/types";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";

export function GamesAdmin() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const allowed = can(user, "games.manage");
  const [config, setConfig] = useState<GamesConfig | null>(null);
  const [kind, setKind] = useState<GameMeta | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    gamesApi
      .config()
      .then((c) => {
        setConfig(c);
        setKind((k) => k ?? c.kinds.find((m) => m.itemKind) ?? null);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Ayarlar okunamadı."));
  }, []);
  useEffect(load, [load]);

  const flip = async (key: "enabled" | "pauseOnCall" | "breakOnly") => {
    if (!config) return;
    const next = { enabled: config.enabled, pauseOnCall: config.pauseOnCall, breakOnly: config.breakOnly, [key]: !config[key] };
    try {
      await gamesApi.updateSettings(next);
      setConfig({ ...config, ...next });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Ayar kaydedilemedi.");
    }
  };

  if (!allowed) return <EmptyState title="Yetki yok" description="Mini oyun yönetimi için games.manage yetkisi gerekir." />;

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-center gap-3">
        <button type="button" onClick={() => navigate(-1)} aria-label="Geri" className="flex size-9 items-center justify-center rounded-xl border border-border/70 bg-card text-muted-foreground shadow-sm hover:bg-accent hover:text-foreground"><ArrowLeft className="size-4" /></button>
        <div>
          <h1 className="text-lg font-semibold tracking-tight">Mini Oyunlar</h1>
          <p className="text-sm text-muted-foreground">Ayarlar ve oyun içerikleri. Kod değil, buradan düzenlenir.</p>
        </div>
      </div>

      {error && <p className="rounded-xl bg-destructive/10 px-4 py-2.5 text-sm text-destructive">{error}</p>}

      <Card title="Ayarlar">
        <div className="grid gap-2 sm:grid-cols-3">
          <Toggle label="Oyunlar açık" hint="Kapalıysa kimse oyun başlatamaz, açık oyunlar sürer." on={!!config?.enabled} onClick={() => void flip("enabled")} />
          <Toggle label="Çağrıda duraklat" hint="Oyuncuya çağrı gelince oyun herkes için durur, bitince devam eder." on={!!config?.pauseOnCall} onClick={() => void flip("pauseOnCall")} />
          <Toggle label="Sadece molada" hint="Oyun kurmak ve katılmak için mola durumunda olmak gerekir." on={!!config?.breakOnly} onClick={() => void flip("breakOnly")} />
        </div>
      </Card>

      <Card title="İçerikler" actions={config && kind ? <Badge tone={(config.itemCounts[kind.itemKind] ?? 0) >= kind.minItems ? "green" : "amber"}>{config.itemCounts[kind.itemKind] ?? 0} aktif · en az {kind.minItems}</Badge> : null}>
        <div className="flex flex-wrap gap-1.5">
          {config?.kinds.filter((m, i, all) => m.itemKind && all.findIndex((o) => o.itemKind === m.itemKind) === i).map((m) => (
            <button key={m.key} type="button" onClick={() => setKind(m)} className={cn("rounded-full px-3 py-1.5 text-xs font-medium", kind?.key === m.key ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:bg-accent")}>
              {ICONS[m.key]} {m.name} <span className="opacity-70">({config.itemCounts[m.itemKind] ?? 0})</span>
            </button>
          ))}
        </div>
        {kind && <Pool key={kind.key} meta={kind} onChanged={load} />}
        <p className="mt-4 text-xs text-muted-foreground">Kulaktan Kulağa Çizim, Çiz & Bil ile aynı kelime havuzunu kullanır. İçeriği olmayan oyunlar (Yalan mı Gerçek mi, Kim Söyledi, Bağlantı Dört, Masa Hokeyi) oyuncuların kendi yazdıklarıyla ya da odadaki mesajlarla oynanır.</p>
      </Card>
    </div>
  );
}

function Toggle({ label, hint, on, onClick }: { label: string; hint: string; on: boolean; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex items-start gap-3 rounded-xl border p-3.5 text-left transition-colors", on ? "border-success/50 bg-success/5" : "border-border/60 hover:bg-accent")}>
      <span className={cn("mt-0.5 flex h-5 w-9 shrink-0 items-center rounded-full p-0.5 transition-colors", on ? "bg-success" : "bg-muted-foreground/40")}>
        <span className={cn("size-4 rounded-full bg-white shadow transition-transform", on && "translate-x-4")} />
      </span>
      <span>
        <span className="block text-sm font-medium">{label}</span>
        <span className="block text-xs text-muted-foreground">{hint}</span>
      </span>
    </button>
  );
}

function Pool({ meta, onChanged }: { meta: GameMeta; onChanged: () => void }) {
  const [items, setItems] = useState<GameItem[] | null>(null);
  const [text, setText] = useState("");
  const [answer, setAnswer] = useState("");
  const [seconds, setSeconds] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<GameItem | null>(null);
  const [imported, setImported] = useState<string | null>(null);
  const picker = useRef<HTMLInputElement>(null);
  const hasAnswer = meta.key === "solve";

  const load = useCallback(() => {
    gamesApi.items(meta.itemKind).then(setItems).catch((e) => setError(e instanceof ApiError ? e.message : "Liste alınamadı."));
  }, [meta.itemKind]);
  useEffect(load, [load]);

  const add = async () => {
    if (!text.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await gamesApi.createItem(meta.itemKind, { text: text.trim(), answer: answer.trim(), seconds: Number(seconds) || 0 });
      setText("");
      setAnswer("");
      setSeconds("");
      load();
      onChanged();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Eklenemedi.");
    } finally {
      setBusy(false);
    }
  };

  const toggle = async (it: GameItem) => {
    try {
      await gamesApi.updateItem(it.id, { text: it.text, answer: it.answer, seconds: it.seconds, active: !it.active });
      load();
      onChanged();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Güncellenemedi.");
    }
  };

  const remove = async () => {
    if (!confirm) return;
    setBusy(true);
    try {
      await gamesApi.deleteItem(confirm.id);
      setConfirm(null);
      load();
      onChanged();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Silinemedi.");
    } finally {
      setBusy(false);
    }
  };

  const importFile = async (file: File | null) => {
    if (!file) return;
    setBusy(true);
    setError(null);
    setImported(null);
    try {
      const r = await gamesApi.importItems(meta.itemKind, file);
      setImported(`${r.added} ${meta.itemLabel.toLocaleLowerCase("tr")} eklendi.`);
      load();
      onChanged();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İçe aktarılamadı.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-4 space-y-4">
      <p className="rounded-xl bg-muted/30 px-3 py-2 text-xs text-muted-foreground">{meta.itemHint}</p>
      <div className="flex flex-wrap items-end gap-2">
        <label className="min-w-0 flex-1 space-y-1">
          <span className="text-xs text-muted-foreground">{meta.itemLabel}</span>
          <Input value={text} onChange={(e) => setText(e.target.value)} placeholder={meta.itemLabel} onKeyDown={(e) => e.key === "Enter" && void add()} />
        </label>
        {hasAnswer && (
          <label className="min-w-0 flex-1 space-y-1">
            <span className="text-xs text-muted-foreground">Örnek çözüm (isteğe bağlı)</span>
            <Input value={answer} onChange={(e) => setAnswer(e.target.value)} placeholder="Sonuçta gösterilir" />
          </label>
        )}
        {meta.secondsLabel && (
          <label className="w-24 space-y-1">
            <span className="text-xs text-muted-foreground">Süre (sn)</span>
            <Input type="number" min={0} max={600} value={seconds} onChange={(e) => setSeconds(e.target.value)} placeholder="oyun" />
          </label>
        )}
        <Button onClick={() => void add()} disabled={busy || !text.trim()} className="h-10"><Plus /> Ekle</Button>
        <Button variant="secondary" onClick={() => picker.current?.click()} disabled={busy} className="h-10"><FileUp /> Excel / CSV</Button>
        <input ref={picker} type="file" accept=".xlsx,.csv" className="hidden" onChange={(e) => { void importFile(e.target.files?.[0] ?? null); e.target.value = ""; }} />
      </div>
      <p className="text-[0.65rem] text-muted-foreground/70">Dosya sütunları: A metin, B cevap, C süre (sn), D seçenekler (| ile ayrılmış). İlk satır başlıksa atlanır.</p>
      {imported && <p className="text-xs text-success">{imported}</p>}
      {error && <p className="text-xs text-destructive">{error}</p>}

      {items === null ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : items.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted-foreground">Henüz içerik yok. Yukarıdan ekle ya da bir Excel yükle.</p>
      ) : (
        <ul className="divide-y divide-border/60 rounded-xl border border-border/60">
          {items.map((it) => (
            <li key={it.id} className={cn("flex items-center gap-3 px-3 py-2 text-sm", !it.active && "opacity-50")}>
              <span className="min-w-0 flex-1">
                <span className={cn("block truncate", !it.active && "line-through")}>{it.text}</span>
                {it.answer && <span className="block truncate text-xs text-muted-foreground">{it.answer}</span>}
              </span>
              {it.seconds > 0 && <span className="rounded-full bg-muted px-2 text-[0.65rem] tabular-nums text-muted-foreground">{it.seconds} sn</span>}
              <button type="button" onClick={() => void toggle(it)} className="text-xs text-muted-foreground hover:text-foreground">{it.active ? "Pasifleştir" : "Aktifleştir"}</button>
              <button type="button" onClick={() => setConfirm(it)} aria-label="Sil" className="rounded-lg p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-4" /></button>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog open={!!confirm} title="İçeriği sil" description={<>"{confirm?.text}" silinecek.</>} confirmLabel="Evet, sil" busy={busy} onConfirm={() => void remove()} onCancel={() => setConfirm(null)} />
    </div>
  );
}
