// Simulator plays the drawn flow as a customer would see it, without
// sending anything to anyone. Outside-system questions are really asked.
// It can pretend any weekday and time; working hours then come from the
// chosen device, and it says so when the chatbot would not greet at all.

import { Fragment, useEffect, useRef, useState } from "react";
import { BellOff, Clock3, Info, Moon, PhoneCall, RotateCcw, SendHorizontal, Star, Sun, Tag, UserRound, X, Flag, Globe } from "lucide-react";
import { DAY_SHORT, TimeInput } from "@/components/whatsapp/settings/TimeParts";
import { ApiError } from "@/api/client";
import { waText } from "@/components/whatsapp/waText";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { BotGraph, BotOption, SimOutput, WAChannel } from "@/whatsapp/types";

type Line = { side: "bot"; out: SimOutput } | { side: "me"; text: string };

export default function Simulator({ graph, botId, channels, onAt, onClose }: { graph: BotGraph; botId?: number; channels: WAChannel[]; onAt: (id: string | undefined) => void; onClose: () => void }) {
  const [lines, setLines] = useState<Line[]>([]);
  const [state, setState] = useState<{ nodeId: string; vars: Record<string, string>; tries: number; done: boolean } | null>(null);
  const [text, setText] = useState("");
  const [hoursOpen, setHoursOpen] = useState(true);
  // the moment the test pretends it is, for "belirli saatlerdeyse"; null is now
  const [at, setAt] = useState<{ clock: string; day: number } | null>(null);
  const [clockOpen, setClockOpen] = useState(false);
  // whose working hours count; with no device the switch below decides
  const [channelId, setChannelId] = useState(channels[0]?.id ?? 0);
  const [assumed, setAssumed] = useState<{ open: boolean; channel?: string } | null>(null);
  // the device list may arrive after the panel opens
  useEffect(() => {
    if (!channels.some((c) => c.id === channelId)) setChannelId(channels[0]?.id ?? 0);
  }, [channels]); // eslint-disable-line react-hooks/exhaustive-deps
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const end = useRef<HTMLDivElement>(null);

  // Only the latest question's answer is shown; a restart drops the rest.
  const seq = useRef(0);
  const run = async (input: { start: boolean; text?: string; choiceId?: string }) => {
    const my = ++seq.current;
    setBusy(true);
    setError(null);
    try {
      const r = await waApi.simulate({ graph, nodeId: input.start ? "" : state?.nodeId ?? "", vars: input.start ? undefined : state?.vars, tries: input.start ? 0 : state?.tries ?? 0, text: input.text, choiceId: input.choiceId, start: input.start, hoursOpen, clock: at?.clock, day: at?.day, botId, channelId: channelId || undefined });
      if (my !== seq.current) return;
      setLines((cur) => [...cur, ...r.outputs.map((o) => ({ side: "bot" as const, out: o }))]);
      setState({ nodeId: r.nodeId, vars: r.vars, tries: r.tries, done: r.done });
      setAssumed({ open: r.hoursOpen, channel: r.channel });
      onAt(r.done ? undefined : r.nodeId);
    } catch (e) {
      if (my === seq.current) setError(e instanceof ApiError ? e.message : "Denenemedi.");
    } finally {
      if (my === seq.current) setBusy(false);
    }
  };

  const restart = () => {
    setLines([]);
    setState(null);
    onAt(undefined);
    void run({ start: true });
  };

  useEffect(() => {
    restart();
    return () => onAt(undefined);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // A new day, time or device is a new test.
  const first = useRef(true);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    restart();
  }, [at?.clock, at?.day, channelId, hoursOpen]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    end.current?.scrollIntoView({ block: "end" });
  }, [lines]);

  const send = (t: string, choice?: BotOption) => {
    if (!state || state.done || busy) return;
    setLines((cur) => [...cur, { side: "me", text: choice ? choice.label : t }]);
    setText("");
    void run({ start: false, text: choice ? choice.label : t, choiceId: choice ? `opt:${choice.id}` : undefined });
  };

  const skipped = lines.some((l) => l.side === "bot" && l.out.kind === "skip");
  const lastMenu = [...lines].reverse().find((l) => l.side === "bot" && l.out.kind === "menu");

  return (
    <aside className="flex h-full w-full flex-col bg-card">
      <header className="flex items-center gap-2 border-b border-border/60 px-4 py-3">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold">Dene</p>
          <p className="text-[0.7rem] text-muted-foreground">Taslak akış, müşteri gibi. Kimseye mesaj gitmez.</p>
        </div>
        {channels.length === 0 ? (
          <button type="button" onClick={() => { setHoursOpen((v) => !v); }} data-tip={hoursOpen ? "Mesai içi sayılıyor. Değiştirmek için tıklayın." : "Mesai dışı sayılıyor. Değiştirmek için tıklayın."} className={cn("flex h-8 items-center gap-1 rounded-full px-2.5 text-[0.7rem] font-medium ring-1", hoursOpen ? "text-amber-600 ring-amber-500/30" : "text-indigo-500 ring-indigo-500/30")}>
            {hoursOpen ? <Sun className="size-3.5" /> : <Moon className="size-3.5" />} {hoursOpen ? "Mesai içi" : "Mesai dışı"}
          </button>
        ) : assumed && (
          <span data-tip={`${assumed.channel ?? "Cihaz"} mesai saatlerine göre`} className={cn("flex h-8 items-center gap-1 rounded-full px-2.5 text-[0.7rem] font-medium ring-1", assumed.open ? "text-amber-600 ring-amber-500/30" : "text-indigo-500 ring-indigo-500/30")}>
            {assumed.open ? <Sun className="size-3.5" /> : <Moon className="size-3.5" />} {assumed.open ? "Mesai içi" : "Mesai dışı"}
          </span>
        )}
        <button type="button" onClick={() => setClockOpen((v) => !v)} data-tip="Denemede saat kaç olsun" className={cn("flex h-8 items-center gap-1 rounded-full px-2.5 text-[0.7rem] font-medium ring-1", at ? "text-violet-600 ring-violet-500/30 dark:text-violet-400" : "text-muted-foreground ring-border/60")}>
          <Clock3 className="size-3.5" /> {at ? `${DAY_SHORT[at.day]} ${at.clock}` : "Şimdi"}
        </button>
        <button type="button" onClick={restart} data-tip="Baştan başlat" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><RotateCcw className="size-4" /></button>
        <button type="button" onClick={onClose} aria-label="Kapat" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-4" /></button>
      </header>
      {clockOpen && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border/60 bg-muted/40 px-4 py-2.5 text-xs">
          <span className="text-muted-foreground">Deneme için saat:</span>
          <select value={at?.day ?? nowDay()} onChange={(e) => setAt({ clock: at?.clock ?? nowClock(), day: Number(e.target.value) })} className="h-9 rounded-lg border border-border/60 bg-card px-2 text-sm">
            {DAY_SHORT.map((d, i) => <option key={d} value={i}>{d}</option>)}
          </select>
          <TimeInput value={at?.clock ?? nowClock()} onChange={(clock) => setAt({ clock, day: at?.day ?? nowDay() })} label="Saat" />
          {at && <button type="button" onClick={() => setAt(null)} className="font-medium text-primary hover:underline">Şimdiye dön</button>}
          {channels.length > 1 && (
            <select value={channelId} onChange={(e) => setChannelId(Number(e.target.value))} className="h-9 rounded-lg border border-border/60 bg-card px-2 text-sm" aria-label="Cihaz">
              {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          )}
          <span className="basis-full text-[0.68rem] text-muted-foreground">{channels.length ? `Mesai saatleri ${channels.find((c) => c.id === channelId)?.name ?? "cihazın"} ayarlarından alınır. ` : ""}Değiştirince deneme baştan başlar.</span>
        </div>
      )}
      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto bg-[#efe7dd] px-3 py-4 dark:bg-[#0b141a]">
        {lines.map((l, i) => l.side === "me" ? (
          <div key={i} className="flex justify-end"><p className="max-w-[80%] rounded-xl rounded-tr-sm bg-[#d9fdd3] px-3 py-1.5 text-sm text-slate-900 shadow-sm dark:bg-[#005c4b] dark:text-slate-50">{l.text}</p></div>
        ) : <BotLine key={i} out={l.out} onPick={(o) => send(o.label, o)} active={l === lastMenu && !!state && !state.done} />)}
        {busy && <div className="flex gap-1 px-2 py-1"><Dot /><Dot d={150} /><Dot d={300} /></div>}
        {state?.done && <p className="py-2 text-center text-[0.7rem] font-medium text-slate-500">{skipped ? "Chatbot başlamadı" : "Chatbot bitti"} · <button type="button" onClick={restart} className="text-primary underline">baştan dene</button></p>}
        {error && <p className="rounded-lg bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</p>}
        <div ref={end} />
      </div>
      {state && Object.keys(state.vars).length > 0 && (
        <details className="border-t border-border/60 px-4 py-2 text-xs">
          <summary className="cursor-pointer font-medium text-muted-foreground">Toplanan bilgiler ({Object.keys(state.vars).length})</summary>
          <dl className="mt-1.5 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 font-mono text-[0.7rem]">
            {Object.entries(state.vars).map(([k, v]) => <Fragment key={k}><dt className="text-muted-foreground">{`{${k}}`}</dt><dd className="truncate">{v}</dd></Fragment>)}
          </dl>
        </details>
      )}
      <form className="flex items-center gap-2 border-t border-border/60 p-3" onSubmit={(e) => { e.preventDefault(); if (text.trim()) send(text.trim()); }}>
        <input value={text} onChange={(e) => setText(e.target.value)} disabled={!state || state.done} placeholder={state?.done ? (skipped ? "Chatbot başlamadı" : "Chatbot bitti") : "Müşteri olarak yazın"} className="h-10 min-w-0 flex-1 rounded-full border border-border/60 bg-muted/40 px-4 text-sm outline-none focus:border-ring/50 disabled:opacity-60" />
        <button type="submit" disabled={!text.trim() || !state || state.done || busy} className="flex size-10 items-center justify-center rounded-full bg-emerald-600 text-white shadow-sm disabled:opacity-40" aria-label="Gönder"><SendHorizontal className="size-4" /></button>
      </form>
    </aside>
  );
}

function Dot({ d = 0 }: { d?: number }) {
  return <span className="size-1.5 animate-bounce rounded-full bg-slate-400" style={{ animationDelay: `${d}ms` }} />;
}

function BotLine({ out, onPick, active }: { out: SimOutput; onPick: (o: BotOption) => void; active: boolean }) {
  const bubble = "max-w-[85%] rounded-xl rounded-tl-sm bg-white px-3 py-1.5 text-sm text-slate-900 shadow-sm dark:bg-[#202c33] dark:text-slate-100";
  switch (out.kind) {
    case "text":
      return <div><p className={cn(bubble, "whitespace-pre-wrap")}>{waText(out.text ?? "")}</p></div>;
    case "media":
      return <div><div className={bubble}><p className="mb-1 truncate font-mono text-[0.65rem] text-slate-500">{out.detail}</p>{out.text && <p className="whitespace-pre-wrap">{waText(out.text)}</p>}</div></div>;
    case "menu":
      return (
        <div className="max-w-[85%] space-y-1">
          <p className={cn(bubble, "max-w-none whitespace-pre-wrap")}>{waText(out.text ?? "")}</p>
          {out.style === "list" ? (
            <div className="overflow-hidden rounded-xl bg-white shadow-sm dark:bg-[#202c33]">
              <p className="border-b border-slate-200 px-3 py-1.5 text-center text-xs font-medium text-sky-600 dark:border-slate-600 dark:text-sky-400">☰ {out.detail || "Seçenekler"}</p>
              {out.options?.map((o) => (
                <button key={o.id} type="button" disabled={!active} onClick={() => onPick(o)} className="block w-full border-b border-slate-100 px-3 py-1.5 text-left last:border-0 enabled:hover:bg-slate-50 disabled:opacity-60 dark:border-slate-700 dark:enabled:hover:bg-slate-700/40">
                  <span className="block text-sm text-slate-900 dark:text-slate-100">{o.label}</span>
                  {o.description && <span className="block text-[0.7rem] text-slate-500">{o.description}</span>}
                </button>
              ))}
            </div>
          ) : (
            out.options?.map((o) => (
              <button key={o.id} type="button" disabled={!active} onClick={() => onPick(o)} className="block w-full rounded-xl bg-white py-1.5 text-center text-sm font-medium text-sky-600 shadow-sm enabled:hover:bg-slate-50 disabled:opacity-60 dark:bg-[#202c33] dark:text-sky-400">{o.label}</button>
            ))
          )}
        </div>
      );
    default: {
      const map: Record<string, { icon: typeof Tag; text: string }> = {
        handoff: { icon: UserRound, text: `Temsilciye aktarıldı${out.detail && out.detail !== "0" ? " (ekibe)" : ""}${out.text ? `: ${out.text}` : ""}` },
        end: { icon: Flag, text: out.detail === "resolve" ? "Chatbot bitti, sohbet kapandı" : "Chatbot bitti, sohbet açık" },
        tag: { icon: Tag, text: `Etiketlendi: ${[out.text, out.detail].filter(Boolean).join(" · ")}` },
        callback: { icon: PhoneCall, text: `Geri arama talebi açıldı${out.text ? `: ${out.text}` : ""}` },
        survey: { icon: Star, text: "Anket gönderildi" },
        api: { icon: Globe, text: `Dış sorgu ${out.detail}` },
      };
      if (out.kind === "skip" || out.kind === "note") {
        const Icon = out.kind === "skip" ? BellOff : Info;
        return (
          <p className={cn("mx-auto flex max-w-[95%] items-start gap-2 rounded-xl px-3 py-2 text-[0.72rem] leading-relaxed shadow-sm", out.kind === "skip" ? "bg-indigo-50 text-indigo-800 dark:bg-indigo-950/70 dark:text-indigo-200" : "bg-amber-50 text-amber-900 dark:bg-amber-950/60 dark:text-amber-100")}>
            <Icon className="mt-0.5 size-3.5 shrink-0" /> <span>{out.text}</span>
          </p>
        );
      }
      const m = map[out.kind] ?? { icon: Tag, text: out.kind };
      return (
        <p className="mx-auto flex w-fit max-w-[90%] items-center gap-1.5 rounded-full bg-white/80 px-3 py-1 text-[0.7rem] font-medium text-slate-600 shadow-sm dark:bg-slate-800/80 dark:text-slate-300">
          <m.icon className="size-3.5 shrink-0" /> {m.text}
        </p>
      );
    }
  }
}

// Now in Turkey time, whatever the computer's clock is set to.
function nowClock(): string {
  return new Date().toLocaleTimeString("tr-TR", { timeZone: "Europe/Istanbul", hour: "2-digit", minute: "2-digit", hour12: false });
}

function nowDay(): number {
  const w = new Date().toLocaleDateString("en-GB", { timeZone: "Europe/Istanbul", weekday: "short" });
  return Math.max(0, ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].indexOf(w));
}
