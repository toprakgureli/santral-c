import { useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, FileJson, Sparkles, Upload } from "lucide-react";
import { ApiError } from "@/api/client";
import { apiTesterApi } from "@/apiTester/api";
import { parsePostmanCollection } from "@/apiTester/postman";
import type { ApiCollectionItem } from "@/apiTester/types";
import { Badge, Button, Field, Input, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";

interface PostmanImportModalProps {
  open: boolean;
  onClose: () => void;
  onImported: (col: ApiCollectionItem) => void;
}

export function PostmanImportModal({ open, onClose, onImported }: PostmanImportModalProps) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [rawJson, setRawJson] = useState("");
  const [fileName, setFileName] = useState("");
  const [asTemplate, setAsTemplate] = useState(true);
  const [templatizeBaseUrl, setTemplatizeBaseUrl] = useState(true);
  const [keepDefaults, setKeepDefaults] = useState(true);

  const [nameOverride, setNameOverride] = useState("");
  const [tagOverride, setTagOverride] = useState("");
  const [baseUrlOverride, setBaseUrlOverride] = useState("");

  const [busy, setBusy] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setRawJson("");
    setFileName("");
    setAsTemplate(true);
    setTemplatizeBaseUrl(true);
    setKeepDefaults(true);
    setNameOverride("");
    setTagOverride("");
    setBaseUrlOverride("");
    setSubmitError(null);
  }, [open]);

  const parsed = useMemo(() => {
    const trimmed = rawJson.trim();
    if (!trimmed) return { data: null, error: null };
    try {
      const data = parsePostmanCollection(trimmed, {
        asTemplate,
        templatizeBaseUrl,
        keepDefaults,
      });
      return { data, error: null };
    } catch (e) {
      return { data: null, error: e instanceof Error ? e.message : "Postman JSON dosyası okunamadı." };
    }
  }, [rawJson, asTemplate, templatizeBaseUrl, keepDefaults]);

  useEffect(() => {
    if (!parsed.data) return;
    setNameOverride(parsed.data.collection.name);
    setTagOverride(parsed.data.collection.projectTag || "Postman");
    setBaseUrlOverride(parsed.data.collection.baseUrl);
  }, [parsed.data]);

  const handleFile = async (file: File | undefined) => {
    if (!file) return;
    setSubmitError(null);
    setFileName(file.name);
    try {
      const text = await file.text();
      setRawJson(text);
    } catch {
      setSubmitError("Dosya okunamadı.");
    }
  };

  const submit = async () => {
    if (!parsed.data) return;
    setBusy(true);
    setSubmitError(null);
    try {
      const created = await apiTesterApi.importCollection(
        {
          ...parsed.data.collection,
          name: (nameOverride || parsed.data.collection.name).trim(),
          projectTag: tagOverride.trim(),
          baseUrl: baseUrlOverride.trim(),
        },
        parsed.data.requests,
      );
      onImported(created);
      onClose();
    } catch (e) {
      setSubmitError(e instanceof ApiError ? e.message : "Postman koleksiyonu içe aktarılamadı.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Postman Koleksiyonu İçe Aktar"
      description="Postman Collection v2.0 / v2.1 JSON dosyasını yükleyerek tüm istekleri tek seferde ekleyin."
      size="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>Vazgeç</Button>
          <Button onClick={submit} disabled={busy || !parsed.data || !nameOverride.trim()}>
            {busy ? "İçe Aktarılıyor..." : parsed.data ? `${parsed.data.requests.length} İsteği İçe Aktar` : "İçe Aktar"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-dashed border-border bg-muted/25 p-3">
          <div className="flex items-center gap-2.5 text-xs">
            <FileJson className="size-4 text-primary" />
            <span>{fileName ? `Seçilen dosya: ${fileName}` : "Postman Export (.json) seçin veya aşağıya JSON yapıştırın."}</span>
          </div>
          <div>
            <input
              ref={fileRef}
              type="file"
              accept=".json,application/json"
              className="hidden"
              onChange={(e) => void handleFile(e.target.files?.[0])}
            />
            <Button type="button" variant="secondary" className="h-8 px-3 text-xs" onClick={() => fileRef.current?.click()}>
              <Upload className="size-3.5" /> JSON Dosyası Seç
            </Button>
          </div>
        </div>

        <Field label="Postman Collection JSON İçeriği">
          <textarea
            rows={5}
            value={rawJson}
            onChange={(e) => setRawJson(e.target.value)}
            placeholder='{"info":{"name":"Müşteri API",...},"item":[...]}'
            className="w-full rounded-xl border border-border/70 bg-muted/40 p-3 font-mono text-xs text-foreground outline-none focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
          />
        </Field>

        {/* Ask whether auto-import should convert values to {{}} template placeholders */}
        <div className="space-y-2.5 rounded-xl border border-border/70 bg-muted/20 p-3.5">
          <div className="flex items-center gap-2 text-xs font-semibold">
            <Sparkles className="size-4 text-primary" />
            <span>Otomatik aktarımda değerler {"{{}}"} şablonu olarak da eklensin mi?</span>
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            <button
              type="button"
              onClick={() => setAsTemplate(true)}
              className={cn(
                "rounded-xl border p-2.5 text-left text-xs transition",
                asTemplate
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border/60 bg-card text-muted-foreground hover:text-foreground",
              )}
            >
              <p className="font-semibold">Evet, {"{{}}"} Şablonuna Çevir (Önerilen)</p>
              <p className="mt-0.5 text-[11px] opacity-85">
                Query parametreleri, :yol parametreleri ve JSON gövde alanlarını doldurulabilir {"{{degisken}}"} kutularına dönüştürür.
              </p>
            </button>
            <button
              type="button"
              onClick={() => setAsTemplate(false)}
              className={cn(
                "rounded-xl border p-2.5 text-left text-xs transition",
                !asTemplate
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border/60 bg-card text-muted-foreground hover:text-foreground",
              )}
            >
              <p className="font-semibold">Hayır, Olduğu Gibi Ham Aktar</p>
              <p className="mt-0.5 text-[11px] opacity-85">
                Sabit değerlere dokunmaz; sadece Postman içinde zaten yazılı {"{{...}}"} alanlarını tanır.
              </p>
            </button>
          </div>
          {asTemplate && (
            <div className="flex flex-wrap gap-4 pt-1 text-xs text-muted-foreground">
              <label className="flex cursor-pointer items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={templatizeBaseUrl}
                  onChange={(e) => setTemplatizeBaseUrl(e.target.checked)}
                />
                <span>Alan adını {"{{base_url}}"} değişkenine çıkar</span>
              </label>
              <label className="flex cursor-pointer items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={keepDefaults}
                  onChange={(e) => setKeepDefaults(e.target.checked)}
                />
                <span>Örnek değerleri formda varsayılan olarak sakla</span>
              </label>
            </div>
          )}
        </div>

        {parsed.error && <p className="text-sm text-destructive">{parsed.error}</p>}

        {parsed.data && (
          <div className="space-y-3 rounded-xl border border-border/60 p-3.5">
            <div className="grid gap-3 sm:grid-cols-3">
              <Field label="Koleksiyon Adı">
                <Input value={nameOverride} onChange={(e) => setNameOverride(e.target.value)} className="h-9 text-xs" />
              </Field>
              <Field label="Etiket">
                <Input value={tagOverride} onChange={(e) => setTagOverride(e.target.value)} className="h-9 text-xs" />
              </Field>
              <Field label="Taban URL ({{base_url}})">
                <Input value={baseUrlOverride} onChange={(e) => setBaseUrlOverride(e.target.value)} placeholder="https://api..." className="h-9 font-mono text-xs" />
              </Field>
            </div>

            {parsed.data.warnings.length > 0 && (
              <div className="space-y-1 rounded-lg border border-amber-500/40 bg-amber-500/10 p-2.5 text-xs text-amber-800 dark:text-amber-300">
                {parsed.data.warnings.map((w, i) => (
                  <p key={i} className="flex items-center gap-1.5">
                    <AlertTriangle className="size-3.5 shrink-0" /> {w}
                  </p>
                ))}
              </div>
            )}

            <div className="space-y-1.5">
              <p className="text-xs font-semibold">Önizleme ({parsed.data.requests.length} İstek)</p>
              <div className="max-h-56 space-y-1.5 overflow-y-auto pr-1">
                {parsed.data.requests.map((r, i) => (
                  <div key={i} className="rounded-lg border border-border/50 bg-muted/25 px-3 py-2 text-xs">
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex min-w-0 items-center gap-2">
                        <Badge tone={r.method === "GET" ? "green" : r.method === "DELETE" ? "red" : "blue"}>{r.method}</Badge>
                        <span className="truncate font-semibold">{r.name}</span>
                      </div>
                      <span className="shrink-0 text-[11px] text-muted-foreground">
                        {r.variables.length} değişken
                      </span>
                    </div>
                    <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">{r.urlTemplate}</p>
                    {r.variables.length > 0 && (
                      <div className="mt-1.5 flex flex-wrap gap-1">
                        {r.variables.map((v) => (
                          <code key={v.key} className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary">
                            {`{{${v.key}}}`}
                          </code>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {submitError && <p className="text-sm text-destructive">{submitError}</p>}
      </div>
    </Modal>
  );
}
