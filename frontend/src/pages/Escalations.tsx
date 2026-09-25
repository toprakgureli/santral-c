import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronUp, Plus, Tags, Trash2, Upload } from "lucide-react";
import { IconChip } from "../components/ui/rows";
import { cn } from "../lib/utils";
import { api, ApiError } from "../api/client";
import type { EscalationCategory } from "../api/types";
import { Button, Card, ErrorText, Input } from "../components/ui";

export function Escalations() {
  const [categories, setCategories] = useState<EscalationCategory[]>([]);
  const [newCat, setNewCat] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [importInfo, setImportInfo] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  function load() {
    api.escalationCategories().then(setCategories).catch(() => setCategories([]));
  }
  useEffect(load, []);

  // Moving a category one step changes the order the agents see everywhere
  // (dashboard, wrap-up card). Optimistic; the server order is re-read after.
  async function moveCategory(index: number, dir: -1 | 1) {
    const to = index + dir;
    if (to < 0 || to >= categories.length) return;
    const next = [...categories];
    [next[index], next[to]] = [next[to], next[index]];
    setCategories(next);
    try {
      await api.reorderEscalationCategories(next.map((c) => c.id));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Sıralama kaydedilemedi.");
    }
    load();
  }

  async function addCategory(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const name = newCat.trim();
    if (!name) return;
    try {
      await api.createEscalationCategory(name);
      setNewCat("");
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Eklenemedi.");
    }
  }

  async function onImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setError(null);
    setImportInfo(null);
    try {
      const res = await api.importEscalationCatalog(file);
      setImportInfo(`${res.addedCategories} kategori, ${res.addedReasons} durum eklendi.`);
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "İçe aktarılamadı.");
    } finally {
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  return (
    <div className="space-y-6">
      <Card
        title="Eskalasyon Durumları"
        icon={Tags}
        actions={
          <div className="flex items-center gap-2">
            <input ref={fileRef} type="file" accept=".xlsx,.csv" className="hidden" onChange={onImport} />
            <Button variant="secondary" onClick={() => fileRef.current?.click()}>
              <Upload className="size-4" /> Excel içe aktar
            </Button>
          </div>
        }
      >
        <p className="mb-4 text-sm text-muted-foreground">
          Kategori bazlı durumları burada tanımlarsınız, temsilciler çağrı sırasında bunları seçer. Oklarla verdiğiniz sıra temsilcilerin listesine aynen yansır. Excel/CSV dosyasında ilk sütun kategori, ikinci sütun durum olmalıdır.
        </p>
        {importInfo && <p className="mb-3 text-sm text-success">{importInfo}</p>}
        <ErrorText>{error}</ErrorText>

        <form className="mb-5 flex gap-2" onSubmit={addCategory}>
          <Input value={newCat} onChange={(e) => setNewCat(e.target.value)} placeholder="Yeni kategori (örn. Memnuniyet)" className="max-w-xs" />
          <Button type="submit"><Plus className="size-4" /> Kategori ekle</Button>
        </form>

        {categories.length === 0 ? (
          <p className="text-sm text-muted-foreground">Henüz kategori yok.</p>
        ) : (
          <div className="grid gap-4 md:grid-cols-2">
            {categories.map((c, i) => (
              <CategoryCard
                key={c.id}
                category={c}
                index={i}
                count={categories.length}
                onMove={(dir) => moveCategory(i, dir)}
                onChange={load}
                onError={setError}
              />
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}

function CategoryCard({
  category,
  index,
  count,
  onMove,
  onChange,
  onError,
}: {
  category: EscalationCategory;
  index: number;
  count: number;
  onMove: (dir: -1 | 1) => void;
  onChange: () => void;
  onError: (msg: string | null) => void;
}) {
  const [newReason, setNewReason] = useState("");
  const [reasons, setReasons] = useState(category.reasons);
  useEffect(() => setReasons(category.reasons), [category.reasons]);

  async function moveReason(i: number, dir: -1 | 1) {
    const to = i + dir;
    if (to < 0 || to >= reasons.length) return;
    const next = [...reasons];
    [next[i], next[to]] = [next[to], next[i]];
    setReasons(next);
    try {
      await api.reorderEscalationReasons(category.id, next.map((r) => r.id));
    } catch (err) {
      onError(err instanceof ApiError ? err.message : "Sıralama kaydedilemedi.");
    }
    onChange();
  }

  async function addReason(e: React.FormEvent) {
    e.preventDefault();
    const name = newReason.trim();
    if (!name) return;
    await api.createEscalationReason(category.id, name).catch(() => undefined);
    setNewReason("");
    onChange();
  }

  return (
    <div className="rounded-2xl bg-muted/30 p-3 ring-1 ring-border/50">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-3">
          <IconChip icon={Tags} tone="warning" />
          <span className="min-w-0">
            <h3 className="truncate text-sm font-semibold leading-tight">{category.name}</h3>
            <span className="block text-xs text-muted-foreground">{index + 1}. sıra · {reasons.length} durum</span>
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-0.5">
          <OrderButton dir={-1} disabled={index === 0} onClick={() => onMove(-1)} title="Bir üste taşı" />
          <OrderButton dir={1} disabled={index === count - 1} onClick={() => onMove(1)} title="Bir alta taşı" />
          <button
            onClick={async () => { await api.deleteEscalationCategory(category.id).catch(() => undefined); onChange(); }}
            title="Kategoriyi sil"
            className="ml-1 text-muted-foreground transition hover:text-destructive"
          >
            <Trash2 className="size-4" />
          </button>
        </div>
      </div>
      <ul className="mb-3 space-y-1">
        {reasons.map((r, i) => (
          <li key={r.id} className="flex items-center justify-between gap-2 rounded-xl bg-card px-2.5 py-1.5 text-sm">
            <span className="flex min-w-0 items-center gap-2.5">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-muted/70 text-[0.65rem] font-semibold tabular-nums text-muted-foreground">{i + 1}</span>
              <span className="truncate">{r.name}</span>
            </span>
            <span className="flex shrink-0 items-center gap-0.5">
              <OrderButton dir={-1} small disabled={i === 0} onClick={() => moveReason(i, -1)} title="Bir üste taşı" />
              <OrderButton dir={1} small disabled={i === reasons.length - 1} onClick={() => moveReason(i, 1)} title="Bir alta taşı" />
              <button
                onClick={async () => { await api.deleteEscalationReason(r.id).catch(() => undefined); onChange(); }}
                title="Durumu sil"
                className="ml-1 text-muted-foreground transition hover:text-destructive"
              >
                <Trash2 className="size-3.5" />
              </button>
            </span>
          </li>
        ))}
        {reasons.length === 0 && <li className="px-1 text-xs text-muted-foreground">Durum yok.</li>}
      </ul>
      <form className="flex gap-2" onSubmit={addReason}>
        <Input value={newReason} onChange={(e) => setNewReason(e.target.value)} placeholder="Yeni durum" className="h-9" />
        <Button type="submit" variant="secondary" className="h-9 shrink-0 px-3"><Plus className="size-4" /></Button>
      </form>
    </div>
  );
}

// OrderButton is one of the up/down arrows that move a row a step.
function OrderButton({ dir, small, disabled, onClick, title }: { dir: -1 | 1; small?: boolean; disabled: boolean; onClick: () => void; title: string }) {
  const Icon = dir === -1 ? ChevronUp : ChevronDown;
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={cn("rounded-md text-muted-foreground transition hover:bg-accent hover:text-foreground disabled:opacity-25 disabled:hover:bg-transparent", small ? "p-0.5" : "p-1")}
    >
      <Icon className={small ? "size-3.5" : "size-4"} />
    </button>
  );
}
