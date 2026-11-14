import { useEffect, useMemo, useState } from "react";
import { Sparkles } from "lucide-react";
import { templatizeRequestInput } from "@/apiTester/postman";
import { parseCurlCommand } from "@/apiTester/runner";
import type { RequestInput } from "@/apiTester/types";
import { Button, Field, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";

interface CurlImportModalProps {
  open: boolean;
  onClose: () => void;
  onImported: (parsed: Partial<RequestInput>) => void;
}

export function CurlImportModal({ open, onClose, onImported }: CurlImportModalProps) {
  const [raw, setRaw] = useState("");
  const [asTemplate, setAsTemplate] = useState(true);
  const [templatizeBaseUrl, setTemplatizeBaseUrl] = useState(false);
  const [keepDefaults, setKeepDefaults] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setRaw("");
      setAsTemplate(true);
      setTemplatizeBaseUrl(false);
      setKeepDefaults(true);
      setError(null);
    }
  }, [open]);

  const preview = useMemo(() => {
    const trimmed = raw.trim();
    if (!trimmed) return { request: null as RequestInput | null, parseErr: null as string | null };
    try {
      const parsed = parseCurlCommand(trimmed);
      const baseReq: RequestInput = {
        name: parsed.name || "cURL İsteği",
        description: "",
        method: parsed.method || "GET",
        urlTemplate: parsed.urlTemplate || "",
        headers: parsed.headers ?? [],
        queryParams: parsed.queryParams ?? [],
        bodyTemplate: parsed.bodyTemplate ?? "",
        contentType: parsed.contentType || "application/json",
        variables: [],
        executionMode: "client",
        allowServerFallback: true,
        runbook: "",
      };
      const res = templatizeRequestInput(baseReq, {
        asTemplate,
        templatizeBaseUrl,
        keepDefaults,
      });
      return { request: res.request, parseErr: null };
    } catch (e) {
      return { request: null, parseErr: e instanceof Error ? e.message : "cURL komutu çözümlenemedi." };
    }
  }, [raw, asTemplate, templatizeBaseUrl, keepDefaults]);

  const apply = () => {
    setError(null);
    if (preview.parseErr || !preview.request) {
      setError(preview.parseErr || "Geçerli bir cURL komutu girin.");
      return;
    }
    onImported(preview.request);
    onClose();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="cURL Komutundan İçe Aktar"
      description="Postman, Swagger veya tarayıcı ağ sekmesinden kopyaladığınız cURL komutunu yapıştırın."
      size="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
          <Button onClick={apply} disabled={!raw.trim() || !preview.request}>Formu Doldur</Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="cURL Komutu">
          <textarea
            rows={7}
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            placeholder={`curl -X POST "https://api.example.com/v1/check?phone=05551234567" \\\n  -H "Authorization: Bearer YOUR_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d '{"phone":"05551234567"}'`}
            className="w-full rounded-xl border border-border/70 bg-muted/40 p-3 font-mono text-xs text-foreground outline-none focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
          />
        </Field>

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
              <p className="font-semibold">Evet, {"{{}}"} Şablonu Olarak Ekle</p>
              <p className="mt-0.5 text-[11px] opacity-85">
                Query ve JSON gövde değerlerini {"{{degisken}}"} parametre formuna dönüştürür.
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
              <p className="font-semibold">Hayır, Sabit Değerlerle Aktar</p>
              <p className="mt-0.5 text-[11px] opacity-85">
                Komuttaki değerleri değiştirmeden doğrudan yükler.
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
                <span>Alan adını {"{{base_url}}"} yap</span>
              </label>
              <label className="flex cursor-pointer items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={keepDefaults}
                  onChange={(e) => setKeepDefaults(e.target.checked)}
                />
                <span>Örnek değerleri varsayılan olarak sakla</span>
              </label>
            </div>
          )}
        </div>

        {preview.parseErr && raw.trim() && (
          <p className="text-xs text-destructive">{preview.parseErr}</p>
        )}

        {preview.request && (
          <div className="space-y-1.5 rounded-xl border border-border/60 bg-muted/25 p-3 text-xs">
            <div className="flex items-center justify-between gap-2">
              <span className="font-semibold">{preview.request.method}</span>
              <code className="truncate font-mono text-[11px] text-muted-foreground">{preview.request.urlTemplate}</code>
            </div>
            {preview.request.variables.length > 0 && (
              <div className="flex flex-wrap items-center gap-1.5 pt-1">
                <span className="text-[11px] text-muted-foreground">Oluşacak değişkenler:</span>
                {preview.request.variables.map((v) => (
                  <code key={v.key} className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary">
                    {`{{${v.key}}}`}
                  </code>
                ))}
              </div>
            )}
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}
      </div>
    </Modal>
  );
}
