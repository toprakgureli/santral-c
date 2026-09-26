// WhatsAppBot: the chatbot flow editor. The drawing in the middle, the
// selected box's settings (or the test chat, or the versions) on the right.
// Changes save themselves as a draft; customers only see a flow once it is
// published.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { AlertTriangle, ArrowLeft, BarChart3, CheckCircle2, History, MessageCircleQuestion, Redo2, Rocket, Settings2, Undo2, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Modal } from "@/components/ui";
import Canvas, { type Selection } from "@/components/whatsapp/bot/Canvas";
import NodeEditor from "@/components/whatsapp/bot/NodeEditor";
import { KINDS, portsOf, rid } from "@/components/whatsapp/bot/nodes";
import Simulator from "@/components/whatsapp/bot/Simulator";
import { BOT_TRIGGER, BotSettingsDialog } from "@/components/whatsapp/settings/BotsTab";
import { DeviceChips } from "@/components/whatsapp/settings/parts";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { BotData, BotGraph, BotNodeType, BotStats, WABot, WAChannel, WAIntegration, WATeam } from "@/whatsapp/types";
import { since } from "@/whatsapp/util";

type Panel = "edit" | "test" | "versions" | null;
type SaveState = "saved" | "dirty" | "saving" | "error";

function prune(g: BotGraph): BotGraph {
  const byId = new Map(g.nodes.map((n) => [n.id, n]));
  const seen = new Set<string>();
  const edges = g.edges.filter((e) => {
    const a = byId.get(e.from);
    if (!a || !byId.has(e.to)) return false;
    const port = e.port || "next";
    if (!portsOf(a).some((p) => p.id === port)) return false;
    const key = `${e.from}:${port}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  return edges.length === g.edges.length ? g : { ...g, edges };
}

export function WhatsAppBot() {
  const { id } = useParams();
  const botId = Number(id);
  const { user } = useAuth();
  const canEdit = can(user, "whatsapp.bot_manage");
  const canPublish = can(user, "whatsapp.bot_publish");

  const [bot, setBot] = useState<WABot | null>(null);
  const [missing, setMissing] = useState(false);
  const [graph, setGraph] = useState<BotGraph>({ nodes: [], edges: [] });
  const [channels, setChannels] = useState<WAChannel[]>([]);
  const [teams, setTeams] = useState<WATeam[]>([]);
  const [integrations, setIntegrations] = useState<WAIntegration[]>([]);
  const [selection, setSelection] = useState<Selection>(null);
  const [panel, setPanel] = useState<Panel>(null);
  const [save, setSave] = useState<SaveState>("saved");
  const [simAt, setSimAt] = useState<string | undefined>();
  const [stats, setStats] = useState<BotStats | null>(null);
  const [days, setDays] = useState(30);
  const [showStats, setShowStats] = useState(false);
  const [problems, setProblems] = useState<string[] | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [settings, setSettings] = useState(false);
  const [versions, setVersions] = useState<{ version: number; publishedBy: string; createdAt: string }[]>([]);
  const [publishing, setPublishing] = useState(false);

  const past = useRef<BotGraph[]>([]);
  const future = useRef<BotGraph[]>([]);
  const lastEdit = useRef<{ key: string; at: number }>({ key: "", at: 0 });
  const graphRef = useRef(graph);
  graphRef.current = graph;
  const loaded = useRef(false);
  const [, force] = useState(0);

  useEffect(() => {
    waApi.bot(botId).then((b) => {
      setBot(b);
      setGraph(b.draft ?? { nodes: [], edges: [] });
      loaded.current = true;
    }).catch(() => setMissing(true));
    waApi.channels().then(setChannels).catch(() => setChannels([]));
    waApi.teams().then(setTeams).catch(() => setTeams([]));
    waApi.integrations().then(setIntegrations).catch(() => setIntegrations([]));
  }, [botId]);

  // History: a snapshot before each change, grouped for typing in one field.
  const snapshot = useCallback(() => {
    past.current.push(graphRef.current);
    if (past.current.length > 80) past.current.shift();
    future.current = [];
    lastEdit.current = { key: "", at: 0 };
    force((x) => x + 1);
  }, []);

  const commit = useCallback((g: BotGraph) => {
    setGraph(prune(g));
    setSave("dirty");
  }, []);

  const undo = useCallback(() => {
    const prev = past.current.pop();
    if (!prev) return;
    future.current.push(graphRef.current);
    commit(prev);
    force((x) => x + 1);
  }, [commit]);
  const redo = useCallback(() => {
    const next = future.current.pop();
    if (!next) return;
    past.current.push(graphRef.current);
    commit(next);
    force((x) => x + 1);
  }, [commit]);

  // The draft saves itself a moment after the last change.
  const saveNow = useCallback(async () => {
    if (!canEdit) return;
    setSave("saving");
    try {
      const b = await waApi.saveDraft(botId, graphRef.current);
      setBot(b);
      setSave((s) => (s === "saving" ? "saved" : s));
    } catch {
      setSave("error");
    }
  }, [botId, canEdit]);
  useEffect(() => {
    if (save !== "dirty") return;
    const t = window.setTimeout(() => void saveNow(), 1200);
    return () => window.clearTimeout(t);
  }, [graph, save, saveNow]);
  useEffect(() => {
    const warn = (e: BeforeUnloadEvent) => {
      if (save === "dirty" || save === "saving") e.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [save]);

  const setData = useCallback((nodeId: string, patch: Partial<BotData>, key: string) => {
    const k = nodeId + ":" + key;
    const now = Date.now();
    if (lastEdit.current.key !== k || now - lastEdit.current.at > 1500) {
      past.current.push(graphRef.current);
      future.current = [];
    }
    lastEdit.current = { key: k, at: now };
    const g = graphRef.current;
    commit({ ...g, nodes: g.nodes.map((n) => (n.id === nodeId ? { ...n, data: { ...n.data, ...patch } } : n)) });
  }, [commit]);

  const addNode = useCallback((type: BotNodeType, x: number, y: number) => {
    snapshot();
    const n = { id: `${type}-${rid()}`, type, x: Math.round(x), y: Math.round(y), data: KINDS[type].data() };
    const g = graphRef.current;
    commit({ ...g, nodes: [...g.nodes, n] });
    setSelection({ kind: "node", id: n.id });
    setPanel("edit");
  }, [commit, snapshot]);

  const connect = useCallback((from: string, port: string, to: string) => {
    snapshot();
    const g = graphRef.current;
    commit({ ...g, edges: [...g.edges.filter((e) => !(e.from === from && (e.port || "next") === port)), { id: rid(), from, port, to }] });
  }, [commit, snapshot]);

  const removeSelected = useCallback(() => {
    const s = selection;
    if (!s) return;
    const g = graphRef.current;
    if (s.kind === "edge") {
      snapshot();
      commit({ ...g, edges: g.edges.filter((e) => e.id !== s.id) });
    } else {
      const n = g.nodes.find((x) => x.id === s.id);
      if (!n || n.type === "start") return;
      snapshot();
      commit({ nodes: g.nodes.filter((x) => x.id !== s.id), edges: g.edges.filter((e) => e.from !== s.id && e.to !== s.id) });
    }
    setSelection(null);
  }, [selection, commit, snapshot]);

  const duplicate = useCallback(() => {
    if (selection?.kind !== "node") return;
    const g = graphRef.current;
    const n = g.nodes.find((x) => x.id === selection.id);
    if (!n || n.type === "start") return;
    snapshot();
    const data: BotData = structuredClone(n.data);
    if (data.options) data.options = data.options.map((o) => ({ ...o, id: rid() }));
    const c = { ...n, id: `${n.type}-${rid()}`, x: n.x + 32, y: n.y + 32, data };
    commit({ ...g, nodes: [...g.nodes, c] });
    setSelection({ kind: "node", id: c.id });
  }, [selection, commit, snapshot]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.closest("input, textarea, select, [contenteditable]")) return;
      if (!canEdit) return;
      const mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === "z" && !e.shiftKey) { e.preventDefault(); undo(); }
      else if (mod && (e.key.toLowerCase() === "y" || (e.key.toLowerCase() === "z" && e.shiftKey))) { e.preventDefault(); redo(); }
      else if (mod && e.key.toLowerCase() === "d") { e.preventDefault(); duplicate(); }
      else if (e.key === "Delete" || e.key === "Backspace") { e.preventDefault(); removeSelected(); }
      else if (e.key === "Escape") setSelection(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [undo, redo, duplicate, removeSelected, canEdit]);

  useEffect(() => {
    if (!showStats) return;
    waApi.botReport(botId, days).then(setStats).catch(() => setStats(null));
  }, [showStats, days, botId]);

  useEffect(() => {
    if (panel === "versions") waApi.botVersions(botId).then(setVersions).catch(() => setVersions([]));
  }, [panel, botId, bot?.publishedVersion]);

  const publish = async () => {
    setPublishing(true);
    setNotice(null);
    try {
      if (save !== "saved") await saveNow();
      const r = await waApi.publishBot(botId);
      if (r.problems?.length) {
        setProblems(r.problems);
      } else if (r.bot) {
        setBot(r.bot);
        setNotice(r.bot.active ? `Sürüm ${r.bot.publishedVersion} yayında. Yeni gelen müşteriler bu akışı görür.` : `Sürüm ${r.bot.publishedVersion} hazır. Müşterilere açmak için Ayarlar'dan cihaz seçip açın.`);
      }
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : "Yayınlanamadı.");
    } finally {
      setPublishing(false);
    }
  };

  const vars = useMemo(() => {
    const out = new Set(["musteri", "numara"]);
    for (const n of graph.nodes) {
      if (n.data.var) out.add(n.data.var);
      for (const m of n.data.map ?? []) if (m.var) out.add(m.var);
    }
    return [...out];
  }, [graph]);
  const names = useMemo(() => ({
    integrations: Object.fromEntries(integrations.map((x) => [x.id, x.name])),
    teams: Object.fromEntries(teams.map((x) => [x.id, x.name])),
  }), [integrations, teams]);

  if (missing) {
    return <div className="flex min-h-[50vh] flex-col items-center justify-center gap-3 text-center"><p className="text-sm font-medium">Bu chatbot bulunamadı ya da görme yetkiniz yok.</p><Link to="/whatsapp/settings?tab=bots" className="text-sm font-medium text-primary">Chatbot'lara dön</Link></div>;
  }

  const selNode = selection?.kind === "node" ? graph.nodes.find((n) => n.id === selection.id) : undefined;
  const side: Panel = panel === "edit" && !selNode ? null : panel === null && selNode ? "edit" : panel;
  const T = bot ? BOT_TRIGGER[bot.trigger] ?? BOT_TRIGGER.entry : null;

  return (
    <div className="-mx-4 -my-6 flex h-[calc(100svh-4rem)] flex-col overflow-hidden md:-mx-6 lg:-mx-8">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-border/50 bg-card/70 px-4 py-2.5">
        <Link to="/whatsapp/settings?tab=bots" data-tip="Chatbot'lara dön" className="flex size-8 items-center justify-center rounded-xl text-muted-foreground hover:bg-accent hover:text-foreground"><ArrowLeft className="size-4" /></Link>
        <div className="min-w-0">
          <p className="flex items-center gap-2 truncate text-sm font-semibold">
            {bot?.name ?? "…"}
            {bot && (bot.active ? <span className="rounded-full bg-success/12 px-2 py-0.5 text-[0.62rem] font-semibold text-success">Çalışıyor</span> : <span className="rounded-full bg-muted px-2 py-0.5 text-[0.62rem] font-semibold text-muted-foreground">Kapalı</span>)}
          </p>
          <p className="flex items-center gap-2 text-[0.7rem] text-muted-foreground">
            {T && <span>{T.label}</span>}
            {bot && <span className="hidden sm:inline-flex"><DeviceChips channels={channels} ids={bot.channelIds} /></span>}
          </p>
        </div>
        <span className="mx-auto" />
        <SaveBadge state={save} bot={bot} readOnly={!canEdit} onRetry={() => void saveNow()} />
        {canEdit && (
          <span className="flex items-center rounded-xl bg-muted/60 p-0.5">
            <IconBtn tip="Geri al (Ctrl+Z)" onClick={undo} disabled={past.current.length === 0}><Undo2 className="size-4" /></IconBtn>
            <IconBtn tip="Yinele (Ctrl+Y)" onClick={redo} disabled={future.current.length === 0}><Redo2 className="size-4" /></IconBtn>
          </span>
        )}
        <span className="flex items-center gap-1">
          <ToggleBtn on={showStats} onClick={() => setShowStats((v) => !v)} tip="Kaç müşteri hangi kutudan geçti"><BarChart3 className="size-4" /> Rapor</ToggleBtn>
          <ToggleBtn on={side === "versions"} onClick={() => setPanel(side === "versions" ? null : "versions")} tip="Yayınlanmış sürümler"><History className="size-4" /> Sürümler</ToggleBtn>
          {canEdit && <ToggleBtn on={false} onClick={() => setSettings(true)} tip="Ad, açılış ve cihazlar"><Settings2 className="size-4" /> Ayarlar</ToggleBtn>}
          <ToggleBtn on={side === "test"} onClick={() => setPanel(side === "test" ? null : "test")} tip="Akışı müşteri gibi deneyin"><MessageCircleQuestion className="size-4" /> Dene</ToggleBtn>
        </span>
        {canPublish && <Button className="h-9" onClick={() => void publish()} disabled={publishing || !bot}><Rocket /> {publishing ? "Yayınlanıyor..." : "Yayınla"}</Button>}
      </header>

      {showStats && stats && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border/50 bg-primary/5 px-4 py-2 text-xs">
          <select value={days} onChange={(e) => setDays(Number(e.target.value))} className="h-7 rounded-lg border border-border/60 bg-card px-2 text-xs">
            <option value={7}>Son 7 gün</option>
            <option value={30}>Son 30 gün</option>
            <option value={90}>Son 90 gün</option>
          </select>
          <Stat label="Başlayan" value={stats.started} />
          <Stat label="Temsilciye aktarılan" value={stats.handoffs} />
          <Stat label="Chatbot'ta biten" value={stats.ended} tone="success" />
          <Stat label="Yarıda bırakan" value={stats.timeouts} tone="destructive" />
          {stats.started > 0 && <span className="text-muted-foreground">· chatbot'ta çözülme oranı %{Math.round((stats.ended / stats.started) * 100)}</span>}
        </div>
      )}
      {notice && (
        <div className="flex items-center gap-2 border-b border-success/30 bg-success/10 px-4 py-2 text-xs text-success">
          <CheckCircle2 className="size-4 shrink-0" /> <span className="flex-1">{notice}</span>
          <button type="button" onClick={() => setNotice(null)} aria-label="Kapat"><X className="size-4" /></button>
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        <div className="relative min-w-0 flex-1">
          {bot && (
            <Canvas
              graph={graph}
              selection={selection}
              onSelect={(s) => { setSelection(s); if (s?.kind === "node" && panel !== "test" && panel !== "versions") setPanel("edit"); }}
              onSnapshot={snapshot}
              onChange={commit}
              onAdd={addNode}
              onConnect={connect}
              onDeleteEdge={(eid) => { snapshot(); commit({ ...graphRef.current, edges: graphRef.current.edges.filter((e) => e.id !== eid) }); setSelection(null); }}
              readOnly={!canEdit}
              stats={showStats ? stats : null}
              names={names}
              simAt={side === "test" ? simAt : undefined}
            />
          )}
        </div>
        {side && (
          <div className="w-[22rem] shrink-0 border-l border-border/60 max-md:absolute max-md:inset-y-0 max-md:right-0 max-md:z-30 max-md:w-full max-md:max-w-sm max-md:shadow-2xl">
            {side === "edit" && selNode && (
              <NodeEditor
                key={selNode.id}
                node={selNode}
                set={(p, k) => setData(selNode.id, p, k)}
                vars={vars}
                teams={teams}
                integrations={integrations}
                readOnly={!canEdit}
                onDelete={removeSelected}
                onDuplicate={duplicate}
                onClose={() => { setSelection(null); setPanel(null); }}
              />
            )}
            {side === "test" && <Simulator graph={graph} onAt={setSimAt} onClose={() => setPanel(null)} />}
            {side === "versions" && (
              <aside className="flex h-full flex-col bg-card">
                <header className="flex items-center gap-2 border-b border-border/60 px-4 py-3">
                  <p className="flex-1 text-sm font-semibold">Sürümler</p>
                  <button type="button" onClick={() => setPanel(null)} aria-label="Kapat" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent"><X className="size-4" /></button>
                </header>
                <div className="flex-1 space-y-1.5 overflow-y-auto p-3">
                  <p className="px-1 pb-2 text-xs text-muted-foreground">Her yayın bir sürüm olarak saklanır. Eski bir sürümü geri yüklerseniz taslağa gelir; müşteriler yeniden yayınlayana kadar mevcut sürümü görmeye devam eder.</p>
                  {versions.length === 0 && <p className="px-1 text-sm text-muted-foreground">Henüz yayınlanmadı.</p>}
                  {versions.map((v) => (
                    <div key={v.version} className="flex items-center gap-3 rounded-xl px-3 py-2 ring-1 ring-border/60">
                      <span className={cn("flex size-8 items-center justify-center rounded-lg text-xs font-bold", v.version === bot?.publishedVersion ? "bg-success/12 text-success" : "bg-muted text-muted-foreground")}>{v.version}</span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm">{v.publishedBy || "?"}</p>
                        <p className="text-[0.68rem] text-muted-foreground">{new Date(v.createdAt).toLocaleString("tr-TR", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}{v.version === bot?.publishedVersion ? " · yayında" : ""}</p>
                      </div>
                      {canEdit && <Button variant="secondary" className="h-7 px-2.5 text-xs" onClick={() => void waApi.restoreVersion(botId, v.version).then((b) => { snapshot(); setBot(b); setGraph(b.draft); setSave("saved"); setNotice(`Sürüm ${v.version} taslağa geri yüklendi. Yayınlayınca müşteriler görür.`); })}>Taslağa al</Button>}
                    </div>
                  ))}
                </div>
              </aside>
            )}
          </div>
        )}
      </div>

      {problems && (
        <Modal open onClose={() => setProblems(null)} title="Yayınlamadan önce düzeltilecekler" description="Akış bu haliyle müşteriye gitmez. Aşağıdakileri düzeltip yeniden deneyin." footer={<Button onClick={() => setProblems(null)}>Tamam</Button>}>
          <ul className="space-y-1.5">
            {problems.map((p, i) => <li key={i} className="flex items-start gap-2 rounded-xl bg-warning/10 px-3 py-2 text-sm"><AlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" /> {p}</li>)}
          </ul>
        </Modal>
      )}
      {settings && bot && <BotSettingsDialog bot={bot} channels={channels} canPublish={canPublish} onClose={() => setSettings(false)} onSaved={(b) => { setBot(b); setSettings(false); }} />}
    </div>
  );
}

function SaveBadge({ state, bot, readOnly, onRetry }: { state: SaveState; bot: WABot | null; readOnly: boolean; onRetry: () => void }) {
  if (readOnly) return <span className="text-[0.7rem] text-muted-foreground">Sadece görüntüleme</span>;
  const now = Date.now();
  const text = state === "saving" ? "Kaydediliyor..." : state === "dirty" ? "Değişiklik var" : state === "error" ? "Kaydedilemedi" : bot?.updatedAt ? `Taslak kaydedildi · ${since(bot.updatedAt, now)} önce` : "Taslak kaydedildi";
  return (
    <span className={cn("flex items-center gap-1.5 text-[0.7rem]", state === "error" ? "text-destructive" : "text-muted-foreground")}>
      <span className={cn("size-1.5 rounded-full", state === "saved" ? "bg-success" : state === "error" ? "bg-destructive" : "animate-pulse bg-warning")} />
      {text}
      {state === "error" && <button type="button" onClick={onRetry} className="font-semibold underline">tekrar dene</button>}
      {bot?.draftChanged && state === "saved" && bot.publishedVersion > 0 && <span className="rounded-full bg-warning/12 px-1.5 py-px font-medium text-warning">yayınlanmadı</span>}
    </span>
  );
}

function IconBtn({ tip, onClick, disabled, children }: { tip: string; onClick: () => void; disabled?: boolean; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} disabled={disabled} data-tip={tip} aria-label={tip} className="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-card hover:text-foreground disabled:opacity-30">{children}</button>;
}

function ToggleBtn({ on, onClick, tip, children }: { on: boolean; onClick: () => void; tip: string; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} data-tip={tip} className={cn("flex h-9 items-center gap-1.5 rounded-xl px-2.5 text-xs font-medium transition-colors", on ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent hover:text-foreground")}>{children}</button>;
}

function Stat({ label, value, tone }: { label: string; value: number; tone?: "success" | "destructive" }) {
  return (
    <span className="flex items-center gap-1.5 rounded-full bg-card px-2.5 py-1 ring-1 ring-border/60">
      <b className={cn("tabular-nums", tone === "success" ? "text-success" : tone === "destructive" ? "text-destructive" : "text-foreground")}>{value}</b>
      <span className="text-muted-foreground">{label}</span>
    </span>
  );
}
