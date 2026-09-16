// EscalationPicker is a two-pane disposition picker: categories on the left,
// the chosen category's reasons on the right, one search box over both.
// Everything stays inside the card (no floating panels), so it fits in the
// wrap-up card and the dashboard column alike. Typing filters both panes and
// jumps to the first category that still has a match.

import { useEffect, useMemo, useState } from "react";
import { Check, ChevronRight, Search, X } from "lucide-react";
import type { EscalationCategory } from "@/api/types";
import { cn } from "@/lib/utils";

export function EscalationPicker({
  categories,
  catId,
  reasonId,
  onChange,
  height = 260,
}: {
  categories: EscalationCategory[];
  catId: number | null;
  reasonId: number | null;
  onChange: (catId: number | null, reasonId: number | null) => void;
  height?: number;
}) {
  const [query, setQuery] = useState("");
  const q = query.trim().toLocaleLowerCase("tr");
  const hit = (s: string) => s.toLocaleLowerCase("tr").includes(q);

  // Categories that match themselves or hold a matching reason; the reason
  // list inside each is narrowed to the matches when a category itself does
  // not match.
  const visible = useMemo(() => {
    if (!q) return categories.map((c) => ({ ...c, matchSelf: false }));
    return categories
      .map((c) => {
        const matchSelf = hit(c.name);
        const reasons = matchSelf ? c.reasons : c.reasons.filter((r) => hit(r.name));
        return { ...c, reasons, matchSelf };
      })
      .filter((c) => c.matchSelf || c.reasons.length > 0);
  }, [categories, q]); // eslint-disable-line react-hooks/exhaustive-deps

  // Keep a valid selection while filtering: jump to the first visible
  // category when the current one disappears.
  useEffect(() => {
    if (visible.length === 0) return;
    if (!visible.some((c) => c.id === catId)) onChange(visible[0].id, null);
  }, [visible, catId]); // eslint-disable-line react-hooks/exhaustive-deps

  const current = visible.find((c) => c.id === catId) ?? null;
  const selectedCat = categories.find((c) => c.id === catId);
  const selectedReason = selectedCat?.reasons.find((r) => r.id === reasonId);

  return (
    <div className="space-y-2">
      <div className="relative">
        <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Kategori veya durum ara..."
          className="h-10 w-full rounded-xl border border-border/70 bg-muted/40 pl-10 pr-9 text-sm outline-none transition placeholder:text-muted-foreground/60 hover:border-border focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
        />
        {query && (
          <button type="button" onClick={() => setQuery("")} aria-label="Aramayı temizle" className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground">
            <X className="size-3.5" />
          </button>
        )}
      </div>

      <div className="grid grid-cols-[minmax(0,5fr)_minmax(0,7fr)] overflow-hidden rounded-xl border border-border/70 bg-muted/20" style={{ height }}>
        {/* Categories */}
        <div className="flex min-h-0 flex-col border-r border-border/60">
          <div className="border-b border-border/60 px-3 py-1.5 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">Kategori</div>
          <ul className="min-h-0 flex-1 overflow-y-auto p-1">
            {visible.map((c) => {
              const active = c.id === catId;
              return (
                <li key={c.id}>
                  <button
                    type="button"
                    onClick={() => onChange(c.id, c.id === catId ? reasonId : null)}
                    className={cn(
                      "flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm transition",
                      active ? "bg-primary/10 font-medium text-primary" : "hover:bg-accent",
                    )}
                  >
                    <span className="truncate">{c.name}</span>
                    <span className={cn("flex shrink-0 items-center gap-1 text-xs", active ? "text-primary/80" : "text-muted-foreground")}>
                      {c.reasons.length}
                      <ChevronRight className="size-3.5" />
                    </span>
                  </button>
                </li>
              );
            })}
            {visible.length === 0 && <li className="px-3 py-6 text-center text-xs text-muted-foreground">Eşleşen kategori yok.</li>}
          </ul>
        </div>

        {/* Reasons of the chosen category */}
        <div className="flex min-h-0 flex-col">
          <div className="truncate border-b border-border/60 px-3 py-1.5 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">
            {current ? `Durum · ${current.name}` : "Durum"}
          </div>
          <ul className="min-h-0 flex-1 overflow-y-auto p-1">
            {current?.reasons.map((r) => {
              const active = r.id === reasonId;
              return (
                <li key={r.id}>
                  <button
                    type="button"
                    onClick={() => onChange(current.id, r.id)}
                    className={cn(
                      "flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left text-sm transition",
                      active ? "bg-success/10 font-medium ring-1 ring-success/40" : "hover:bg-accent",
                    )}
                  >
                    <span className="min-w-0 break-words">{r.name}</span>
                    {active && <Check className="size-4 shrink-0 text-success" />}
                  </button>
                </li>
              );
            })}
            {!current && <li className="px-3 py-6 text-center text-xs text-muted-foreground">Önce soldan bir kategori seçin.</li>}
            {current && current.reasons.length === 0 && <li className="px-3 py-6 text-center text-xs text-muted-foreground">Bu kategoride durum yok.</li>}
          </ul>
        </div>
      </div>

      {/* What is chosen, in one line */}
      <div className={cn("flex min-h-8 items-center gap-1.5 text-sm", selectedReason ? "text-foreground" : "text-muted-foreground")}>
        {selectedReason ? (
          <>
            <Check className="size-4 shrink-0 text-success" />
            <span className="truncate">
              <span className="font-medium">{selectedCat?.name}</span>
              <span className="mx-1.5 text-muted-foreground">›</span>
              {selectedReason.name}
            </span>
          </>
        ) : (
          <span className="text-xs">Kategoriyi, sonra durumu seçin.</span>
        )}
      </div>
    </div>
  );
}
