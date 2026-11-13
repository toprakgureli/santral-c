import { useEffect, useState } from "react";
import { parseCurlCommand } from "@/apiTester/runner";
import type { RequestInput } from "@/apiTester/types";
import { Button, Field, Modal } from "@/components/ui";

interface CurlImportModalProps {
  open: boolean;
  onClose: () => void;
  onImported: (parsed: Partial<RequestInput>) => void;
}

export function CurlImportModal({ open, onClose, onImported }: CurlImportModalProps) {
  const [raw, setRaw] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setRaw("");
      setError(null);
    }
  }, [open]);

  const apply = () => {
    setError(null);
    try {
      const parsed = parseCurlCommand(raw);
      onImported(parsed);
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "cURL komutu çözümlenemedi.");
    }
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
          <Button onClick={apply} disabled={!raw.trim()}>Formu Doldur</Button>
        </>
      }
    >
      <div className="space-y-3">
        <Field label="cURL Komutu">
          <textarea
            rows={8}
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            placeholder={`curl -X POST "https://api.example.com/v1/check?phone={{phone}}" \\\n  -H "Authorization: Bearer YOUR_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d '{"phone":"{{phone}}"}'`}
            className="w-full rounded-xl border border-border/70 bg-muted/40 p-3 font-mono text-xs text-foreground outline-none focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
          />
        </Field>
        {error && <p className="text-sm text-destructive">{error}</p>}
      </div>
    </Modal>
  );
}
