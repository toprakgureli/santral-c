import { useEffect, useState } from "react";
import { ApiError } from "@/api/client";
import { apiTesterApi } from "@/apiTester/api";
import type { ApiCollectionItem, CollectionInput } from "@/apiTester/types";
import { Button, Field, Input, Modal } from "@/components/ui";
import { Switch } from "@/components/whatsapp/settings/parts";

interface CollectionModalProps {
  open: boolean;
  editing: ApiCollectionItem | null;
  onClose: () => void;
  onSaved: (col: ApiCollectionItem) => void;
}

export function CollectionModal({ open, editing, onClose, onSaved }: CollectionModalProps) {
  const [name, setName] = useState("");
  const [projectTag, setProjectTag] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [description, setDescription] = useState("");
  const [active, setActive] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    if (editing) {
      setName(editing.name);
      setProjectTag(editing.projectTag);
      setBaseUrl(editing.baseUrl);
      setDescription(editing.description);
      setActive(editing.active);
    } else {
      setName("");
      setProjectTag("");
      setBaseUrl("");
      setDescription("");
      setActive(true);
    }
  }, [open, editing]);

  const submit = async () => {
    setBusy(true);
    setError(null);
    const payload: CollectionInput = {
      name: name.trim(),
      projectTag: projectTag.trim(),
      baseUrl: baseUrl.trim(),
      description: description.trim(),
      active,
    };
    try {
      const saved = editing
        ? await apiTesterApi.updateCollection(editing.id, payload)
        : await apiTesterApi.createCollection(payload);
      onSaved(saved);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Koleksiyon kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={editing ? "Koleksiyonu Düzenle" : "Yeni API Koleksiyonu"}
      description="Proje veya ürün bazlı test isteklerini aynı grupta toplayın."
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>Vazgeç</Button>
          <Button onClick={submit} disabled={busy || !name.trim()}>
            {busy ? "Kaydediliyor..." : editing ? "Güncelle" : "Oluştur"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Koleksiyon Adı">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Örn. E-Ticaret Entegrasyon API" />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Proje / Ürün Etiketi (Opsiyonel)">
            <Input value={projectTag} onChange={(e) => setProjectTag(e.target.value)} placeholder="Örn. ERP, CRM, Kargo" />
          </Field>
          <Field label="Varsayılan Taban URL ({{base_url}})">
            <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://api.musteri.com" />
          </Field>
        </div>
        <Field label="Açıklama">
          <textarea
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Bu koleksiyonun hangi müşteri veya ürün için kullanıldığını yazın..."
            className="w-full rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2 text-sm text-foreground outline-none focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
          />
        </Field>
        {editing && (
          <div className="flex items-center justify-between rounded-xl border border-border/60 px-3.5 py-2.5">
            <div>
              <p className="text-sm font-medium">Aktif</p>
              <p className="text-xs text-muted-foreground">Pasif koleksiyonlar sadece yöneticilere görünür.</p>
            </div>
            <Switch on={active} onChange={setActive} label="Koleksiyon aktif" />
          </div>
        )}
        {error && <p className="text-sm text-destructive">{error}</p>}
      </div>
    </Modal>
  );
}
