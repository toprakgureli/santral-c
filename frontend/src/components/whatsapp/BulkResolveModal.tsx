import { useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Bot, Check, CheckCheck, ChevronDown, Clock, Filter, Hourglass, Lock, Search, Sparkles, UserCheck, Users, X } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import UserAvatar from "@/components/ui/UserAvatar";
import { useTopmost } from "@/components/ui/windowStack";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAAgent, WAChannel, WAConversation, WAPerson, WATicketStatus } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { isWaiting, prettyPhone, STATUS_WORD } from "@/whatsapp/util";

type TimeBasis = "lastMessage" | "ticketAge" | "awaiting";
type WaitingFilter = "all" | "waiting" | "replied";

const HOUR_PRESETS = [
  { hours: 0, label: "Tümü" },
  { hours: 12, label: "12 sa+" },
  { hours: 24, label: "24 sa+" },
  { hours: 48, label: "48 sa+" },
  { hours: 50, label: "50 sa+" },
  { hours: 72, label: "72 sa+" },
  { hours: 168, label: "7 gün+" },
];

const STATUS_OPTIONS: { key: Exclude<WATicketStatus, "resolved">; label: string; color: string }[] = [
  { key: "open", label: "Açık", color: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30" },
  { key: "pending", label: "Müşteri Bekleniyor", color: "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30" },
  { key: "bot", label: "Chatbot'ta", color: "bg-violet-500/15 text-violet-600 dark:text-violet-400 border-violet-500/30" },
];

function elapsedHours(c: WAConversation, basis: TimeBasis, now: number): number {
  let iso: string | undefined;
  switch (basis) {
    case "lastMessage":
      iso = c.last?.at ?? c.ticket?.createdAt;
      break;
    case "ticketAge":
      iso = c.ticket?.createdAt ?? c.last?.at;
      break;
    case "awaiting":
      iso = c.ticket?.awaitingSince;
      break;
  }
  if (!iso) return 0;
  const ms = now - Date.parse(iso);
  return ms > 0 ? ms / 3600000 : 0;
}

function formatHours(h: number): string {
  if (h < 1) {
    const mins = Math.max(1, Math.floor(h * 60));
    return `${mins} dk`;
  }
  if (h < 48) {
    return `${Math.floor(h)} sa`;
  }
  const days = Math.floor(h / 24);
  const remHours = Math.floor(h % 24);
  return remHours > 0 ? `${days} gün ${remHours} sa` : `${days} gün`;
}

export default function BulkResolveModal({
  open,
  channels,
  onClose,
}: {
  open: boolean;
  channels: WAChannel[];
  onClose: () => void;
}) {
  const { user } = useAuth();
  const wa = useWhatsApp();
  const canAll = can(user, "whatsapp.bulk_resolve_all");
  const canOwn = canAll || can(user, "whatsapp.bulk_resolve_own");

  const [agents, setAgents] = useState<WAAgent[]>([]);
  const [serverCandidates, setServerCandidates] = useState<WAConversation[]>([]);
  const [loadingCandidates, setLoadingCandidates] = useState(false);

  // Filters
  // ownerSelection: empty set when canAll means "All owners + pool"; otherwise specific keys ("pool" or String(userId))
  const [allOwners, setAllOwners] = useState<boolean>(true);
  const [selectedOwners, setSelectedOwners] = useState<Set<string>>(() => new Set([String(wa.me)]));
  const [comboOpen, setComboOpen] = useState(false);
  const [agentQuery, setAgentQuery] = useState("");
  const comboRef = useRef<HTMLDivElement>(null);

  const [statuses, setStatuses] = useState<Set<Exclude<WATicketStatus, "resolved">>>(() => new Set(["open", "pending"]));
  const [waitingFilter, setWaitingFilter] = useState<WaitingFilter>("all");
  const [timeBasis, setTimeBasis] = useState<TimeBasis>("lastMessage");
  const [minHoursInput, setMinHoursInput] = useState<string>("24");
  const [channelId, setChannelId] = useState<number>(0);
  const [searchQuery, setSearchQuery] = useState("");

  // Selection & submission
  const [excludedIds, setExcludedIds] = useState<Set<number>>(() => new Set());
  const [sendSurvey, setSendSurvey] = useState(false);
  const [note, setNote] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [resultMsg, setResultMsg] = useState("");
  const [now, setNow] = useState(() => Date.now());

  const isTop = useTopmost(open);

  useEffect(() => {
    if (!open) return;
    setNow(Date.now());
    setError("");
    setResultMsg("");
    setExcludedIds(new Set());
    if (!canAll) {
      setAllOwners(false);
      setSelectedOwners(new Set([String(wa.me)]));
    }
    setLoadingCandidates(true);
    waApi
      .bulkCandidates()
      .then((list) => setServerCandidates(list))
      .catch(() => setServerCandidates([]))
      .finally(() => setLoadingCandidates(false));
    if (canAll) {
      waApi
        .agents()
        .then(setAgents)
        .catch(() => setAgents([]));
    }
  }, [open, canAll, wa.me]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isTop()) {
        if (comboOpen) setComboOpen(false);
        else onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, isTop, comboOpen, onClose]);

  useEffect(() => {
    if (!comboOpen) return;
    const onDown = (e: MouseEvent) => {
      if (!comboRef.current?.contains(e.target as Node)) setComboOpen(false);
    };
    window.addEventListener("mousedown", onDown);
    return () => window.removeEventListener("mousedown", onDown);
  }, [comboOpen]);

  // Merge live conversations from context with server candidates so the list is always complete and fresh.
  const allUnresolved = useMemo(() => {
    const map = new Map<number, WAConversation>();
    for (const c of serverCandidates) {
      if (c.ticket && c.ticket.status !== "resolved") map.set(c.id, c);
    }
    for (const c of wa.conversations) {
      if (c.ticket && c.ticket.status !== "resolved") {
        map.set(c.id, c);
      } else if (c.ticket?.status === "resolved") {
        map.delete(c.id);
      }
    }
    return [...map.values()];
  }, [serverCandidates, wa.conversations]);

  // Build agent options with their open ticket counts
  const ownerOptions = useMemo(() => {
    const counts = new Map<string, number>();
    const peopleMap = new Map<number, WAPerson>();
    for (const a of agents) {
      peopleMap.set(a.id, a);
    }
    for (const c of allUnresolved) {
      const owner = c.ticket?.owner;
      const key = owner ? String(owner.id) : "pool";
      counts.set(key, (counts.get(key) ?? 0) + 1);
      if (owner && !peopleMap.has(owner.id)) {
        peopleMap.set(owner.id, owner);
      }
    }
    const list = [...peopleMap.values()].sort((a, b) => {
      if (a.id === wa.me) return -1;
      if (b.id === wa.me) return 1;
      return (counts.get(String(b.id)) ?? 0) - (counts.get(String(a.id)) ?? 0) || a.name.localeCompare(b.name, "tr");
    });
    return {
      poolCount: counts.get("pool") ?? 0,
      mineCount: counts.get(String(wa.me)) ?? 0,
      counts,
      people: list,
    };
  }, [agents, allUnresolved, wa.me]);

  const filteredPeople = useMemo(() => {
    const q = agentQuery.trim().toLocaleLowerCase("tr");
    if (!q) return ownerOptions.people;
    return ownerOptions.people.filter((p) => p.name.toLocaleLowerCase("tr").includes(q));
  }, [ownerOptions.people, agentQuery]);

  const minHours = useMemo(() => {
    const n = Number(minHoursInput.replace(",", "."));
    return Number.isFinite(n) && n >= 0 ? n : 0;
  }, [minHoursInput]);

  // Reset excludedIds whenever filter criteria change so newly filtered results start selected
  useEffect(() => {
    setExcludedIds(new Set());
  }, [allOwners, selectedOwners, statuses, waitingFilter, timeBasis, minHours, channelId, searchQuery]);

  const matchingConversations = useMemo(() => {
    const needle = searchQuery.trim().toLocaleLowerCase("tr");
    const digits = searchQuery.replace(/\D/g, "");
    return allUnresolved
      .filter((c) => {
        const t = c.ticket;
        if (!t || t.status === "resolved") return false;
        if (channelId && c.channelId !== channelId) return false;

        // Permission & Owner check
        if (!canAll) {
          if (t.owner?.id !== wa.me) return false;
        } else if (!allOwners) {
          const key = t.owner ? String(t.owner.id) : "pool";
          if (!selectedOwners.has(key)) return false;
        }

        // Status check
        if (!statuses.has(t.status)) return false;

        // Waiting filter check
        if (waitingFilter === "waiting" && !t.awaitingSince && !isWaiting(c)) return false;
        if (waitingFilter === "replied" && (t.awaitingSince || isWaiting(c))) return false;

        // Elapsed time check
        if (minHours > 0) {
          if (timeBasis === "awaiting" && !t.awaitingSince) return false;
          const h = elapsedHours(c, timeBasis, now);
          if (h < minHours) return false;
        }

        // Text search check
        if (needle) {
          const matchName = c.contact.display.toLocaleLowerCase("tr").includes(needle);
          const matchPhone = digits.length >= 3 && c.contact.waId.includes(digits);
          const matchTag = c.contact.tags.some((tag) => tag.toLocaleLowerCase("tr").includes(needle));
          const matchNum = String(t.number) === needle.replace("#", "");
          if (!matchName && !matchPhone && !matchTag && !matchNum) return false;
        }

        return true;
      })
      .sort((a, b) => elapsedHours(b, timeBasis, now) - elapsedHours(a, timeBasis, now));
  }, [allUnresolved, channelId, canAll, wa.me, allOwners, selectedOwners, statuses, waitingFilter, minHours, timeBasis, now, searchQuery]);

  const selectedIds = useMemo(
    () => matchingConversations.filter((c) => !excludedIds.has(c.id)).map((c) => c.id),
    [matchingConversations, excludedIds],
  );

  if (!open || !canOwn) return null;

  const toggleStatus = (key: Exclude<WATicketStatus, "resolved">) => {
    setStatuses((cur) => {
      const next = new Set(cur);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleOwnerKey = (key: string) => {
    setAllOwners(false);
    setSelectedOwners((cur) => {
      const next = allOwners ? new Set<string>() : new Set(cur);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleRow = (id: number) => {
    setExcludedIds((cur) => {
      const next = new Set(cur);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selectedIds.length === matchingConversations.length) {
      setExcludedIds(new Set(matchingConversations.map((c) => c.id)));
    } else {
      setExcludedIds(new Set());
    }
  };

  const ownerSummaryLabel = () => {
    if (!canAll) return "Sadece Benim Sohbetlerim";
    if (allOwners) return `Tüm Sorumlular ve Havuz (${allUnresolved.length})`;
    if (selectedOwners.size === 0) return "Sorumlu seçilmedi";
    if (selectedOwners.size === 1) {
      if (selectedOwners.has("pool")) return `Havuzdakiler / Sahipsiz (${ownerOptions.poolCount})`;
      if (selectedOwners.has(String(wa.me))) return `Sadece Ben (${ownerOptions.mineCount})`;
      const id = Number([...selectedOwners][0]);
      const p = ownerOptions.people.find((x) => x.id === id);
      return p ? `${p.name} (${ownerOptions.counts.get(String(id)) ?? 0})` : "1 temsilci seçili";
    }
    return `${selectedOwners.size} sorumlu filtresi seçili`;
  };

  const submitBulkResolve = async () => {
    if (selectedIds.length === 0 || submitting) return;
    setSubmitting(true);
    setError("");
    setResultMsg("");
    try {
      const res = await waApi.bulkResolve({
        conversationIds: selectedIds,
        sendSurvey,
        note: note.trim() || undefined,
      });
      setServerCandidates((cur) => cur.filter((c) => !res.ids.includes(c.id)));
      void wa.reload();
      setResultMsg(
        res.skipped > 0
          ? `${res.resolved} sohbet çözüldü olarak kapatıldı (${res.skipped} sohbet atlandı).`
          : `${res.resolved} sohbet başarıyla çözüldü olarak kapatıldı.`,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Toplu çözümleme sırasında bir hata oluştu.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-3 backdrop-blur-xs md:p-6"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="animate-in fade-in zoom-in-95 flex max-h-[92svh] w-full max-w-4xl flex-col overflow-hidden rounded-3xl border border-border bg-card text-card-foreground shadow-2xl duration-150">
        {/* Header */}
        <div className="flex shrink-0 items-center justify-between border-b border-border/60 px-6 py-4">
          <div className="flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-2xl bg-wa-accent/15 text-wa-accent">
              <CheckCheck className="size-5" />
            </span>
            <div>
              <h2 className="text-base font-bold tracking-tight">Toplu Çözümleme Sistemi</h2>
              <p className="text-xs text-muted-foreground">
                {canAll
                  ? "Temsilci, statü ve geçen süre kriterlerine göre açık talepleri filtreleyip toplu olarak kapatın."
                  : "Sorumlu olduğunuz açık talepleri statü ve geçen süre kriterlerine göre filtreleyip toplu olarak kapatın."}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Kapat"
            className="flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Filters Panel */}
        <div className="shrink-0 space-y-4 border-b border-border/60 bg-muted/25 px-6 py-4">
          <div className="grid gap-3 md:grid-cols-2">
            {/* 1. Agent / Owner Multi-Select Combobox */}
            <div className="space-y-1.5" ref={comboRef}>
              <label className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                <Users className="size-3.5" />
                Temsilci / Sorumlu Seçimi
              </label>
              {canAll ? (
                <div className="relative">
                  <button
                    type="button"
                    onClick={() => setComboOpen((v) => !v)}
                    className="flex h-10 w-full items-center justify-between rounded-xl border border-border bg-card px-3.5 text-left text-sm font-medium transition-colors hover:border-wa-accent/50"
                  >
                    <span className="truncate">{ownerSummaryLabel()}</span>
                    <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", comboOpen && "rotate-180")} />
                  </button>

                  {comboOpen && (
                    <div className="animate-in fade-in zoom-in-95 absolute left-0 right-0 top-full z-40 mt-1.5 flex max-h-80 flex-col overflow-hidden rounded-2xl border border-border bg-popover text-popover-foreground shadow-xl duration-100">
                      <div className="border-b border-border/60 p-2">
                        <div className="relative">
                          <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                          <input
                            value={agentQuery}
                            onChange={(e) => setAgentQuery(e.target.value)}
                            placeholder="Temsilci ara..."
                            className="h-8 w-full rounded-lg bg-muted/70 pl-8 pr-3 text-xs outline-none focus:ring-1 focus:ring-wa-accent"
                          />
                        </div>
                        <div className="mt-2 flex flex-wrap gap-1">
                          <button
                            type="button"
                            onClick={() => {
                              setAllOwners(true);
                              setSelectedOwners(new Set());
                            }}
                            className={cn(
                              "rounded-lg px-2 py-1 text-[0.7rem] font-semibold transition-colors",
                              allOwners ? "bg-wa-accent text-wa-on-accent" : "bg-muted text-muted-foreground hover:text-foreground",
                            )}
                          >
                            Tümünü Seç
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setAllOwners(false);
                              setSelectedOwners(new Set([String(wa.me)]));
                            }}
                            className={cn(
                              "rounded-lg px-2 py-1 text-[0.7rem] font-semibold transition-colors",
                              !allOwners && selectedOwners.size === 1 && selectedOwners.has(String(wa.me))
                                ? "bg-wa-accent text-wa-on-accent"
                                : "bg-muted text-muted-foreground hover:text-foreground",
                            )}
                          >
                            Sadece Ben
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setAllOwners(false);
                              setSelectedOwners(new Set(["pool"]));
                            }}
                            className={cn(
                              "rounded-lg px-2 py-1 text-[0.7rem] font-semibold transition-colors",
                              !allOwners && selectedOwners.size === 1 && selectedOwners.has("pool")
                                ? "bg-wa-accent text-wa-on-accent"
                                : "bg-muted text-muted-foreground hover:text-foreground",
                            )}
                          >
                            Sadece Havuz
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setAllOwners(false);
                              setSelectedOwners(new Set());
                            }}
                            className="rounded-lg bg-muted px-2 py-1 text-[0.7rem] font-semibold text-muted-foreground transition-colors hover:text-foreground"
                          >
                            Temizle
                          </button>
                        </div>
                      </div>

                      <div className="overflow-y-auto p-1.5">
                        {/* Pool option */}
                        <button
                          type="button"
                          onClick={() => toggleOwnerKey("pool")}
                          className="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-xs transition-colors hover:bg-accent"
                        >
                          <span
                            className={cn(
                              "flex size-4 shrink-0 items-center justify-center rounded border",
                              allOwners || selectedOwners.has("pool")
                                ? "border-wa-accent bg-wa-accent text-wa-on-accent"
                                : "border-border bg-card",
                            )}
                          >
                            {(allOwners || selectedOwners.has("pool")) && <Check className="size-3" />}
                          </span>
                          <span className="flex-1 font-medium text-amber-600 dark:text-amber-400">Havuzdakiler (Sahipsiz)</span>
                          <span className="rounded-full bg-muted px-2 py-0.5 text-[0.68rem] font-semibold tabular-nums text-muted-foreground">
                            {ownerOptions.poolCount}
                          </span>
                        </button>

                        {/* Agents */}
                        {filteredPeople.map((p) => {
                          const key = String(p.id);
                          const checked = allOwners || selectedOwners.has(key);
                          const count = ownerOptions.counts.get(key) ?? 0;
                          return (
                            <button
                              key={p.id}
                              type="button"
                              onClick={() => toggleOwnerKey(key)}
                              className="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-accent"
                            >
                              <span
                                className={cn(
                                  "flex size-4 shrink-0 items-center justify-center rounded border",
                                  checked ? "border-wa-accent bg-wa-accent text-wa-on-accent" : "border-border bg-card",
                                )}
                              >
                                {checked && <Check className="size-3" />}
                              </span>
                              <UserAvatar
                                userId={p.id}
                                name={p.name}
                                hasAvatar={p.hasAvatar}
                                version={p.avatarVersion}
                                className="size-6 shrink-0"
                                fallbackClassName="bg-primary/15 text-[0.55rem] text-primary"
                              />
                              <span className="min-w-0 flex-1 truncate font-medium">
                                {p.name} {p.id === wa.me && <span className="text-wa-accent">(Sen)</span>}
                              </span>
                              <span className="rounded-full bg-muted px-2 py-0.5 text-[0.68rem] font-semibold tabular-nums text-muted-foreground">
                                {count}
                              </span>
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>
              ) : (
                <div className="flex h-10 items-center gap-2 rounded-xl border border-border/70 bg-muted/60 px-3.5 text-xs font-medium text-muted-foreground">
                  <Lock className="size-3.5 shrink-0 text-wa-accent" />
                  <span className="truncate">Sadece Benim Sorumlu Olduğum Sohbetler ({ownerOptions.mineCount})</span>
                </div>
              )}
            </div>

            {/* 2. Elapsed Time Filter */}
            <div className="space-y-1.5">
              <div className="flex items-center justify-between gap-2">
                <label className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                  <Clock className="size-3.5" />
                  Geçen Süre Kriteri (Saat)
                </label>
                <select
                  value={timeBasis}
                  onChange={(e) => setTimeBasis(e.target.value as TimeBasis)}
                  className="rounded-lg border border-border bg-card px-2 py-0.5 text-[0.7rem] font-medium outline-none"
                >
                  <option value="lastMessage">Son mesajdan itibaren</option>
                  <option value="ticketAge">Talep açılışından itibaren</option>
                  <option value="awaiting">Cevap beklediği süreden itibaren</option>
                </select>
              </div>
              <div className="flex items-center gap-2">
                <div className="relative w-28 shrink-0">
                  <input
                    type="number"
                    min={0}
                    step={1}
                    value={minHoursInput}
                    onChange={(e) => setMinHoursInput(e.target.value)}
                    placeholder="Örn: 50"
                    className="h-10 w-full rounded-xl border border-border bg-card pl-3 pr-10 text-sm font-semibold tabular-nums outline-none focus:border-wa-accent"
                  />
                  <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">saat</span>
                </div>
                <div className="flex flex-1 flex-wrap gap-1">
                  {HOUR_PRESETS.map((p) => {
                    const active = minHours === p.hours;
                    return (
                      <button
                        key={p.hours}
                        type="button"
                        onClick={() => setMinHoursInput(String(p.hours))}
                        className={cn(
                          "rounded-lg px-2 py-1 text-[0.72rem] font-medium transition-colors",
                          active
                            ? "bg-wa-accent text-wa-on-accent"
                            : "border border-border/70 bg-card text-muted-foreground hover:text-foreground",
                        )}
                      >
                        {p.label}
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>
          </div>

          {/* 3. Statuses, Waiting Filter, Channel & Search */}
          <div className="grid gap-3 md:grid-cols-12">
            <div className="space-y-1.5 md:col-span-5">
              <label className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                <Filter className="size-3.5" />
                Talep Statüleri
              </label>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_OPTIONS.map((st) => {
                  const on = statuses.has(st.key);
                  return (
                    <button
                      key={st.key}
                      type="button"
                      onClick={() => toggleStatus(st.key)}
                      className={cn(
                        "flex h-9 items-center gap-1.5 rounded-xl border px-3 text-xs font-medium transition-all",
                        on ? st.color : "border-border bg-card text-muted-foreground opacity-65 hover:opacity-100",
                      )}
                    >
                      <span
                        className={cn(
                          "flex size-3.5 items-center justify-center rounded border",
                          on ? "border-current bg-current/20" : "border-muted-foreground/40",
                        )}
                      >
                        {on && <Check className="size-2.5" />}
                      </span>
                      {st.label}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="space-y-1.5 md:col-span-3">
              <label className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                <Hourglass className="size-3.5" />
                Cevap Durumu
              </label>
              <select
                value={waitingFilter}
                onChange={(e) => setWaitingFilter(e.target.value as WaitingFilter)}
                className="h-9 w-full rounded-xl border border-border bg-card px-3 text-xs font-medium outline-none"
              >
                <option value="all">Tümü (Bekleyen + Yazılan)</option>
                <option value="waiting">Müşteri Cevap Bekleyenler</option>
                <option value="replied">Son Mesajı Bizden Olanlar</option>
              </select>
            </div>

            <div className="space-y-1.5 md:col-span-4">
              <label className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                <Search className="size-3.5" />
                {channels.length > 1 ? "Hat & Müşteri Filtresi" : "Müşteri / Talep No Ara"}
              </label>
              <div className="flex gap-1.5">
                {channels.length > 1 && (
                  <select
                    value={channelId}
                    onChange={(e) => setChannelId(Number(e.target.value))}
                    className="h-9 max-w-[8.5rem] shrink-0 rounded-xl border border-border bg-card px-2.5 text-xs font-medium outline-none"
                  >
                    <option value={0}>Tüm hatlar</option>
                    {channels.map((ch) => (
                      <option key={ch.id} value={ch.id}>
                        {ch.name}
                      </option>
                    ))}
                  </select>
                )}
                <input
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="İsim, numara, #talep..."
                  className="h-9 min-w-0 flex-1 rounded-xl border border-border bg-card px-3 text-xs outline-none focus:border-wa-accent"
                />
              </div>
            </div>
          </div>
        </div>

        {/* Preview List Header */}
        <div className="flex shrink-0 items-center justify-between border-b border-border/60 bg-card px-6 py-2.5 text-xs">
          <div className="flex items-center gap-2">
            <span className="font-semibold">
              Filtreye Uyan: <span className="text-wa-accent">{matchingConversations.length}</span> sohbet
            </span>
            <span className="text-muted-foreground">·</span>
            <span className="text-muted-foreground">
              Çözümlenecek Seçili: <b className="text-foreground">{selectedIds.length}</b>
            </span>
            {loadingCandidates && <span className="text-muted-foreground">(Güncelleniyor...)</span>}
          </div>
          {matchingConversations.length > 0 && (
            <button
              type="button"
              onClick={toggleSelectAll}
              className="rounded-lg px-2.5 py-1 font-semibold text-wa-accent transition-colors hover:bg-wa-accent/10"
            >
              {selectedIds.length === matchingConversations.length ? "Seçimi Kaldır" : "Tümünü Seç"}
            </button>
          )}
        </div>

        {/* Matching Conversations List */}
        <div className="min-h-0 flex-1 overflow-y-auto divide-y divide-border/50">
          {matchingConversations.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-2 px-6 py-14 text-center">
              <span className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
                <Sparkles className="size-5" />
              </span>
              <p className="text-sm font-semibold">Seçilen kriterlere uyan açık sohbet bulunamadı</p>
              <p className="max-w-md text-xs text-muted-foreground">
                Saat sınırını düşürerek, statü filtrelerini genişleterek veya farklı bir temsilci seçerek tekrar deneyebilirsiniz.
              </p>
            </div>
          ) : (
            matchingConversations.map((c) => {
              const t = c.ticket!;
              const checked = !excludedIds.has(c.id);
              const lastH = elapsedHours(c, "lastMessage", now);
              const ageH = elapsedHours(c, "ticketAge", now);
              const owner = t.owner;
              return (
                <label
                  key={c.id}
                  onClick={() => toggleRow(c.id)}
                  className={cn(
                    "flex cursor-pointer items-center gap-3 px-6 py-2.5 transition-colors hover:bg-accent/40",
                    !checked && "opacity-55",
                  )}
                >
                  <span
                    className={cn(
                      "flex size-4 shrink-0 items-center justify-center rounded border transition-colors",
                      checked ? "border-wa-accent bg-wa-accent text-wa-on-accent" : "border-border bg-card",
                    )}
                  >
                    {checked && <Check className="size-3" />}
                  </span>

                  <ContactAvatar name={c.contact.display} seed={c.contact.waId} className="size-9 shrink-0 text-xs" />

                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-semibold">{c.contact.display}</span>
                      <span className="shrink-0 text-[0.7rem] text-muted-foreground">{prettyPhone(c.contact.waId)}</span>
                      <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[0.65rem] font-semibold tabular-nums text-muted-foreground">
                        #{t.number}
                      </span>
                    </div>
                    <p className="truncate text-xs text-muted-foreground">
                      {c.last ? c.last.preview || "Medya / mesaj" : "Henüz mesaj yok"}
                    </p>
                  </div>

                  {/* Owner */}
                  <div className="hidden shrink-0 items-center gap-1.5 text-xs sm:flex">
                    {owner ? (
                      <span className="flex items-center gap-1.5 rounded-full bg-muted/70 px-2.5 py-1 text-[0.72rem] font-medium">
                        <UserAvatar
                          userId={owner.id}
                          name={owner.name}
                          hasAvatar={owner.hasAvatar}
                          version={owner.avatarVersion}
                          className="size-4"
                          fallbackClassName="bg-primary/15 text-[0.45rem] text-primary"
                        />
                        <span className="max-w-[7rem] truncate">{owner.id === wa.me ? "Sen" : owner.name}</span>
                      </span>
                    ) : t.status === "bot" ? (
                      <span className="flex items-center gap-1 rounded-full bg-violet-500/15 px-2.5 py-1 text-[0.7rem] font-semibold text-violet-600 dark:text-violet-400">
                        <Bot className="size-3" /> Chatbot
                      </span>
                    ) : (
                      <span className="rounded-full bg-amber-500/15 px-2.5 py-1 text-[0.7rem] font-semibold text-amber-600 dark:text-amber-400">
                        Havuzda
                      </span>
                    )}
                  </div>

                  {/* Status & Elapsed */}
                  <div className="flex shrink-0 flex-col items-end gap-1 text-right">
                    <span
                      className={cn(
                        "rounded-full px-2 py-0.5 text-[0.68rem] font-semibold",
                        t.status === "open"
                          ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
                          : t.status === "pending"
                            ? "bg-amber-500/15 text-amber-600 dark:text-amber-400"
                            : "bg-violet-500/15 text-violet-600 dark:text-violet-400",
                      )}
                    >
                      {STATUS_WORD[t.status] ?? t.status}
                    </span>
                    <span className="text-[0.68rem] tabular-nums text-muted-foreground">
                      Son mesaj: <b className="text-foreground">{formatHours(lastH)}</b> · Açılış: {formatHours(ageH)}
                    </span>
                  </div>
                </label>
              );
            })
          )}
        </div>

        {/* Alerts */}
        {(error || resultMsg) && (
          <div className="shrink-0 border-t border-border/60 px-6 py-2.5">
            {error && (
              <p className="flex items-center gap-2 rounded-xl bg-destructive/10 px-3 py-2 text-xs font-medium text-destructive">
                <AlertCircle className="size-4 shrink-0" />
                {error}
              </p>
            )}
            {resultMsg && (
              <p className="flex items-center gap-2 rounded-xl bg-emerald-500/15 px-3 py-2 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                <UserCheck className="size-4 shrink-0" />
                {resultMsg}
              </p>
            )}
          </div>
        )}

        {/* Footer */}
        <div className="flex shrink-0 flex-col gap-3 border-t border-border/60 bg-muted/20 px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-1 flex-col gap-2 sm:flex-row sm:items-center">
            <label className="flex cursor-pointer items-center gap-2 text-xs font-medium select-none">
              <input
                type="checkbox"
                checked={sendSurvey}
                onChange={(e) => setSendSurvey(e.target.checked)}
                className="size-4 rounded border-border accent-wa-accent"
              />
              <span>Müşterilere memnuniyet anketi gönder</span>
            </label>
            <input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="İsteğe bağlı kapanış notu (sohbet geçmişine yazılır)..."
              className="h-9 min-w-0 flex-1 rounded-xl border border-border bg-card px-3 text-xs outline-none focus:border-wa-accent"
            />
          </div>

          <div className="flex shrink-0 items-center justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              className="h-10 rounded-xl px-4 text-xs font-semibold text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              Kapat
            </button>
            <button
              type="button"
              disabled={selectedIds.length === 0 || submitting}
              onClick={() => void submitBulkResolve()}
              className="flex h-10 items-center gap-2 rounded-xl bg-wa-accent px-5 text-xs font-semibold text-wa-on-accent shadow-sm transition-opacity hover:opacity-95 disabled:opacity-50"
            >
              <CheckCheck className="size-4" />
              {submitting ? "Çözümleniyor..." : `Seçilen ${selectedIds.length} Sohbeti Çözümle`}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
