// Eskalasyonlar: the escalation records, newest first, in pages. What the
// page lists follows the permissions: escalation.list_all shows everyone's
// records, escalation.list_own only the viewer's own. A customer number
// narrows the list; with escalation.search that lookup always spans every
// agent, which is the same history the dashboard shows during a call.

import { useEffect, useMemo, useState } from "react";
import { ArrowUpDown, CalendarRange, CheckCircle2, ClipboardList, Search, TriangleAlert, Users, X, Zap } from "lucide-react";
import { ListRow, Toolbar } from "../components/ui/rows";
import { api, ApiError } from "../api/client";
import type { EscalationCategory, EscalationRecord } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { displayNumber } from "../softphone/dial";
import { Badge, Card, DateField, EmptyState, Pagination, Select, Skeleton } from "../components/ui";
import { cn } from "../lib/utils";

const PER_PAGE = 25;

function ymd(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

type Preset = "all" | "today" | "yesterday" | "last7" | "last30" | "month" | "custom";

const PRESETS: { key: Preset; label: string }[] = [
  { key: "all", label: "Tüm tarihler" },
  { key: "today", label: "Bugün" },
  { key: "yesterday", label: "Dün" },
  { key: "last7", label: "Son 7 gün" },
  { key: "last30", label: "Son 30 gün" },
  { key: "month", label: "Bu ay" },
  { key: "custom", label: "Tarih aralığı" },
];

function presetRange(key: Preset): { from: string; to: string } {
  const now = new Date();
  const today = ymd(now);
  const shift = (days: number) => ymd(new Date(now.getFullYear(), now.getMonth(), now.getDate() - days));
  switch (key) {
    case "today": return { from: today, to: today };
    case "yesterday": return { from: shift(1), to: shift(1) };
    case "last7": return { from: shift(6), to: today };
    case "last30": return { from: shift(29), to: today };
    case "month": return { from: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), to: today };
    default: return { from: "", to: "" };
  }
}

