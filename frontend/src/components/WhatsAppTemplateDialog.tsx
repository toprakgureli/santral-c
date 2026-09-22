// WhatsAppTemplateDialog lets an agent write their own follow-up message.
// Placeholders are inserted by chips, the preview renders with the agent's
// real name, and the text is saved on their account.

import { useEffect, useRef, useState } from "react";
import { MessageCircle, RotateCcw } from "lucide-react";
import { api, ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";
import { DEFAULT_WHATSAPP_TEMPLATE, WHATSAPP_PLACEHOLDERS, WHATSAPP_TEMPLATE_MAX, renderWhatsAppTemplate } from "@/lib/whatsapp";

export default function WhatsAppTemplateDialog({ open, onClose, previewNumber }: { open: boolean; onClose: () => void; previewNumber?: string }) {
  const { user, setUser } = useAuth();
  const [text, setText] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const area = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (open) {
      setText(user?.whatsappTemplate?.trim() ? user.whatsappTemplate : DEFAULT_WHATSAPP_TEMPLATE);
      setError(null);
    }
  }, [open, user?.whatsappTemplate]);

  function insert(token: string) {
    const el = area.current;
    if (!el) {
      setText((t) => t + token);
      return;
    }
    const start = el.selectionStart ?? text.length;
    const end = el.selectionEnd ?? text.length;
    const next = text.slice(0, start) + token + text.slice(end);
    setText(next);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(start + token.length, start + token.length);
    });
  }

  async function save() {
    setSaving(true);
    setError(null);
    try {
      // Saving the default as empty keeps future default changes flowing through.
      const value = text.trim() === DEFAULT_WHATSAPP_TEMPLATE ? "" : text.trim();
      const u = await api.setMyWhatsAppTemplate(value);
      setUser(u);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setSaving(false);
    }
  }

  const preview = renderWhatsAppTemplate(text, { name: user?.name ?? "", number: previewNumber || "5304230113" });
  const tooLong = text.length > WHATSAPP_TEMPLATE_MAX;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="WhatsApp mesajın"
      description="Ulaşamadığın müşteriye tek tıkla gidecek mesaj. Sadece senin hesabında geçerli."
      footer={
        <>
          <Button variant="ghost" onClick={() => setText(DEFAULT_WHATSAPP_TEMPLATE)} className="mr-auto h-9 px-3 text-xs">
            <RotateCcw className="size-3.5" /> Varsayılana dön
          </Button>
          <Button variant="secondary" onClick={onClose} className="h-9" disabled={saving}>Vazgeç</Button>
          <Button onClick={save} className="h-9" disabled={saving || tooLong || !text.trim()}>
            {saving ? "Kaydediliyor..." : "Kaydet"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs text-muted-foreground">Ekle:</span>
          {WHATSAPP_PLACEHOLDERS.map((p) => (
            <button
              key={p.key}
              type="button"
              onClick={() => insert(p.key)}
              title={p.label}
              className="rounded-full border border-border/70 bg-muted/40 px-2.5 py-1 font-mono text-xs transition-colors hover:border-border hover:bg-accent"
            >
              {p.key}
            </button>
          ))}
        </div>

        <textarea
          ref={area}
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={5}
          className={cn(
            "w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2.5 text-sm outline-none transition focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20",
            tooLong && "border-destructive",
          )}
        />
        <div className={cn("text-right text-[0.7rem]", tooLong ? "text-destructive" : "text-muted-foreground")}>
          {text.length} / {WHATSAPP_TEMPLATE_MAX}
        </div>

        {/* Preview in a WhatsApp-like bubble */}
        <div>
          <div className="mb-1.5 flex items-center gap-1.5 text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">
            <MessageCircle className="size-3.5 text-[#25D366]" /> Önizleme
          </div>
          <div className="rounded-2xl rounded-tl-sm bg-[#25D366]/10 px-4 py-3 text-sm leading-relaxed ring-1 ring-[#25D366]/25">
            {preview}
          </div>
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}
      </div>
    </Modal>
  );
}
