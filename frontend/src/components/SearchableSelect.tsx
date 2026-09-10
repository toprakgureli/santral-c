import { useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Search } from "lucide-react";
import { cn } from "@/lib/utils";

export interface Option {
  id: number;
  label: string;
  hint?: string;
}

// SearchableSelect is a large combobox: a button that opens a panel with a
// search box and a filtered option list. Used for the escalation category and
// reason pickers.
export function SearchableSelect({
  value,
  onChange,
  options,
  placeholder = "Seçin",
  searchPlaceholder = "Ara...",
  disabled,
  size = "md",
}: {
  value: number | null;
  onChange: (id: number) => void;
  options: Option[];
  placeholder?: string;
  searchPlaceholder?: string;
  disabled?: boolean;
  size?: "md" | "lg";
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
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

  const selected = options.find((o) => o.id === value) ?? null;
  const filtered = useMemo(() => {
    const q = query.trim().toLocaleLowerCase("tr");
    if (!q) return options;
    return options.filter((o) => o.label.toLocaleLowerCase("tr").includes(q) || o.hint?.toLocaleLowerCase("tr").includes(q));
  }, [options, query]);

  const trigger = size === "lg" ? "h-12 text-base" : "h-10 text-sm";

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "flex w-full items-center justify-between gap-2 rounded-xl border border-border/70 bg-muted/40 px-3.5 text-left outline-none transition",
          "hover:border-border hover:bg-muted/60 focus-visible:border-ring/60 focus-visible:ring-4 focus-visible:ring-ring/20",
          "disabled:pointer-events-none disabled:opacity-50",
          trigger,
        )}
      >
        <span className={cn("truncate", !selected && "text-muted-foreground/70")}>{selected ? selected.label : placeholder}</span>
        <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div className="absolute z-30 mt-1.5 w-full overflow-hidden rounded-xl border border-border bg-card shadow-xl">
          <div className="relative border-b border-border/60 p-2">
            <Search className="pointer-events-none absolute left-4 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={searchPlaceholder}
              className="h-9 w-full rounded-lg bg-muted/50 pl-8 pr-3 text-sm outline-none placeholder:text-muted-foreground/60"
            />
          </div>
          <ul className="max-h-64 overflow-y-auto p-1">
            {filtered.map((o) => (
              <li key={o.id}>
                <button
                  type="button"
                  onClick={() => { onChange(o.id); setOpen(false); setQuery(""); }}
                  className={cn(
                    "flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm transition hover:bg-accent",
                    o.id === value && "bg-accent/60",
                  )}
                >
                  <span className="min-w-0">
                    <span className="block truncate">{o.label}</span>
                    {o.hint && <span className="block truncate text-xs text-muted-foreground">{o.hint}</span>}
                  </span>
                  {o.id === value && <Check className="size-4 shrink-0 text-primary" />}
                </button>
              </li>
            ))}
            {filtered.length === 0 && <li className="px-3 py-4 text-center text-sm text-muted-foreground">Sonuç yok.</li>}
          </ul>
        </div>
      )}
    </div>
  );
}
