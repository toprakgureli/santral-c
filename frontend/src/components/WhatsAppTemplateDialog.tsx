// WhatsAppTemplateDialog lets an agent write their own WhatsApp texts: one
// for a customer they could not reach, one for a customer on the phone right
// now. Placeholders are inserted by chips, the preview renders with the
// agent's real name, both texts are saved on their account.

import { useEffect, useRef, useState } from "react";
import { MessageCircle, RotateCcw } from "lucide-react";
import { api, ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";
import {
  DEFAULT_WHATSAPP_LIVE_TEMPLATE,
  DEFAULT_WHATSAPP_TEMPLATE,
  WHATSAPP_KINDS,
  WHATSAPP_PLACEHOLDERS,
  WHATSAPP_TEMPLATE_MAX,
  renderWhatsAppTemplate,
  type WhatsAppKind,
} from "@/lib/whatsapp";

const DEFAULTS: Record<WhatsAppKind, string> = { unreached: DEFAULT_WHATSAPP_TEMPLATE, live: DEFAULT_WHATSAPP_LIVE_TEMPLATE };

export default function WhatsAppTemplateDialog({
  open,
  onClose,
  previewNumber,
  initialKind = "unreached",
}: {
  open: boolean;
  onClose: () => void;
  previewNumber?: string;
  initialKind?: WhatsAppKind;
}) {
  const { user, setUser } = useAuth();
  const [kind, setKind] = useState<WhatsAppKind>(initialKind);
  const [texts, setTexts] = useState<Record<WhatsAppKind, string>>({ unreached: "", live: "" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const area = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (!open) return;
    setKind(initialKind);
    setTexts({
      unreached: user?.whatsappTemplate?.trim() ? user.whatsappTemplate : DEFAULT_WHATSAPP_TEMPLATE,
      live: user?.whatsappTemplateLive?.trim() ? user.whatsappTemplateLive : DEFAULT_WHATSAPP_LIVE_TEMPLATE,
    });
    setError(null);
  }, [open, initialKind, user?.whatsappTemplate, user?.whatsappTemplateLive]);

  const text = texts[kind];
  const setText = (v: string) => setTexts((t) => ({ ...t, [kind]: v }));

  function insert(token: string) {
    const el = area.current;
    if (!el) {
      setText(text + token);
      return;
    }
    const start = el.selectionStart ?? text.length;
    const end = el.selectionEnd ?? text.length;
    setText(text.slice(0, start) + token + text.slice(end));
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(start + token.length, start + token.length);
    });
  }

  async function save() {
    setSaving(true);
    setError(null);
    try {
      // A text equal to the default is stored empty, so default wording
      // changes keep flowing to accounts that never customised it.
      const store = (k: WhatsAppKind) => (texts[k].trim() === DEFAULTS[k] ? "" : texts[k].trim());
      const u = await api.setMyWhatsAppTemplates({ template: store("unreached"), live: store("live") });
      setUser(u);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setSaving(false);
    }
  }

  const preview = renderWhatsAppTemplate(text, { name: user?.name ?? "", number: previewNumber || "5304230113" });
  const tooLong = Object.values(texts).some((t) => t.length > WHATSAPP_TEMPLATE_MAX);
  const empty = Object.values(texts).some((t) => !t.trim());

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="WhatsApp mesajların"
      description="İki ayrı metin, ikisi de sadece senin hesabında geçerli."
      footer={
        <>
          <Button variant="ghost" onClick={() => setText(DEFAULTS[kind])} className="mr-auto h-9 px-3 text-xs">
            <RotateCcw className="size-3.5" /> Bu metni varsayılana dön
          </Button>
          <Button variant="secondary" onClick={onClose} className="h-9" disabled={saving}>Vazgeç</Button>
          <Button onClick={save} className="h-9" disabled={saving || tooLong || empty}>
            {saving ? "Kaydediliyor..." : "İkisini de kaydet"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {/* Which text */}
        <div className="grid grid-cols-2 gap-1 rounded-xl bg-muted/40 p-1">
          {WHATSAPP_KINDS.map((k) => (
            <button
              key={k.kind}
              type="button"
              onClick={() => setKind(k.kind)}
              className={cn(
                "rounded-lg px-3 py-2 text-left transition-colors",
                kind === k.kind ? "bg-card shadow-sm ring-1 ring-border/60" : "hover:bg-accent/60",
              )}
            >
              <span className="block text-sm font-medium leading-tight">{k.label}</span>
              <span className="block text-xs text-muted-foreground">{k.hint}</span>
            </button>
          ))}
        </div>

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
            text.length > WHATSAPP_TEMPLATE_MAX && "border-destructive",
          )}
        />
        <div className={cn("text-right text-[0.7rem]", text.length > WHATSAPP_TEMPLATE_MAX ? "text-destructive" : "text-muted-foreground")}>
          {text.length} / {WHATSAPP_TEMPLATE_MAX}
        </div>

        <div>
          <div className="mb-1.5 flex items-center gap-1.5 text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">
            <MessageCircle className="size-3.5 text-[#25D366]" /> Önizleme
          </div>
          <div className="rounded-2xl rounded-tl-sm bg-[#25D366]/10 px-4 py-3 text-sm leading-relaxed ring-1 ring-[#25D366]/25">{preview}</div>
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}
      </div>
    </Modal>
  );
}
