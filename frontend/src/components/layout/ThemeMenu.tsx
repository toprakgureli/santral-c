// ThemeMenu is the theme picker: a palette button that opens a list of the
// five themes, each with a three-colour swatch (background, card, primary)
// so the choice can be read before it is made.

import { useEffect, useRef, useState } from "react";
import { Check, Palette } from "lucide-react";
import { useTheme } from "@/contexts/ThemeContext";
import { cn } from "@/lib/utils";

export default function ThemeMenu({ align = "right" }: { align?: "left" | "right" }) {
  const { theme, themes, setTheme, info } = useTheme();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label="Tema seç"
        aria-haspopup="menu"
        aria-expanded={open}
        data-tip={`Tema: ${info.label}`}
        className={cn("flex size-10 items-center justify-center rounded-xl text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", open && "bg-accent text-foreground")}
      >
        <Palette className="size-4" />
      </button>

      {open && (
        <div
          role="menu"
          className={cn(
            "animate-in fade-in slide-in-from-top-1 absolute z-50 mt-2 w-60 overflow-hidden rounded-xl border border-border bg-popover p-1 text-popover-foreground shadow-lg duration-150",
            align === "right" ? "right-0" : "left-0",
          )}
        >
          <div className="px-3 pt-2 pb-1 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">Tema</div>
          {themes.map((t) => {
            const active = t.id === theme;
            return (
              <button
                key={t.id}
                type="button"
                role="menuitemradio"
                aria-checked={active}
                onClick={() => {
                  setTheme(t.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-sm transition-colors hover:bg-accent",
                  active && "bg-accent/70",
                )}
              >
                <span className="flex shrink-0 -space-x-1.5" aria-hidden="true">
                  {t.swatch.map((c, i) => (
                    <span key={i} className="size-4 rounded-full ring-2 ring-popover" style={{ backgroundColor: c }} />
                  ))}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block font-medium leading-tight">{t.label}</span>
                  <span className="block truncate text-xs text-muted-foreground">{t.hint}</span>
                </span>
                {active && <Check className="size-4 shrink-0 text-success" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
