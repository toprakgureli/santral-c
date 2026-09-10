import { useEffect, useRef, useState } from "react";
import { Plus, Trash2, Upload } from "lucide-react";
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
          Kategori bazlı durumları burada tanımlarsınız; temsilciler çağrı sırasında bunları seçer. Excel/CSV dosyasında ilk sütun kategori, ikinci sütun durum olmalıdır.
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
            {categories.map((c) => (
              <CategoryCard key={c.id} category={c} onChange={load} />
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}

function CategoryCard({ category, onChange }: { category: EscalationCategory; onChange: () => void }) {
  const [newReason, setNewReason] = useState("");

  async function addReason(e: React.FormEvent) {
    e.preventDefault();
    const name = newReason.trim();
    if (!name) return;
    await api.createEscalationReason(category.id, name).catch(() => undefined);
    setNewReason("");
    onChange();
  }

  return (
    <div className="rounded-xl border border-border/60 bg-muted/30 p-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="font-semibold">{category.name}</h3>
        <button
          onClick={async () => { await api.deleteEscalationCategory(category.id).catch(() => undefined); onChange(); }}
          title="Kategoriyi sil"
          className="text-muted-foreground transition hover:text-destructive"
        >
          <Trash2 className="size-4" />
        </button>
      </div>
      <ul className="mb-3 space-y-1">
        {category.reasons.map((r) => (
          <li key={r.id} className="flex items-center justify-between gap-2 rounded-lg bg-card px-3 py-1.5 text-sm">
            <span>{r.name}</span>
            <button
              onClick={async () => { await api.deleteEscalationReason(r.id).catch(() => undefined); onChange(); }}
              title="Durumu sil"
              className="text-muted-foreground transition hover:text-destructive"
            >
              <Trash2 className="size-3.5" />
            </button>
          </li>
        ))}
        {category.reasons.length === 0 && <li className="px-1 text-xs text-muted-foreground">Durum yok.</li>}
      </ul>
      <form className="flex gap-2" onSubmit={addReason}>
        <Input value={newReason} onChange={(e) => setNewReason(e.target.value)} placeholder="Yeni durum" className="h-9" />
        <Button type="submit" variant="secondary" className="h-9 shrink-0 px-3"><Plus className="size-4" /></Button>
      </form>
    </div>
  );
}
