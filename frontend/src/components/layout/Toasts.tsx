// Toasts: short cards at the top right that tell the person something new
// happened (a notice, a live alert for the team). Anything in the panel
// announces one with pushToast; one stack shows them, so two sources never
// cover each other. A toast goes away on its own after a while or when
// closed, and opens its link when clicked.

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Bell, Siren, X } from "lucide-react";
import { cn } from "@/lib/utils";

export interface Toast {
  id: string;
  text: string;
  link?: string;
  tone?: "notice" | "alert";
  onOpen?: () => void;
}

const EVENT = "santral:toast";
const SHOW_MS = 12000;
const MOST = 4;

export function pushToast(t: Toast) {
  window.dispatchEvent(new CustomEvent<Toast>(EVENT, { detail: t }));
}

export default function Toasts() {
  const navigate = useNavigate();
  const [items, setItems] = useState<Toast[]>([]);

  useEffect(() => {
    const on = (e: Event) => {
      const t = (e as CustomEvent<Toast>).detail;
      setItems((cur) => [t, ...cur.filter((x) => x.id !== t.id)].slice(0, MOST));
    };
    window.addEventListener(EVENT, on);
    return () => window.removeEventListener(EVENT, on);
  }, []);

  useEffect(() => {
    if (items.length === 0) return;
    const t = window.setTimeout(() => setItems((cur) => cur.slice(0, -1)), SHOW_MS);
    return () => window.clearTimeout(t);
  }, [items]);

  if (items.length === 0) return null;
  const close = (id: string) => setItems((cur) => cur.filter((x) => x.id !== id));
  return (
    <div className="fixed top-20 right-4 z-[72] flex w-[22rem] max-w-[calc(100vw-2rem)] flex-col gap-2">
      {items.map((t) => {
        const Icon = t.tone === "alert" ? Siren : Bell;
        return (
          <div key={t.id} role="status" className="animate-in fade-in slide-in-from-right-4 flex items-start gap-2.5 rounded-xl border border-border bg-popover p-3 text-popover-foreground shadow-xl duration-200">
            <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-lg", t.tone === "alert" ? "bg-warning/14 text-warning" : "bg-primary/10 text-primary")}>
              <Icon className="size-4" />
            </span>
            <button
              type="button"
              onClick={() => {
                close(t.id);
                t.onOpen?.();
                if (t.link) navigate(t.link);
              }}
              className="min-w-0 flex-1 text-left text-sm leading-snug"
            >
              {t.text}
            </button>
            <button type="button" onClick={() => close(t.id)} aria-label="Kapat" className="flex size-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent">
              <X className="size-4" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
