import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  Edit3,
  FolderPlus,
  Globe,
  History,
  Lock,
  PhoneCall,
  Play,
  Plus,
  RefreshCw,
  Search,
  Server,
  ShieldCheck,
  Terminal,
  Trash2,
} from "lucide-react";
import { ApiError } from "@/api/client";
import { apiTesterApi } from "@/apiTester/api";
import { useApiTesterFeature } from "@/apiTester/ApiTesterContext";
import {
  buildCurlCommand,
  detectExtensionBridge,
  extractPlaceholders,
  interpolate,
  renderClientRequest,
  runFromClientIP,
} from "@/apiTester/runner";
import type {
  ApiCollectionItem,
  ApiRequestItem,
  ExecutionResult,
  ExtensionBridgeState,
  RunLogItem,
} from "@/apiTester/types";
import { useAuth } from "@/auth/AuthContext";
import { CollectionModal } from "@/components/apiTester/CollectionModal";
import { RequestEditorModal } from "@/components/apiTester/RequestEditorModal";
import { Badge, Button, Card, ConfirmDialog, EmptyState, Input, Skeleton } from "@/components/ui";
import { usePeerContext } from "@/followups/PeerHints";
import { can } from "@/lib/permissions";
import { cn, formatDateTime } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

const METHOD_TONE: Record<string, string> = {
  GET: "bg-emerald-500/12 text-emerald-700 dark:text-emerald-400",
  POST: "bg-sky-500/12 text-sky-700 dark:text-sky-400",
  PUT: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  PATCH: "bg-violet-500/12 text-violet-700 dark:text-violet-400",
  DELETE: "bg-destructive/12 text-destructive",
  HEAD: "bg-muted text-muted-foreground",
  OPTIONS: "bg-muted text-muted-foreground",
};

function formatBody(raw: string): { text: string; isJson: boolean } {
  if (!raw) return { text: "", isJson: false };
  try {
    const parsed = JSON.parse(raw);
    return { text: JSON.stringify(parsed, null, 2), isJson: true };
  } catch {
    return { text: raw, isJson: false };
  }
}

function viaLabel(via: ExecutionResult["executedVia"] | string): string {
  if (via === "extension") return "Kendi IP (Eklenti)";
  if (via === "browser") return "Kendi IP (Tarayıcı)";
  if (via === "server") return "Sunucu Proxy IP";
  return via;
}

