// Canvas draws a chatbot flow: boxes that can be dragged, arrows from a
// box's exits to other boxes, panning by dragging the background, zooming
// with ctrl + wheel or the buttons. New boxes come from the strip on the
// left, by clicking or dragging them onto the drawing.

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ChevronLeft, Maximize2, Minus, Plus, X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { BotGraph, BotNode, BotNodeType, BotStats } from "@/whatsapp/types";
import { BODY, HEAD, heightOf, inPoint, KINDS, PALETTE, portPoint, portsOf, preview, ROW, W } from "@/components/whatsapp/bot/nodes";

export type Selection = { kind: "node" | "edge"; id: string } | null;

interface View {
  x: number;
  y: number;
  z: number;
}

export default function Canvas({
  graph,
  selection,
  onSelect,
  onSnapshot,
  onChange,
  onAdd,
  onConnect,
  onDeleteEdge,
  readOnly,
  stats,
  names,
  simAt,
}: {
  graph: BotGraph;
  selection: Selection;
  onSelect: (s: Selection) => void;
  onSnapshot: () => void;
  onChange: (g: BotGraph) => void;
  onAdd: (type: BotNodeType, x: number, y: number) => void;
  onConnect: (from: string, port: string, to: string) => void;
  onDeleteEdge: (id: string) => void;
  readOnly: boolean;
  stats: BotStats | null;
  names: { integrations: Record<number, string>; teams: Record<number, string> };
  simAt?: string;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<View>({ x: 40, y: 40, z: 1 });
  const viewRef = useRef(view);
  viewRef.current = view;
  const graphRef = useRef(graph);
  graphRef.current = graph;
  const [palette, setPalette] = useState(() => window.innerWidth >= 1024);
  const hideStrip = readOnly || !palette;
  const hideStripRef = useRef(hideStrip);
  hideStripRef.current = hideStrip;
  const [link, setLink] = useState<{ from: string; port: string; x: number; y: number } | null>(null);
  const [hoverEdge, setHoverEdge] = useState<string | null>(null);
  const fitted = useRef(false);

  const toCanvas = useCallback((cx: number, cy: number) => {
    const r = box.current!.getBoundingClientRect();
    const v = viewRef.current;
    return { x: (cx - r.left - v.x) / v.z, y: (cy - r.top - v.y) / v.z };
  }, []);

  const fit = useCallback(() => {
    const el = box.current;
    const g = graphRef.current;
    if (!el || g.nodes.length === 0) return;
    const minX = Math.min(...g.nodes.map((n) => n.x));
    const minY = Math.min(...g.nodes.map((n) => n.y));
    const maxX = Math.max(...g.nodes.map((n) => n.x + W));
    const maxY = Math.max(...g.nodes.map((n) => n.y + heightOf(n)));
    const pad = 60;
    // The strip of new boxes covers the left edge while editing.
    const left = hideStripRef.current ? pad : 210;
    const w = el.clientWidth - left - pad;
    const z = Math.min(1.1, Math.max(0.35, Math.min(w / (maxX - minX || 1), (el.clientHeight - pad * 2) / (maxY - minY || 1))));
    setView({ z, x: left + (w - (maxX - minX) * z) / 2 - minX * z, y: Math.max(pad, (el.clientHeight - (maxY - minY) * z) / 2) - minY * z });
  }, []);

  useLayoutEffect(() => {
    if (!fitted.current && graph.nodes.length > 0) {
      fitted.current = true;
      fit();
    }
  }, [graph.nodes.length, fit]);

  // Zoom around the pointer with ctrl + wheel; a plain wheel pans.
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const v = viewRef.current;
      if (e.ctrlKey || e.metaKey) {
        const r = el.getBoundingClientRect();
        const z = Math.min(1.8, Math.max(0.3, v.z * Math.exp(-e.deltaY * 0.0015)));
        const px = e.clientX - r.left, py = e.clientY - r.top;
        setView({ z, x: px - ((px - v.x) / v.z) * z, y: py - ((py - v.y) / v.z) * z });
      } else {
        setView({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY });
      }
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  const zoomBy = (f: number) => {
    const el = box.current;
    if (!el) return;
    const v = viewRef.current;
    const z = Math.min(1.8, Math.max(0.3, v.z * f));
    const px = el.clientWidth / 2, py = el.clientHeight / 2;
    setView({ z, x: px - ((px - v.x) / v.z) * z, y: py - ((py - v.y) / v.z) * z });
  };

  const startPan = (e: React.PointerEvent) => {
    if (e.button !== 0 && e.button !== 1) return;
    onSelect(null);
    const sx = e.clientX, sy = e.clientY, v0 = viewRef.current;
    const move = (ev: PointerEvent) => setView({ ...v0, x: v0.x + ev.clientX - sx, y: v0.y + ev.clientY - sy });
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  const startDrag = (e: React.PointerEvent, n: BotNode) => {
    if (e.button !== 0) return;
    e.stopPropagation();
    onSelect({ kind: "node", id: n.id });
    if (readOnly) return;
    const sx = e.clientX, sy = e.clientY, x0 = n.x, y0 = n.y;
    let moved = false;
    const move = (ev: PointerEvent) => {
      const z = viewRef.current.z;
      const dx = (ev.clientX - sx) / z, dy = (ev.clientY - sy) / z;
      if (!moved && Math.abs(dx) + Math.abs(dy) < 3) return;
      if (!moved) {
        moved = true;
        onSnapshot();
      }
      const g = graphRef.current;
      onChange({ ...g, nodes: g.nodes.map((m) => (m.id === n.id ? { ...m, x: Math.round((x0 + dx) / 4) * 4, y: Math.round((y0 + dy) / 4) * 4 } : m)) });
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  const startLink = (e: React.PointerEvent, n: BotNode, port: string) => {
    if (readOnly || e.button !== 0) return;
    e.stopPropagation();
    const p = toCanvas(e.clientX, e.clientY);
    setLink({ from: n.id, port, x: p.x, y: p.y });
    const move = (ev: PointerEvent) => {
      const q = toCanvas(ev.clientX, ev.clientY);
      setLink((cur) => (cur ? { ...cur, x: q.x, y: q.y } : cur));
    };
    const up = (ev: PointerEvent) => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      setLink(null);
      const target = document.elementFromPoint(ev.clientX, ev.clientY)?.closest<HTMLElement>("[data-node]");
      const to = target?.dataset.node;
      if (to && to !== n.id) onConnect(n.id, port, to);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  const byId = new Map(graph.nodes.map((n) => [n.id, n]));
  const linked = new Set(graph.edges.map((e) => `${e.from}:${e.port || "next"}`));
  const maxEnter = stats ? Math.max(1, ...Object.values(stats.nodes)) : 1;
  const sel = selection?.kind === "edge" ? graph.edges.find((e) => e.id === selection.id) : undefined;
  const selMid = (() => {
    if (!sel) return null;
    const a = byId.get(sel.from), b = byId.get(sel.to);
    const p1 = a && portPoint(a, sel.port), p2 = b && inPoint(b);
    if (!p1 || !p2) return null;
    return { x: (p1.x + p2.x) / 2, y: (p1.y + p2.y) / 2 };
  })();

  return (
    <div
      ref={box}
      className="relative h-full w-full cursor-grab touch-none overflow-hidden bg-muted/30 select-none active:cursor-grabbing"
      style={{ backgroundImage: "radial-gradient(circle, color-mix(in oklab, var(--color-muted-foreground) 22%, transparent) 1px, transparent 1.2px)", backgroundSize: `${20 * view.z}px ${20 * view.z}px`, backgroundPosition: `${view.x}px ${view.y}px` }}
      onPointerDown={startPan}
      onDragOver={(e) => { if (!readOnly) e.preventDefault(); }}
      onDrop={(e) => {
        const t = e.dataTransfer.getData("text/bot-node") as BotNodeType;
        if (!t || readOnly) return;
        e.preventDefault();
        const p = toCanvas(e.clientX, e.clientY);
        onAdd(t, p.x - W / 2, p.y - HEAD / 2);
      }}
    >
      <div className="absolute top-0 left-0 origin-top-left" style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.z})` }}>
        <svg className="pointer-events-none absolute top-0 left-0 overflow-visible" width="1" height="1">
          <defs>
            <marker id="bot-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M0,0 L10,5 L0,10 z" className="fill-muted-foreground/70" />
            </marker>
            <marker id="bot-arrow-on" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M0,0 L10,5 L0,10 z" className="fill-primary" />
            </marker>
          </defs>
          {graph.edges.map((e) => {
            const a = byId.get(e.from), b = byId.get(e.to);
            const p1 = a && portPoint(a, e.port), p2 = b && inPoint(b);
            if (!p1 || !p2) return null;
            const d = curve(p1, p2);
            const on = selection?.kind === "edge" && selection.id === e.id;
            const hot = on || hoverEdge === e.id || (selection?.kind === "node" && (selection.id === e.from || selection.id === e.to));
            return (
              <g key={e.id}>
                <path d={d} fill="none" stroke="transparent" strokeWidth={14} className="pointer-events-auto cursor-pointer" onPointerDown={(ev) => { ev.stopPropagation(); onSelect({ kind: "edge", id: e.id }); }} onPointerEnter={() => setHoverEdge(e.id)} onPointerLeave={() => setHoverEdge(null)} />
                <path d={d} fill="none" className={cn("transition-colors", hot ? "stroke-primary" : "stroke-muted-foreground/45")} strokeWidth={hot ? 2.4 : 1.8} markerEnd={`url(#${hot ? "bot-arrow-on" : "bot-arrow"})`} />
              </g>
            );
          })}
          {link && (() => {
            const a = byId.get(link.from);
            const p1 = a && portPoint(a, link.port);
            return p1 ? <path d={curve(p1, { x: link.x, y: link.y })} fill="none" className="stroke-primary" strokeWidth={2} strokeDasharray="6 4" /> : null;
          })()}
        </svg>

        {selMid && !readOnly && (
          <button type="button" onPointerDown={(e) => e.stopPropagation()} onClick={() => sel && onDeleteEdge(sel.id)} data-tip="Oku kaldır" className="absolute z-20 flex size-6 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-destructive text-destructive-foreground shadow-md" style={{ left: selMid.x, top: selMid.y }}>
            <X className="size-3.5" />
          </button>
        )}

        {graph.nodes.map((n) => {
          const k = KINDS[n.type] ?? KINDS.message;
          const ports = portsOf(n);
          const on = selection?.kind === "node" && selection.id === n.id;
          const enter = stats?.nodes[n.id] ?? 0;
          const fails = stats?.fails[n.id] ?? 0;
          const drops = stats?.drops[n.id] ?? 0;
          return (
            <div
              key={n.id}
              data-node={n.id}
              className={cn(
                "absolute rounded-2xl bg-card shadow-sm ring-1 transition-shadow",
                on ? "shadow-lg ring-2 ring-primary" : link && link.from !== n.id ? "ring-2 ring-primary/40" : "ring-border/70 hover:shadow-md",
                simAt === n.id && "ring-2 ring-emerald-500 shadow-lg shadow-emerald-500/20",
              )}
              style={{ left: n.x, top: n.y, width: W, height: heightOf(n) }}
              onPointerDown={(e) => startDrag(e, n)}
            >
              {n.type !== "start" && <span className="absolute top-[19px] -left-[6px] size-3 -translate-y-1/2 rounded-full border-2 border-card bg-muted-foreground/60" />}
              <div className={cn("flex items-center gap-2 px-3", !readOnly && "cursor-move")} style={{ height: HEAD }}>
                <span className={cn("flex size-6 items-center justify-center rounded-lg", k.chip)}><k.icon className="size-3.5" /></span>
                <span className="min-w-0 flex-1 truncate text-[0.8rem] font-semibold">{k.label}</span>
                {stats && enter > 0 && (
                  <span className="rounded-full bg-primary/10 px-1.5 py-px text-[0.62rem] font-semibold text-primary tabular-nums" data-tip={`${enter} kez geçildi`}>{enter}</span>
                )}
              </div>
              <div className="px-3 text-[0.72rem] leading-snug text-muted-foreground" style={{ height: BODY }}>
                <p className="line-clamp-2 break-words">{preview(n, names)}</p>
                {stats && (fails > 0 || drops > 0) && (
                  <p className="mt-0.5 flex gap-2 text-[0.6rem] font-medium">
                    {fails > 0 && <span className="text-warning">{fails} anlaşılmadı</span>}
                    {drops > 0 && <span className="text-destructive">{drops} yarıda bıraktı</span>}
                  </p>
                )}
              </div>
              {stats && enter > 0 && <span className="absolute bottom-0 left-3 h-0.5 rounded-full bg-primary/60" style={{ width: `${Math.max(4, (enter / maxEnter) * (W - 24))}px` }} />}
              {ports.length > 0 && (
                <div className="border-t border-border/50 pt-1">
                  {ports.map((p) => {
                    const has = linked.has(`${n.id}:${p.id}`);
                    return (
                      <div key={p.id} className="relative flex items-center justify-end pr-4 pl-3" style={{ height: ROW }}>
                        <span className={cn("truncate text-[0.7rem]", p.tone === "ok" ? "text-success" : p.tone === "bad" ? "text-destructive" : p.tone === "muted" ? "text-muted-foreground/80 italic" : "text-foreground/80")}>{p.label}</span>
                        <span
                          onPointerDown={(e) => startLink(e, n, p.id)}
                          data-tip={readOnly ? undefined : has ? "Sürükleyip başka kutuya bırakın" : "Buradan bir kutuya ok çekin"}
                          className={cn("absolute top-1/2 -right-[7px] size-3.5 -translate-y-1/2 rounded-full border-2 transition-transform", readOnly ? "" : "cursor-crosshair hover:scale-125", has ? "border-card bg-primary" : p.tone === "muted" ? "border-muted-foreground/40 bg-card" : "border-warning bg-card")}
                        />
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {!readOnly && !palette && (
        <button type="button" onPointerDown={(e) => e.stopPropagation()} onClick={() => setPalette(true)} className="absolute top-3 left-3 z-10 flex h-10 items-center gap-1.5 rounded-full bg-primary px-4 text-sm font-semibold text-primary-foreground shadow-lg shadow-primary/30">
          <Plus className="size-4" /> Kutu ekle
        </button>
      )}
      {!readOnly && palette && (
        <div className="absolute top-3 left-3 z-10 flex max-h-[calc(100%-1.5rem)] w-44 flex-col gap-0.5 overflow-y-auto rounded-2xl bg-card/95 p-1.5 shadow-lg ring-1 ring-border/60 backdrop-blur" onPointerDown={(e) => e.stopPropagation()}>
          <p className="flex items-center px-2 pt-1 pb-1.5 text-[0.65rem] font-semibold tracking-wide text-muted-foreground uppercase">
            <span className="flex-1">Kutu ekle</span>
            <button type="button" onClick={() => setPalette(false)} data-tip="Gizle" aria-label="Gizle" className="rounded-md p-0.5 hover:bg-accent hover:text-foreground"><ChevronLeft className="size-3.5" /></button>
          </p>
          {PALETTE.map((t) => {
            const k = KINDS[t];
            return (
              <button
                key={t}
                type="button"
                draggable
                onDragStart={(e) => { e.dataTransfer.setData("text/bot-node", t); e.dataTransfer.effectAllowed = "copy"; }}
                onClick={() => {
                  const el = box.current!;
                  const v = viewRef.current;
                  const cx = (el.clientWidth / 2 - v.x) / v.z, cy = (el.clientHeight / 2 - v.y) / v.z;
                  onAdd(t, cx - W / 2 + (Math.random() * 40 - 20), cy - 40 + (Math.random() * 40 - 20));
                  if (el.clientWidth < 700) setPalette(false);
                }}
                data-tip={k.hint}
                className="flex items-center gap-2 rounded-xl px-2 py-1.5 text-left text-xs font-medium transition-colors hover:bg-accent"
              >
                <span className={cn("flex size-6 shrink-0 items-center justify-center rounded-lg", k.chip)}><k.icon className="size-3.5" /></span>
                {k.label}
              </button>
            );
          })}
        </div>
      )}

      <div className="absolute right-3 bottom-3 z-10 flex items-center gap-0.5 rounded-full bg-card/95 p-1 shadow-lg ring-1 ring-border/60 backdrop-blur" onPointerDown={(e) => e.stopPropagation()}>
        <button type="button" onClick={() => zoomBy(1 / 1.2)} data-tip="Uzaklaş" className="flex size-7 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><Minus className="size-3.5" /></button>
        <span className="w-10 text-center text-[0.68rem] font-medium tabular-nums text-muted-foreground">%{Math.round(view.z * 100)}</span>
        <button type="button" onClick={() => zoomBy(1.2)} data-tip="Yakınlaş" className="flex size-7 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><Plus className="size-3.5" /></button>
        <button type="button" onClick={fit} data-tip="Hepsini sığdır" className="flex size-7 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><Maximize2 className="size-3.5" /></button>
      </div>
      <p className="pointer-events-none absolute bottom-3 left-3 hidden rounded-full bg-card/80 px-3 py-1 text-[0.65rem] text-muted-foreground ring-1 ring-border/50 backdrop-blur lg:block">
        Boş yeri sürükleyerek kaydırın · Ctrl + tekerlek ile yakınlaştırın · Ok çekmek için sağdaki noktayı sürükleyin
      </p>
    </div>
  );
}

function curve(a: { x: number; y: number }, b: { x: number; y: number }): string {
  const dx = Math.max(50, Math.abs(b.x - a.x) * 0.5);
  return `M ${a.x} ${a.y} C ${a.x + dx} ${a.y}, ${b.x - dx} ${b.y}, ${b.x - 6} ${b.y}`;
}
