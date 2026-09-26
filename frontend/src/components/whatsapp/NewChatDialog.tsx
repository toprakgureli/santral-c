// NewChatDialog starts a conversation with a number. WhatsApp only lets a
// business write first with an approved template, so the number and the
// device come first, then the template; the conversation is opened only
// when the template goes out. If the number already has a conversation,
// it is offered instead.

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowRight, MessageCircle } from "lucide-react";
import { Button, Modal } from "@/components/ui";
import TemplatePicker, { type TemplateChoice } from "@/components/whatsapp/TemplatePicker";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation } from "@/whatsapp/types";
import { listTime, newClientId, STATUS_WORD } from "@/whatsapp/util";

export default function NewChatDialog({ open, number: initialNumber, name: initialName, onClose, onStarted }: { open: boolean; number?: string; name?: string; onClose: () => void; onStarted?: (c: WAConversation) => void }) {
  const navigate = useNavigate();
  const [channels, setChannels] = useState<WAChannel[]>([]);
  const [channel, setChannel] = useState(0);
  const [number, setNumber] = useState("");
  const [name, setName] = useState("");
  const [existing, setExisting] = useState<WAConversation[]>([]);
  const [picking, setPicking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setNumber(initialNumber ?? "");
    setName(initialName ?? "");
    setError(null);
    setPicking(false);
    waApi.channels().then((l) => {
      const act = l.filter((c) => c.active);
      setChannels(act);
      setChannel((cur) => (act.some((c) => c.id === cur) ? cur : act[0]?.id ?? 0));
    }).catch(() => setChannels([]));
  }, [open, initialNumber, initialName]);

  // Conversations this number already has.
  const digits = number.replace(/\D/g, "");
  useEffect(() => {
    if (!open || digits.length < 10) {
      setExisting([]);
      return;
    }
    let live = true;
    const t = window.setTimeout(() => {
      waApi.lookup(number).then((l) => live && setExisting(l)).catch(() => live && setExisting([]));
    }, 300);
    return () => {
      live = false;
      window.clearTimeout(t);
    };
  }, [open, digits]); // eslint-disable-line react-hooks/exhaustive-deps

  const openChat = (c: WAConversation) => {
    onClose();
    navigate(`/whatsapp/${c.id}`);
  };

  const send = async (choice: TemplateChoice) => {
    // Errors here reach the template window, which shows them.
    const conv = await waApi.start(channel, number, name);
    await waApi.send(conv.id, { clientId: newClientId(), kind: "template", templateId: choice.templateId, params: choice.params });
    onStarted?.(conv);
    onClose();
    navigate(`/whatsapp/${conv.id}`);
  };

  const sameDevice = existing.find((c) => c.channelId === channel);
  const now = Date.now();

  return (
    <>
      <Modal open={open && !picking} onClose={onClose} title="WhatsApp'tan yaz" description="Müşteriye ilk mesaj yalnızca Meta'nın onayladığı bir şablonla gönderilebilir. Numarayı yazın, sonra şablonu seçin."
        footer={<>
          {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
          <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
          <Button onClick={() => { setError(null); setPicking(true); }} disabled={!channel || digits.length < 10}>Şablon seç <ArrowRight /></Button>
        </>}
      >
        {channels.length === 0 ? (
          <p className="rounded-xl bg-muted/40 px-4 py-6 text-center text-sm text-muted-foreground">Açık bir WhatsApp numarası yok ya da hiçbir numarada çalışmıyorsunuz.</p>
        ) : (
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
            {existing.length > 0 && (
              <div className="space-y-1.5 rounded-2xl bg-emerald-500/8 p-3 ring-1 ring-emerald-600/20">
                <p className="text-xs font-medium text-emerald-800 dark:text-emerald-300">Bu numarayla zaten sohbet var{sameDevice && sameDevice.windowEndsAt && Date.parse(sameDevice.windowEndsAt) > now ? ". Müşteri son 24 saatte yazmış, şablonsuz da yazabilirsiniz." : "."}</p>
                {existing.slice(0, 3).map((c) => (
                  <button key={c.id} type="button" onClick={() => openChat(c)} className="flex w-full items-center gap-2.5 rounded-xl bg-card px-2.5 py-2 text-left ring-1 ring-border/60 hover:bg-accent/60">
                    <MessageCircle className="size-4 shrink-0 text-emerald-600" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">{c.contact.display} <span className="font-normal text-muted-foreground">· {c.channelName}</span></span>
                      <span className="block truncate text-[0.7rem] text-muted-foreground">{c.ticket ? STATUS_WORD[c.ticket.status] : "Kayıt"}{c.last ? ` · ${listTime(c.last.at)} · ${c.last.preview}` : ""}</span>
                    </span>
                    <span className={cn("shrink-0 text-xs font-semibold text-primary")}>Aç</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </Modal>
      {picking && (
        <TemplatePicker channelId={channel} open onClose={() => setPicking(false)} onSend={send} defaults={{ musteri: name.trim().split(" ")[0] ?? "" }} />
      )}
    </>
  );
}