export function ApiTester() {
  const { user } = useAuth();
  const { enabled: featureEnabled, refresh: refreshFeature, setEnabledLocal } = useApiTesterFeature();
  const phone = useSoftphoneContext();
  const { ctx: peerCtx } = usePeerContext();

  const canManage = can(user, "api_tester.manage");
  const canExecute = canManage || can(user, "api_tester.execute");
  const canToggle = canManage || can(user, "system.settings");

  const [collections, setCollections] = useState<ApiCollectionItem[]>([]);
  const [moduleActive, setModuleActive] = useState(true);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});

  const [selectedColId, setSelectedColId] = useState<number | null>(null);
  const [selectedReqId, setSelectedReqId] = useState<number | null>(null);
  const [varValues, setVarValues] = useState<Record<string, string>>({});
  const [runModeOverride, setRunModeOverride] = useState<"client" | "server" | null>(null);

  const [bridgeState, setBridgeState] = useState<ExtensionBridgeState>("checking");
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<ExecutionResult | null>(null);
  const [responseTab, setResponseTab] = useState<"body" | "headers" | "logs">("body");
  const [logs, setLogs] = useState<RunLogItem[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const [colModalOpen, setColModalOpen] = useState(false);
  const [editingCol, setEditingCol] = useState<ApiCollectionItem | null>(null);
  const [deletingCol, setDeletingCol] = useState<ApiCollectionItem | null>(null);

  const [reqModalOpen, setReqModalOpen] = useState(false);
  const [reqModalColId, setReqModalColId] = useState<number | null>(null);
  const [editingReq, setEditingReq] = useState<ApiRequestItem | null>(null);
  const [deletingReq, setDeletingReq] = useState<ApiRequestItem | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);

  const checkBridge = useCallback(async () => {
    setBridgeState("checking");
    const st = await detectExtensionBridge();
    setBridgeState(st);
  }, []);

  const loadCollections = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiTesterApi.collections();
      setModuleActive(res.enabled);
      setCollections(res.collections ?? []);
      setExpanded((prev) => {
        const next = { ...prev };
        for (const c of res.collections ?? []) {
          if (next[c.id] === undefined) next[c.id] = true;
        }
        return next;
      });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "API koleksiyonları yüklenemedi.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadCollections();
    void checkBridge();
  }, [loadCollections, checkBridge]);

  const selectedPair = useMemo(() => {
    for (const c of collections) {
      if (selectedColId && c.id !== selectedColId) continue;
      for (const r of c.requests) {
        if (r.id === selectedReqId) return { col: c, req: r };
      }
    }
    for (const c of collections) {
      if (c.requests.length > 0) return { col: c, req: c.requests[0] };
    }
    return null;
  }, [collections, selectedColId, selectedReqId]);

  const activeCol = selectedPair?.col ?? null;
  const activeReq = selectedPair?.req ?? null;

  const activePhone = phone.peer || phone.lastPeer || "";
  const activeCallerName = peerCtx?.whatsapp?.name || peerCtx?.whatsapp?.profileName || "";

  // Re-initialize form parameters when switching to a different request template.
  useEffect(() => {
    if (!activeReq || !activeCol) return;
    setResult(null);
    setResponseTab("body");
    setRunModeOverride(null);
    const nextVars: Record<string, string> = {};
    if (activeCol.baseUrl) nextVars.base_url = activeCol.baseUrl;
    for (const v of activeReq.variables ?? []) {
      let val = v.defaultValue || "";
      if (!val && v.source === "call_phone" && activePhone) val = activePhone;
      if (!val && v.source === "call_name" && activeCallerName) val = activeCallerName;
      nextVars[v.key] = val;
    }
    setVarValues(nextVars);
    // Intentionally reset only when selected request changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeReq?.id]);

  const loadLogs = useCallback(async (reqId: number) => {
    setLogsLoading(true);
    try {
      const rows = await apiTesterApi.logs(reqId, 20);
      setLogs(rows);
    } catch {
      setLogs([]);
    } finally {
      setLogsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (responseTab === "logs" && activeReq) {
      void loadLogs(activeReq.id);
    }
  }, [responseTab, activeReq, loadLogs]);

  const dynamicVarFields = useMemo(() => {
    if (!activeReq) return [];
    const defined = new Map((activeReq.variables ?? []).map((v) => [v.key, v]));
    const keys = extractPlaceholders(
      activeReq.urlTemplate,
      activeReq.bodyTemplate,
      ...(activeReq.headers ?? []).map((h) => h.value),
      ...(activeReq.queryParams ?? []).map((q) => q.value),
    ).filter((k) => k !== "base_url");

    const list = [...(activeReq.variables ?? [])];
    for (const k of keys) {
      if (!defined.has(k)) {
        list.push({
          key: k,
          label: k,
          defaultValue: "",
          required: false,
          source: "manual",
          placeholder: "",
        });
      }
    }
    return list;
  }, [activeReq]);

  const effectiveMode: "client" | "server" = useMemo(() => {
    if (!activeReq) return "client";
    if (activeReq.executionMode === "server") return "server";
    if (runModeOverride === "server" && activeReq.allowServerFallback) return "server";
    return "client";
  }, [activeReq, runModeOverride]);

  const renderedPreview = useMemo(() => {
    if (!activeCol || !activeReq) return "";
    const merged: Record<string, string> = { ...varValues };
    if (activeCol.baseUrl && !merged.base_url) merged.base_url = activeCol.baseUrl;
    const base = interpolate(activeReq.urlTemplate, merged);
    if (!activeReq.queryParams?.length) return base;
    const qs = activeReq.queryParams
      .filter((q) => q.key.trim())
      .map((q) => `${q.key}=${interpolate(q.value, merged)}`)
      .join("&");
    return qs ? `${base}${base.includes("?") ? "&" : "?"}${qs}` : base;
  }, [activeCol, activeReq, varValues]);

  const fillFromActiveCall = () => {
    if (!activeReq) return;
    setVarValues((prev) => {
      const next = { ...prev };
      let filledAny = false;
      for (const f of dynamicVarFields) {
        if (f.source === "call_phone" && activePhone) {
          next[f.key] = activePhone;
          filledAny = true;
        } else if (f.source === "call_name" && activeCallerName) {
          next[f.key] = activeCallerName;
          filledAny = true;
        } else if (!filledAny && activePhone && /phone|tel|gsm|num/i.test(f.key)) {
          next[f.key] = activePhone;
        }
      }
      return next;
    });
  };

  const execute = async (forceServer = false) => {
    if (!activeCol || !activeReq || !canExecute) return;
    setRunning(true);
    setResult(null);
    setResponseTab("body");

    const modeToRun = forceServer ? "server" : effectiveMode;
    try {
      if (modeToRun === "server") {
        const res = await apiTesterApi.executeProxy(activeReq.id, varValues);
        setResult(res);
      } else {
        const rendered = renderClientRequest(activeCol, activeReq, varValues);
        const res = await runFromClientIP(rendered, bridgeState);
        setResult(res);
        void apiTesterApi
          .logRun(activeReq.id, {
            method: rendered.method,
            targetUrl: rendered.url,
            executionMode: res.executedVia === "extension" ? "extension" : "browser",
            statusCode: res.statusCode,
            durationMs: res.durationMs,
            errorMessage: res.error,
          })
          .catch(() => undefined);
      }
    } catch (e) {
      setResult({
        statusCode: 0,
        statusText: "",
        headers: [],
        body: "",
        durationMs: 0,
        targetUrl: renderedPreview,
        executedVia: modeToRun === "server" ? "server" : "browser",
        error: e instanceof ApiError ? e.message : e instanceof Error ? e.message : "İstek çalıştırılamadı.",
      });
    } finally {
      setRunning(false);
    }
  };

  const copyCurl = () => {
    if (!activeCol || !activeReq) return;
    try {
      const rendered = renderClientRequest(activeCol, activeReq, varValues);
      const cmd = buildCurlCommand(rendered);
      void navigator.clipboard.writeText(cmd);
      setCopiedKey("curl");
      window.setTimeout(() => setCopiedKey(null), 1500);
    } catch (e) {
      setResult({
        statusCode: 0,
        statusText: "",
        headers: [],
        body: "",
        durationMs: 0,
        targetUrl: renderedPreview,
        executedVia: "browser",
        error: e instanceof Error ? e.message : "cURL oluşturulamadı.",
      });
    }
  };

  const copyText = (key: string, val: string) => {
    void navigator.clipboard.writeText(val);
    setCopiedKey(key);
    window.setTimeout(() => setCopiedKey(null), 1500);
  };

  const confirmDelete = async () => {
    setDeleteBusy(true);
    try {
      if (deletingReq) {
        await apiTesterApi.deleteRequest(deletingReq.id);
        setDeletingReq(null);
      } else if (deletingCol) {
        await apiTesterApi.deleteCollection(deletingCol.id);
        setDeletingCol(null);
      }
      await loadCollections();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Silme işlemi başarısız oldu.");
    } finally {
      setDeleteBusy(false);
    }
  };

  const activateModule = async () => {
    try {
      const res = await apiTesterApi.setStatus(true);
      setModuleActive(res.enabled);
      setEnabledLocal(res.enabled);
      await refreshFeature();
      await loadCollections();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Modül aktifleştirilemedi.");
    }
  };

  const filteredCollections = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return collections;
    return collections
      .map((c) => {
        const colMatch =
          c.name.toLowerCase().includes(q) ||
          c.projectTag.toLowerCase().includes(q) ||
          c.description.toLowerCase().includes(q);
        if (colMatch) return c;
        const reqs = c.requests.filter(
          (r) =>
            r.name.toLowerCase().includes(q) ||
            r.urlTemplate.toLowerCase().includes(q) ||
            r.method.toLowerCase().includes(q),
        );
        return reqs.length > 0 ? { ...c, requests: reqs } : null;
      })
      .filter((c): c is ApiCollectionItem => c !== null);
  }, [collections, search]);

  const formattedResponse = useMemo(() => formatBody(result?.body ?? ""), [result?.body]);

  if (loading) {
    return (
      <div className="grid gap-5 lg:grid-cols-12">
        <Skeleton className="h-[28rem] lg:col-span-4" />
        <Skeleton className="h-[28rem] lg:col-span-8" />
      </div>
    );
  }

  if ((!featureEnabled || !moduleActive) && !canManage) {
    return (
      <Card title="API Test Koleksiyonları" icon={Terminal}>
        <EmptyState
          icon={<Terminal />}
          title="API Testleri modülü şu anda pasif"
          description="Bu özellik sistem yöneticisi tarafından geçici olarak kapatılmış."
        />
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {(!featureEnabled || !moduleActive) && canManage && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm">
          <div className="flex items-center gap-2.5 text-amber-800 dark:text-amber-300">
            <AlertTriangle className="size-4 shrink-0" />
            <span>API Testleri menü butonu şu anda pasif. Koleksiyonları hazırlayabilirsiniz; personelin görmesi için aktifleştirin.</span>
          </div>
          {canToggle && (
            <Button className="h-8 px-3 text-xs" onClick={activateModule}>
              Menüyü Aktifleştir
            </Button>
          )}
        </div>
      )}

      {error && (
        <div className="rounded-xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="grid items-start gap-5 lg:grid-cols-12">
        {/* Left sidebar: Collections & Requests tree */}
        <div className="lg:col-span-4">
          <Card
            title="Test Koleksiyonları"
            icon={Terminal}
            actions={
              canManage ? (
                <Button
                  className="h-8 px-2.5 text-xs"
                  onClick={() => {
                    setEditingCol(null);
                    setColModalOpen(true);
                  }}
                >
                  <FolderPlus className="size-3.5" /> Yeni Koleksiyon
                </Button>
              ) : undefined
            }
          >
            <div className="space-y-3">
              <div className="relative">
                <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Koleksiyon, etiket veya URL ara..."
                  className="h-9 pl-9 text-xs"
                />
              </div>

              {filteredCollections.length === 0 ? (
                <EmptyState
                  icon={<Terminal />}
                  title={collections.length === 0 ? "Henüz API koleksiyonu yok" : "Eşleşen istek bulunamadı"}
                  description={canManage ? "Müşteri projeleri için ilk API koleksiyonunu oluşturun." : "Yöneticiniz koleksiyon eklediğinde burada listelenecektir."}
                />
              ) : (
                <div className="space-y-2">
                  {filteredCollections.map((col) => {
                    const isOpen = expanded[col.id] ?? true;
                    return (
                      <div key={col.id} className="rounded-xl border border-border/60 bg-muted/20">
                        <div className="flex items-center gap-1.5 px-2.5 py-2">
                          <button
                            type="button"
                            onClick={() => setExpanded((p) => ({ ...p, [col.id]: !isOpen }))}
                            className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
                          >
                            {isOpen ? <ChevronDown className="size-4 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-4 shrink-0 text-muted-foreground" />}
                            <span className="truncate text-sm font-semibold">{col.name}</span>
                            {col.projectTag && <Badge tone="blue">{col.projectTag}</Badge>}
                            {!col.active && <Badge tone="slate">Pasif</Badge>}
                          </button>
                          {canManage && (
                            <div className="flex items-center gap-0.5">
                              <button
                                type="button"
                                title="Bu koleksiyona istek ekle"
                                onClick={() => {
                                  setReqModalColId(col.id);
                                  setEditingReq(null);
                                  setReqModalOpen(true);
                                }}
                                className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                              >
                                <Plus className="size-3.5" />
                              </button>
                              <button
                                type="button"
                                title="Koleksiyonu düzenle"
                                onClick={() => {
                                  setEditingCol(col);
                                  setColModalOpen(true);
                                }}
                                className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                              >
                                <Edit3 className="size-3.5" />
                              </button>
                              <button
                                type="button"
                                title="Koleksiyonu sil"
                                onClick={() => setDeletingCol(col)}
                                className="rounded-lg p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                              >
                                <Trash2 className="size-3.5" />
                              </button>
                            </div>
                          )}
                        </div>

                        {isOpen && (
                          <div className="space-y-0.5 border-t border-border/50 p-1.5">
                            {col.requests.length === 0 ? (
                              <p className="px-2 py-2 text-xs text-muted-foreground">Bu koleksiyonda henüz istek yok.</p>
                            ) : (
                              col.requests.map((r) => {
                                const selected = activeReq?.id === r.id;
                                return (
                                  <button
                                    key={r.id}
                                    type="button"
                                    onClick={() => {
                                      setSelectedColId(col.id);
                                      setSelectedReqId(r.id);
                                    }}
                                    className={cn(
                                      "flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-xs transition",
                                      selected
                                        ? "bg-primary/12 font-semibold text-foreground ring-1 ring-primary/25"
                                        : "text-muted-foreground hover:bg-accent/70 hover:text-foreground",
                                    )}
                                  >
                                    <span className={cn("w-12 shrink-0 rounded-md px-1.5 py-0.5 text-center font-mono text-[10px] font-bold", METHOD_TONE[r.method] ?? "bg-muted text-foreground")}>
                                      {r.method}
                                    </span>
                                    <span className="min-w-0 flex-1 truncate">{r.name}</span>
                                    {!r.active && <Badge tone="slate">Pasif</Badge>}
                                  </button>
                                );
                              })
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </Card>
        </div>

        {/* Right workspace: Request runner & Response viewer */}
        <div className="space-y-5 lg:col-span-8">
          {!activeReq || !activeCol ? (
            <Card>
              <EmptyState
                icon={<Terminal />}
                title="Bir API isteği seçin"
                description="Sol listeden bir istek seçerek parametreleri doldurup kendi IP adresinizden test edebilirsiniz."
              />
            </Card>
          ) : (
            <>
              <Card
                title={activeReq.name}
                icon={Globe}
                actions={
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={cn("rounded-lg px-2 py-0.5 font-mono text-xs font-bold", METHOD_TONE[activeReq.method] ?? "bg-muted")}>
                      {activeReq.method}
                    </span>
                    {canManage && (
                      <>
                        <Button
                          variant="secondary"
                          className="h-8 px-2.5 text-xs"
                          onClick={() => {
                            setReqModalColId(activeCol.id);
                            setEditingReq(activeReq);
                            setReqModalOpen(true);
                          }}
                        >
                          <Edit3 className="size-3.5" /> Düzenle
                        </Button>
                        <Button
                          variant="ghost"
                          className="h-8 px-2 text-xs text-destructive hover:bg-destructive/10"
                          onClick={() => setDeletingReq(activeReq)}
                        >
                          <Trash2 className="size-3.5" />
                        </Button>
                      </>
                    )}
                  </div>
                }
              >
                <div className="space-y-4">
                  {activeReq.description && (
                    <p className="text-sm text-muted-foreground">{activeReq.description}</p>
                  )}

                  {/* Resolved URL preview + extension bridge status */}
                  <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-border/60 bg-muted/30 px-3.5 py-2.5">
                    <code className="min-w-0 flex-1 break-all font-mono text-xs text-foreground">
                      {renderedPreview || activeReq.urlTemplate}
                    </code>
                    <div className="flex shrink-0 items-center gap-1.5">
                      {bridgeState === "ready" ? (
                        <Badge tone="green">Chrome Eklentisi Hazır (Kendi IP, CORS'suz)</Badge>
                      ) : bridgeState === "wrong_origin" ? (
                        <Badge tone="amber">Eklenti Panel Adresi Farklı</Badge>
                      ) : (
                        <Badge tone="slate">Eklenti Yok (Tarayıcı Fetch)</Badge>
                      )}
                      <button
                        type="button"
                        onClick={() => void checkBridge()}
                        title="Eklenti bağlantısını yenile"
                        className="rounded-lg p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                      >
                        <RefreshCw className={cn("size-3.5", bridgeState === "checking" && "animate-spin")} />
                      </button>
                    </div>
                  </div>

                  {/* Troubleshooting Runbook */}
                  {activeReq.runbook && (
                    <div className="flex items-start gap-2.5 rounded-xl border border-sky-500/30 bg-sky-500/8 px-3.5 py-2.5 text-xs text-sky-900 dark:text-sky-200">
                      <BookOpen className="mt-0.5 size-4 shrink-0 text-sky-600 dark:text-sky-400" />
                      <div className="space-y-0.5">
                        <p className="font-semibold">Destek Notu (Runbook)</p>
                        <p className="whitespace-pre-wrap leading-relaxed">{activeReq.runbook}</p>
                      </div>
                    </div>
                  )}

                  {/* Active call autofill bar */}
                  {activePhone && dynamicVarFields.length > 0 && (
                    <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-emerald-500/35 bg-emerald-500/10 px-3.5 py-2 text-xs">
                      <div className="flex items-center gap-2 text-emerald-800 dark:text-emerald-300">
                        <PhoneCall className="size-4 shrink-0" />
                        <span>
                          Aktif / Son Çağrı: <strong>{activePhone}</strong>
                          {activeCallerName ? ` (${activeCallerName})` : ""}
                        </span>
                      </div>
                      <Button variant="secondary" className="h-7 px-2.5 text-xs" onClick={fillFromActiveCall}>
                        Çağrı Bilgisiyle Doldur
                      </Button>
                    </div>
                  )}

                  {/* Dynamic variables form */}
                  {dynamicVarFields.length > 0 && (
                    <div className="grid gap-3 sm:grid-cols-2">
                      {dynamicVarFields.map((v) => (
                        <label key={v.key} className="block space-y-1">
                          <span className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                            <span>{v.label || v.key}</span>
                            {v.required && <span className="text-destructive">*</span>}
                            <code className="rounded bg-muted px-1 py-0.5 text-[10px]">{`{{${v.key}}}`}</code>
                          </span>
                          <Input
                            value={varValues[v.key] ?? ""}
                            onChange={(e) => setVarValues((p) => ({ ...p, [v.key]: e.target.value }))}
                            placeholder={v.placeholder || v.defaultValue || v.key}
                            className="h-9 text-sm"
                          />
                        </label>
                      ))}
                    </div>
                  )}

                  {/* Headers preview */}
                  {activeReq.headers && activeReq.headers.length > 0 && (
                    <div className="space-y-1.5">
                      <p className="text-xs font-medium text-muted-foreground">Tanımlı Header Bilgileri</p>
                      <div className="flex flex-wrap gap-1.5">
                        {activeReq.headers.map((h, i) => (
                          <span
                            key={`${h.key}-${i}`}
                            className="inline-flex items-center gap-1 rounded-lg border border-border/60 bg-muted/40 px-2.5 py-1 font-mono text-[11px]"
                          >
                            {h.secret && <Lock className="size-3 text-amber-500" />}
                            <span className="font-semibold">{h.key}:</span>
                            <span className="max-w-[16rem] truncate text-muted-foreground">
                              {h.secret ? "••••••••" : h.value}
                            </span>
                          </span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Actions & Execution mode */}
                  <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border/60 pt-3">
                    <div className="flex flex-wrap items-center gap-2 text-xs">
                      {activeReq.executionMode === "server" ? (
                        <span className="inline-flex items-center gap-1.5 rounded-lg border border-border/70 bg-muted/50 px-2.5 py-1.5 font-medium text-muted-foreground">
                          <Server className="size-3.5" /> Sunucu IP Modu (Gizli anahtarlar backend'de kalır)
                        </span>
                      ) : activeReq.allowServerFallback ? (
                        <div className="inline-flex rounded-xl border border-border/70 bg-muted/40 p-0.5">
                          <button
                            type="button"
                            onClick={() => setRunModeOverride("client")}
                            className={cn(
                              "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium transition",
                              effectiveMode === "client" ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
                            )}
                          >
                            <Globe className="size-3.5" /> Kendi IP'm
                          </button>
                          <button
                            type="button"
                            onClick={() => setRunModeOverride("server")}
                            className={cn(
                              "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium transition",
                              effectiveMode === "server" ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
                            )}
                          >
                            <Server className="size-3.5" /> Sunucu Proxy
                          </button>
                        </div>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 rounded-lg border border-border/70 bg-muted/50 px-2.5 py-1.5 font-medium">
                          <ShieldCheck className="size-3.5 text-emerald-600" /> Sadece Kendi IP'n
                        </span>
                      )}
                    </div>

                    <div className="flex flex-wrap items-center gap-2">
                      {canExecute && (
                        <Button variant="secondary" className="h-9 px-3 text-xs" onClick={copyCurl}>
                          {copiedKey === "curl" ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
                          {copiedKey === "curl" ? "Kopyalandı" : "cURL Kopyala"}
                        </Button>
                      )}
                      <Button
                        className="h-9 px-4 text-xs"
                        disabled={!canExecute || running}
                        onClick={() => void execute(false)}
                      >
                        <Play className="size-3.5" />
                        {running ? "Gönderiliyor..." : "İsteği Gönder"}
                      </Button>
                    </div>
                  </div>
                  {!canExecute && (
                    <p className="text-xs text-muted-foreground">Bu koleksiyonu görüntüleme yetkiniz var; test isteği çalıştırmak için `api_tester.execute` yetkisi gerekir.</p>
                  )}
                </div>
              </Card>

              {/* CORS Blocked Warning with Explicit User-Friendly Offer */}
              {result?.corsBlocked && (
                <div className="space-y-3 rounded-2xl border border-amber-500/45 bg-amber-500/10 p-4 text-sm">
                  <div className="flex items-start gap-2.5">
                    <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-400" />
                    <div className="space-y-1">
                      <p className="font-semibold text-amber-900 dark:text-amber-200">
                        Tarayıcı CORS Politikası Nedeniyle İstek Engellendi
                      </p>
                      <p className="text-xs leading-relaxed text-amber-800/90 dark:text-amber-300/90">
                        {result.extensionHint === "wrong_origin"
                          ? `SantralC Chrome eklentisi yüklü görünüyor ancak eklentide kayıtlı panel adresi bu sayfayla (${window.location.origin}) eşleşmiyor.`
                          : "Hedef API tarayıcıdan doğrudan erişime (CORS) izin vermediği ve SantralC Chrome eklentisi aktif olmadığı için Chrome isteği durdurdu."}
                        {" "}İstersen bu isteği <strong>Santral-C sunucu proxysi üzerinden</strong> gönderebiliriz (not: müşteri sunucusuna senin IP'n yerine Santral-C sunucu IP'si gider), ya da hazır <strong>cURL</strong> komutunu kopyalayıp kendi terminalinden kendi IP'nle atabilirsin.
                      </p>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 pl-7">
                    {activeReq.allowServerFallback && canExecute && (
                      <Button
                        className="h-8 px-3 text-xs"
                        disabled={running}
                        onClick={() => void execute(true)}
                      >
                        <Server className="size-3.5" /> İsteği Sunucu Proxysinden Gönder
                      </Button>
                    )}
                    <Button variant="secondary" className="h-8 px-3 text-xs" onClick={copyCurl}>
                      <Copy className="size-3.5" /> {copiedKey === "curl" ? "cURL Kopyalandı" : "cURL Kopyala (Kendi IP'n)"}
                    </Button>
                  </div>
                </div>
              )}

              {/* Response & Logs Panel */}
              <Card
                title="Yanıt ve Geçmiş"
                icon={History}
                actions={
                  result ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={result.statusCode >= 200 && result.statusCode < 300 ? "green" : result.statusCode >= 400 || result.error ? "red" : "amber"}>
                        {result.statusCode > 0 ? `${result.statusCode} ${result.statusText}`.trim() : "Hata"}
                      </Badge>
                      <Badge tone="slate">{result.durationMs} ms</Badge>
                      <Badge tone={result.executedVia === "server" ? "amber" : "blue"}>{viaLabel(result.executedVia)}</Badge>
                    </div>
                  ) : undefined
                }
              >
                <div className="space-y-3">
                  <div className="flex items-center justify-between gap-2 border-b border-border/60 pb-2">
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        onClick={() => setResponseTab("body")}
                        className={cn(
                          "rounded-lg px-3 py-1.5 text-xs font-medium transition",
                          responseTab === "body" ? "bg-primary/12 text-primary" : "text-muted-foreground hover:text-foreground",
                        )}
                      >
                        Yanıt Gövdesi
                      </button>
                      <button
                        type="button"
                        onClick={() => setResponseTab("headers")}
                        className={cn(
                          "rounded-lg px-3 py-1.5 text-xs font-medium transition",
                          responseTab === "headers" ? "bg-primary/12 text-primary" : "text-muted-foreground hover:text-foreground",
                        )}
                      >
                        Header'lar {result?.headers?.length ? `(${result.headers.length})` : ""}
                      </button>
                      <button
                        type="button"
                        onClick={() => setResponseTab("logs")}
                        className={cn(
                          "rounded-lg px-3 py-1.5 text-xs font-medium transition",
                          responseTab === "logs" ? "bg-primary/12 text-primary" : "text-muted-foreground hover:text-foreground",
                        )}
                      >
                        Son Çalıştırmalar
                      </button>
                    </div>
                    {responseTab === "body" && result?.body && (
                      <Button
                        variant="secondary"
                        className="h-7 px-2.5 text-xs"
                        onClick={() => copyText("resp", formattedResponse.text)}
                      >
                        {copiedKey === "resp" ? <Check className="size-3 text-emerald-500" /> : <Copy className="size-3" />}
                        {copiedKey === "resp" ? "Kopyalandı" : "Yanıtı Kopyala"}
                      </Button>
                    )}
                  </div>

                  {responseTab === "body" && (
                    !result ? (
                      <p className="py-6 text-center text-xs text-muted-foreground">Henüz istek gönderilmedi. Parametreleri kontrol edip "İsteği Gönder"e tıklayın.</p>
                    ) : result.error && !result.body ? (
                      <div className="rounded-xl border border-destructive/40 bg-destructive/8 p-3 text-xs text-destructive">
                        {result.error}
                      </div>
                    ) : (
                      <div className="space-y-2">
                        {result.error && (
                          <p className="text-xs text-destructive">{result.error}</p>
                        )}
                        {result.truncated && (
                          <p className="text-xs text-amber-600">Yanıt 1 MB sınırını aştığı için kırpıldı.</p>
                        )}
                        <pre className="max-h-[26rem] overflow-auto rounded-xl border border-border/60 bg-muted/40 p-3.5 font-mono text-xs leading-relaxed text-foreground">
                          {formattedResponse.text || "(Boş yanıt gövdesi)"}
                        </pre>
                      </div>
                    )
                  )}

                  {responseTab === "headers" && (
                    !result || result.headers.length === 0 ? (
                      <p className="py-6 text-center text-xs text-muted-foreground">Görüntülenecek yanıt header'ı yok.</p>
                    ) : (
                      <div className="overflow-x-auto">
                        <table className="w-full text-xs">
                          <tbody>
                            {result.headers.map((h, i) => (
                              <tr key={`${h.key}-${i}`} className="border-t border-border/50">
                                <td className="py-1.5 pr-4 font-mono font-semibold text-foreground">{h.key}</td>
                                <td className="py-1.5 font-mono break-all text-muted-foreground">{h.value}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )
                  )}

                  {responseTab === "logs" && (
                    logsLoading ? (
                      <Skeleton className="h-32" />
                    ) : logs.length === 0 ? (
                      <p className="py-6 text-center text-xs text-muted-foreground">Bu istek için henüz çalıştırma kaydı yok.</p>
                    ) : (
                      <div className="overflow-x-auto">
                        <table className="w-full text-xs">
                          <thead>
                            <tr className="text-left text-muted-foreground">
                              <th className="pb-2">Zaman</th>
                              <th className="pb-2">Kullanıcı</th>
                              <th className="pb-2">Mod</th>
                              <th className="pb-2">Durum</th>
                              <th className="pb-2">Süre</th>
                            </tr>
                          </thead>
                          <tbody>
                            {logs.map((l) => (
                              <tr key={l.id} className="border-t border-border/50">
                                <td className="py-2 whitespace-nowrap text-muted-foreground">{formatDateTime(l.createdAt)}</td>
                                <td className="py-2 font-medium">{l.userName || "-"}</td>
                                <td className="py-2">
                                  <Badge tone={l.executionMode === "server" ? "amber" : "blue"}>{viaLabel(l.executionMode)}</Badge>
                                </td>
                                <td className="py-2 tabular-nums">
                                  {l.statusCode > 0 ? l.statusCode : l.errorMessage ? "Hata" : "-"}
                                </td>
                                <td className="py-2 tabular-nums text-muted-foreground">{l.durationMs} ms</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )
                  )}
                </div>
              </Card>
            </>
          )}
        </div>
      </div>

      <CollectionModal
        open={colModalOpen}
        editing={editingCol}
        onClose={() => setColModalOpen(false)}
        onSaved={() => void loadCollections()}
      />

      <RequestEditorModal
        open={reqModalOpen}
        collectionId={reqModalColId}
        editing={editingReq}
        onClose={() => setReqModalOpen(false)}
        onSaved={(saved) => {
          setSelectedColId(saved.collectionId);
          setSelectedReqId(saved.id);
          void loadCollections();
        }}
      />

      <ConfirmDialog
        open={!!deletingCol || !!deletingReq}
        tone="danger"
        title={deletingReq ? "İstek şablonu silinsin mi?" : "Koleksiyon silinsin mi?"}
        description={
          deletingReq
            ? `"${deletingReq.name}" şablonu kalıcı olarak silinecek.`
            : `"${deletingCol?.name}" koleksiyonu ve içindeki tüm istekler silinecek.`
        }
        confirmLabel="Evet, sil"
        busy={deleteBusy}
        onConfirm={() => void confirmDelete()}
        onCancel={() => {
          setDeletingCol(null);
          setDeletingReq(null);
        }}
      />
    </div>
  );
}
