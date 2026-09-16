// EscalationForm is the three-step escalation entry (kategori, durum, not)
// for one customer number, with the customer's past escalations above it.
// The dashboard card and the after-call wrap-up card both render it.

import { useCallback, useEffect, useState, type ReactNode } from "react";
import { api, ApiError } from "@/api/client";
import type { EscalationCategory, EscalationRecord } from "@/api/types";
import { Badge, Button } from "@/components/ui";
import { EscalationPicker } from "@/components/escalation/EscalationPicker";
import { cn } from "@/lib/utils";

export function EscalationForm({
  categories,
  number,
  canSearch,
  callUuid,
  onSaved,
  onHistory,
  aside,
  showHistory = true,
  pickerHeight,
}: {
  categories: EscalationCategory[];
  number: string;
  canSearch: boolean;
  callUuid?: string;
  onSaved?: (record: EscalationRecord) => void;
  // Reports the loaded history so a parent can show a count in its header.
  onHistory?: (items: EscalationRecord[]) => void;
  // Rendered left of the save button (e.g. a skip link).
  aside?: ReactNode;
  // The parent may render the history itself (the wrap-up card does).
  showHistory?: boolean;
  pickerHeight?: number;
}) {
  const [catId, setCatId] = useState<number | null>(null);
  const [reasonId, setReasonId] = useState<number | null>(null);
  const [note, setNote] = useState("");
  const [history, setHistory] = useState<EscalationRecord[]>([]);
  const [status, setStatus] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  const [saving, setSaving] = useState(false);

  const loadHistory = useCallback(
    (n: string) => {
      const key = n.trim();
      if (!canSearch || !key) {
        setHistory([]);
        onHistory?.([]);
        return;
      }
      api
        .escalationHistory(key)
        .then((items) => {
          setHistory(items);
          onHistory?.(items);
        })
        .catch(() => {
          setHistory([]);
          onHistory?.([]);
        });
    },
    [canSearch, onHistory],
  );

  useEffect(() => {
    loadHistory(number);
  }, [number, loadHistory]);

  // A new number starts a clean form.
  useEffect(() => {
    setCatId(null);
    setReasonId(null);
    setNote("");
    setStatus(null);
  }, [number]);


  async function save() {
    if (!number.trim() || reasonId === null) {
      setStatus({ kind: "err", text: "Durum seçin." });
      return;
    }
    setSaving(true);
    setStatus(null);
    try {
      const rec = await api.logEscalation({ number: number.trim(), reasonId, note: note.trim() || undefined, callUuid });
      setNote("");
      setReasonId(null);
      setCatId(null);
      setStatus({ kind: "ok", text: "Eskalasyon kaydedildi." });
      loadHistory(number);
      onSaved?.(rec);
    } catch (e) {
      setStatus({ kind: "err", text: e instanceof ApiError ? e.message : "Kaydedilemedi." });
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      {showHistory && canSearch && history.length > 0 && (
        <div className="rounded-xl bg-muted/30 p-3">
          <p className="mb-2 text-xs font-semibold text-muted-foreground">Geçmiş görüşmeler</p>
          <ul className="max-h-44 space-y-2 overflow-y-auto">
            {history.map((h) => (
              <li key={h.id} className="rounded-lg bg-card px-3 py-2 text-sm ring-1 ring-border/50">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-medium">{h.agentName} görüştü</span>
                  <span className="text-xs text-muted-foreground">{h.createdAt}</span>
                </div>
                <div className="mt-1">
                  <Badge tone="amber">{h.categoryName}</Badge> <span className="text-muted-foreground">{h.reasonName}</span>
                </div>
                {h.note && <p className="mt-1 text-foreground/80">{h.note}</p>}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">Kategori ve durum</span>
        <EscalationPicker
          categories={categories}
          catId={catId}
          reasonId={reasonId}
          height={pickerHeight}
          onChange={(c, r) => {
            setCatId(c);
            setReasonId(r);
          }}
        />
      </div>

      {reasonId !== null && (
        <div className="space-y-1.5 animate-in fade-in slide-in-from-top-1 duration-200">
          <span className="text-xs font-medium text-muted-foreground">Not</span>
          <textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Görüşme notu (opsiyonel)"
            rows={3}
            className="w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2.5 text-sm outline-none focus-visible:border-ring/60 focus-visible:bg-card"
          />
        </div>
      )}

      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          {status && <span className={cn("text-sm", status.kind === "ok" ? "text-success" : "text-destructive")}>{status.text}</span>}
          {aside}
        </div>
        <Button className="h-11 px-6" onClick={save} disabled={saving || reasonId === null}>
          {saving ? "Kaydediliyor..." : "Eskalasyonu Kaydet"}
        </Button>
      </div>
    </div>
  );
}
