// NewChatDialog starts a conversation with a number. WhatsApp only lets a
// business write first with an approved template, so the chat opens with
// the template picker.

import { useEffect, useState } from "react";
import { ApiError } from "@/api/client";
import { Button, Modal } from "@/components/ui";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation } from "@/whatsapp/types";

export default function NewChatDialog({ open, channels, onClose, onStarted }: { open: boolean; channels: WAChannel[]; onClose: () => void; onStarted: (c: WAConversation) => void }) {
  const [channel, setChannel] = useState(0);
  const [number, setNumber] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setChannel(channels[0]?.id ?? 0);
    setNumber("");
    setName("");
    setError(null);
  }, [open, channels]);

  const start = async () => {
    setBusy(true);
    setError(null);
    try {
      const c = await waApi.start(channel, number, name);
      onStarted(c);
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Sohbet başlatılamadı.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="Yeni sohbet" description="Müşteriye ilk mesajı yalnızca onaylı bir şablonla gönderebilirsiniz. Sohbet açılınca şablon seçin."
      footer={<>
        {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
        <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
        <Button onClick={() => void start()} disabled={busy || !channel || number.replace(/\D/g, "").length < 10}>{busy ? "Açılıyor..." : "Sohbeti aç"}</Button>
      </>}
    >
      <div className="space-y-3">
        {channels.length > 1 && (
          <label className="block space-y-1">
            <span className="text-xs font-medium text-muted-foreground">Hangi numaradan</span>
            <select value={channel} onChange={(e) => setChannel(Number(e.target.value))} className="h-10 w-full rounded-xl border border-border/60 bg-card px-3 text-sm outline-none">
              {channels.map((c) => <option key={c.id} value={c.id}>{c.name}{c.displayPhone ? ` · ${c.displayPhone}` : ""}</option>)}
            </select>
          </label>
        )}
        <label className="block space-y-1">
          <span className="text-xs font-medium text-muted-foreground">Müşterinin numarası</span>
          <input autoFocus value={number} onChange={(e) => setNumber(e.target.value)} inputMode="tel" placeholder="0530 123 45 67" className="h-10 w-full rounded-xl border border-border/60 bg-card px-3 font-mono text-sm outline-none focus:border-ring/50 focus:ring-4 focus:ring-ring/15" />
        </label>
        <label className="block space-y-1">
          <span className="text-xs font-medium text-muted-foreground">Adı (isteğe bağlı)</span>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Ayşe Yılmaz" className="h-10 w-full rounded-xl border border-border/60 bg-card px-3 text-sm outline-none focus:border-ring/50 focus:ring-4 focus:ring-ring/15" />
        </label>
      </div>
    </Modal>
  );
}