export function EscalationSearch() {
  const { user } = useAuth();
  const canAll = can(user, "escalation.list_all");
  const canOwn = can(user, "escalation.list_own");
  const canSearch = can(user, "escalation.search");
  // Without a list permission the page is search-only: nothing lists until a number is typed.
  const searchOnly = !canAll && !canOwn && canSearch;

  const [number, setNumber] = useState("");
  const [debounced, setDebounced] = useState("");
  const [preset, setPreset] = useState<Preset>("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [agentId, setAgentId] = useState<number>(0);
  const [categoryId, setCategoryId] = useState<number>(0);
  const [page, setPage] = useState(1);

  const [agents, setAgents] = useState<{ id: number; name: string }[]>([]);
  const [categories, setCategories] = useState<EscalationCategory[]>([]);
  const [items, setItems] = useState<EscalationRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [scope, setScope] = useState<"all" | "own">(canAll ? "all" : "own");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(number.trim()), 350);
    return () => window.clearTimeout(t);
  }, [number]);

  useEffect(() => {
    if (canAll) api.escalationAgents().then(setAgents).catch(() => setAgents([]));
    api.escalationCategories().then(setCategories).catch(() => setCategories([]));
  }, [canAll]);

  // Any filter change goes back to page one.
  useEffect(() => {
    setPage(1);
  }, [debounced, from, to, agentId, categoryId]);

  useEffect(() => {
    if (searchOnly && !debounced) {
      setItems([]);
      setTotal(0);
      setLoading(false);
      return;
    }
    if (preset === "custom" && from && to && to < from) return;
    let live = true;
    setLoading(true);
    api
      .listEscalations({ number: debounced || undefined, from: from || undefined, to: to || undefined, agentId: agentId || undefined, categoryId: categoryId || undefined, page, perPage: PER_PAGE })
      .then((r) => {
        if (!live) return;
        setItems(r.items);
        setTotal(r.total);
        setScope(r.scope);
        setError(null);
      })
      .catch((e) => {
        if (!live) return;
        setError(e instanceof ApiError ? e.message : "Kayıtlar alınamadı.");
        setItems([]);
        setTotal(0);
      })
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [debounced, from, to, agentId, categoryId, page, searchOnly, preset]);

  function choosePreset(key: Preset) {
    setPreset(key);
    if (key !== "custom") {
      const r = presetRange(key);
      setFrom(r.from);
      setTo(r.to);
    }
  }

  const activeFilters = useMemo(() => {
    const parts: string[] = [];
    if (debounced) parts.push(displayNumber(debounced));
    if (preset !== "all") parts.push(PRESETS.find((p) => p.key === preset)?.label ?? "");
    if (agentId) parts.push(agents.find((a) => a.id === agentId)?.name ?? "");
    if (categoryId) parts.push(categories.find((c) => c.id === categoryId)?.name ?? "");
    return parts.filter(Boolean);
  }, [debounced, preset, agentId, categoryId, agents, categories]);

  const scopeLabel = debounced && canSearch ? "Bu numara için tüm temsilciler" : scope === "all" ? "Tüm temsilciler" : "Sadece kendi kayıtların";

  return (
    <div className="space-y-4">
      <Card
        title="Eskalasyonlar"
        icon={ClipboardList}
        actions={
          <span className="hidden items-center gap-1.5 text-xs text-muted-foreground md:inline-flex">
            <Users className="size-3.5" />
            {scopeLabel}
          </span>
        }
      >
        {/* Filters */}
        <Toolbar>
          <div className="relative min-w-[16rem] flex-1">
            <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              value={number}
              onChange={(e) => setNumber(e.target.value)}
              placeholder="Müşteri numarası (örn. 05304230113)"
              inputMode="tel"
              autoFocus={searchOnly}
              className="h-10 w-full rounded-xl border border-border/70 bg-muted/40 pl-10 pr-9 text-sm outline-none transition placeholder:text-muted-foreground/60 hover:border-border focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20"
            />
            {number && (
              <button type="button" onClick={() => setNumber("")} aria-label="Numarayı temizle" className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground">
                <X className="size-3.5" />
              </button>
            )}
          </div>

          <label className="relative" title="Tarih">
            <CalendarRange className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Select value={preset} onChange={(e) => choosePreset(e.target.value as Preset)} className="h-10 w-40 pl-9">
              {PRESETS.map((p) => (
                <option key={p.key} value={p.key}>{p.label}</option>
              ))}
            </Select>
          </label>
          {preset === "custom" && (
            <div className="flex items-center gap-1">
              <DateField value={from} max={to || undefined} onChange={setFrom} className="w-40" title="Başlangıç" />
              <span className="text-muted-foreground">-</span>
              <DateField value={to} min={from || undefined} max={ymd(new Date())} onChange={setTo} className="w-40" title="Bitiş" />
            </div>
          )}

          {canAll && (
            <label className="relative" title="Temsilci">
              <Users className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Select value={agentId} onChange={(e) => setAgentId(Number(e.target.value))} className="h-10 w-48 pl-9">
                <option value={0}>Tüm temsilciler</option>
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>{a.name}</option>
                ))}
              </Select>
            </label>
          )}

          <label className="relative" title="Kategori">
            <ArrowUpDown className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Select value={categoryId} onChange={(e) => setCategoryId(Number(e.target.value))} className="h-10 w-52 pl-9">
              <option value={0}>Tüm kategoriler</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </Select>
          </label>
        </Toolbar>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>
            {loading ? "Yükleniyor..." : (
              <>
                <span className="font-medium text-foreground">{total}</span> kayıt
                {activeFilters.length > 0 && <> · {activeFilters.join(" · ")}</>}
              </>
            )}
          </span>
          <span className="md:hidden">{scopeLabel}</span>
        </div>

        {error && <p className="mt-3 text-sm text-destructive">{error}</p>}

        {/* Records */}
        <div className="mt-3">
          {loading && items.length === 0 ? (
            <div className="space-y-2">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-16 w-full rounded-xl" />
              ))}
            </div>
          ) : items.length === 0 ? (
            searchOnly && !debounced ? (
              <EmptyState title="Numara ile arayın" description="Bir müşteri numarası girerek o müşterinin geçmiş eskalasyonlarını görüntüleyin." />
            ) : (
              <EmptyState title="Kayıt yok" description={debounced ? `${displayNumber(debounced)} için eskalasyon kaydı bulunamadı.` : "Bu filtrelerle eşleşen eskalasyon kaydı yok."} />
            )
          ) : (
            <ul className={cn("space-y-1", loading && "opacity-60")}>
              {items.map((r) => {
                const none = r.categoryName === "Eskalasyon yok";
                const auto = r.note.startsWith("Kendiliğinden");
                return (
                  <li key={r.id}>
                    <ListRow
                      icon={none ? CheckCircle2 : auto ? Zap : TriangleAlert}
                      tone={none ? "muted" : auto ? "primary" : "warning"}
                      title={
                        <span className="flex items-center gap-2">
                          <button type="button" onClick={() => setNumber(r.number)} title="Bu numaranın kayıtlarını göster" className="font-mono tabular-nums tracking-wide hover:underline">{displayNumber(r.number) || r.number}</button>
                          <span className="text-xs font-normal text-muted-foreground">{r.agentName}</span>
                        </span>
                      }
                      sub={<span className="flex items-center gap-2"><Badge tone={none ? "slate" : "amber"}>{r.categoryName}</Badge><span>{r.reasonName}</span></span>}
                      trailing={<span className="font-mono text-xs tabular-nums text-muted-foreground">{r.createdAt}</span>}
                    >
                      {r.note && <p className="pl-11 text-sm text-foreground/80">{r.note}</p>}
                    </ListRow>
                  </li>
                );
              })}
            </ul>
          )}
          <Pagination page={page} perPage={PER_PAGE} total={total} onChange={setPage} />
        </div>
      </Card>
    </div>
  );
}
