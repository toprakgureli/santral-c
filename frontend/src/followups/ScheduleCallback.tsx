// ScheduleCallback plans a call back to a customer for a time: from the
// wrap-up card after a call, from the follow-up list, or by hand with any
// number. When the time comes the panel reminds its owner on every page.

import { useState } from "react";
import { AlarmClock, Loader2 } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Field, Input, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";
import { announceFollowupsChanged, followApi } from "./api";

// local writes a time as a datetime-local input expects it.
function local(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

// inMinutes is now plus some minutes, on a five-minute mark.
function inMinutes(m: number): Date {
  const d = new Date(Date.now() + m * 60000);
  d.setSeconds(0, 0);
  d.setMinutes(Math.ceil(d.getMinutes() / 5) * 5);
  return d;
}

function tomorrowAt(h: number): Date {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  d.setHours(h, 0, 0, 0);
  return d;
}

const PRESETS: { label: string; at: () => Date }[] = [
  { label: "30 dk sonra", at: () => inMinutes(30) },
  { label: "1 saat sonra", at: () => inMinutes(60) },
  { label: "2 saat sonra", at: () => inMinutes(120) },
  { label: "Yarın 10:00", at: () => tomorrowAt(10) },
];

export default function ScheduleCallback({ number: given, onClose, onSaved }: { number?: string; onClose: () => void; onSaved?: () => void }) {
  const [number, setNumber] = useState(given ?? "");
  const [at, setAt] = useState(() => local(inMinutes(60)));
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = async () => {
    const due = new Date(at);
    if (Number.isNaN(due.getTime())) {
      setError("Geçerli bir tarih ve saat seç.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await followApi.createReminder({ number: number.trim(), note: note.trim(), dueAt: due.toISOString() });
      announceFollowupsChanged();
      onSaved?.();
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Geri arama planlanamadı.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title="Geri arama planla"
      description="Saati gelince her sayfada hatırlatılır. O müşteriyle kim görüşürse görüşsün kendiliğinden kapanır."
      dirty={!!note.trim() || (!given && !!number.trim())}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>Vazgeç</Button>
          <Button onClick={() => void save()} disabled={busy || number.trim().length < 6}>
            {busy ? <Loader2 className="animate-spin" /> : <AlarmClock />} Planla
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
        <Field label="Numara">
          <Input value={number} onChange={(e) => setNumber(e.target.value)} readOnly={!!given} inputMode="tel" placeholder="05xx xxx xx xx" autoFocus={!given} className={cn(given && "bg-muted/50 font-mono")} />
        </Field>
        <div className="space-y-2">
          <Field label="Ne zaman">
            <Input type="datetime-local" value={at} onChange={(e) => setAt(e.target.value)} />
          </Field>
          <div className="flex flex-wrap gap-1.5">
            {PRESETS.map((p) => (
              <button key={p.label} type="button" onClick={() => setAt(local(p.at()))} className="rounded-full bg-muted/70 px-3 py-1 text-xs font-medium transition-colors hover:bg-accent">{p.label}</button>
            ))}
          </div>
        </div>
        <Field label="Not (isteğe bağlı)">
          <Input value={note} maxLength={300} onChange={(e) => setNote(e.target.value)} placeholder="Örn: Fatura itirazı için dönüş sözü verildi" />
        </Field>
      </div>
    </Modal>
  );
}
