// LiveTranscriptBox renders the real-time speech-to-text stream in a Spotify
// lyrics style: spoken lines glide upward smoothly, the active utterance stays
// bright and prominent in focus while older lines softly dim above it, and
// scrolling up pauses auto-scroll with a one-click "Canli akisa don" pill.

import { useEffect, useRef, useState } from "react";
import { Activity, ArrowDown, Captions, CaptionsOff, ChevronDown, ChevronUp, Maximize2, Minimize2, Sparkles } from "lucide-react";
import { cn } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { liveTranscript, useLiveTranscript, type SpeakerMode } from "@/softphone/liveTranscript";

function formatSec(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
}

export default function LiveTranscriptBox({
  size = "compact",
  className,
}: {
  size?: "compact" | "large";
  className?: string;
}) {
  const phone = useSoftphoneContext();
  const state = useLiveTranscript();
  const [collapsed, setCollapsed] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [showLogs, setShowLogs] = useState(false);
  const [pinned, setPinned] = useState(true);
  const [missedCount, setMissedCount] = useState(0);
  const scrollRef = useRef<HTMLDivElement>(null);
  const activeLineRef = useRef<HTMLDivElement>(null);
  const prevLenRef = useRef(0);
  const programmaticScrollRef = useRef(false);

  const inCall = phone.status === "in-call" || phone.status === "held";

  useEffect(() => {
    liveTranscript.syncCall({
      inCall,
      callId: phone.callId,
      peer: phone.peer,
      answeredAt: phone.answeredAt,
      muted: phone.muted,
      wave: phone.wave,
      streams: phone.streams,
      setRemotePcmTap: phone.setRemotePcmTap,
    });
  }, [inCall, phone.callId, phone.peer, phone.answeredAt, phone.muted, phone.wave, phone.streams, phone.setRemotePcmTap]);

  // Smooth Spotify-lyrics style glide when pinned to the live flow.
  useEffect(() => {
    const el = scrollRef.current;
    const newCount = state.lines.length;
    const added = Math.max(0, newCount - prevLenRef.current);
    prevLenRef.current = newCount;

    if (!el) return;
    if (pinned) {
      programmaticScrollRef.current = true;
      if (typeof el.scrollTo === "function") {
        el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
      } else {
        el.scrollTop = el.scrollHeight;
      }
      const t = window.setTimeout(() => {
        programmaticScrollRef.current = false;
      }, 260);
      if (missedCount !== 0) setMissedCount(0);
      return () => window.clearTimeout(t);
    } else if (added > 0) {
      setMissedCount((c) => c + added);
    }
  }, [state.lines, state.interimAgent, state.interimCustomer, pinned, missedCount]);

  function onScroll() {
    if (programmaticScrollRef.current) return;
    const el = scrollRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 36;
    setPinned(atBottom);
    if (atBottom && missedCount > 0) {
      setMissedCount(0);
    }
  }

  function scrollToBottom() {
    const el = scrollRef.current;
    setPinned(true);
    setMissedCount(0);
    if (el) {
      programmaticScrollRef.current = true;
      if (typeof el.scrollTo === "function") {
        el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
      } else {
        el.scrollTop = el.scrollHeight;
      }
      window.setTimeout(() => {
        programmaticScrollRef.current = false;
      }, 260);
    }
  }

  if (!inCall) return null;

  const isLarge = size === "large" || expanded;
  const hasInterim = Boolean(state.interimAgent) || Boolean(state.interimCustomer);
  const hasContent = state.lines.length > 0 || hasInterim;

  return (
    <div
      className={cn(
        "overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-b from-card/95 via-background/90 to-card/95 text-xs shadow-inner transition-all duration-300",
        isLarge ? "ring-1 ring-primary/20" : "",
        className,
      )}
    >
      {/* Top control bar */}
      <div className="flex items-center justify-between gap-1.5 border-b border-border/50 bg-muted/25 px-3 py-2">
        <button
          type="button"
          onClick={() => setCollapsed((v) => !v)}
          className="flex min-w-0 flex-1 items-center gap-2 text-left font-semibold text-foreground"
        >
          <span
            className={cn(
              "size-2 shrink-0 rounded-full",
              !state.enabled
                ? "bg-muted-foreground/40"
                : state.listening || state.simulating
                  ? "animate-pulse bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.8)]"
                  : "bg-amber-500",
            )}
          />
          <span className="truncate">Canlı Konuşma Akışı</span>
          {state.lines.length > 0 && (
            <span className="rounded-full bg-primary/15 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-primary tabular-nums">
              {state.lines.length}
            </span>
          )}
          {collapsed ? (
            <ChevronDown className="ml-auto size-3.5 text-muted-foreground" />
          ) : (
            <ChevronUp className="ml-auto size-3.5 text-muted-foreground" />
          )}
        </button>

        <button
          type="button"
          onClick={() => setShowLogs((v) => !v)}
          data-tip="Ses kanalı ve konuşma tanıma loglarını göster"
          className={cn(
            "flex items-center gap-1 rounded-lg px-2 py-0.5 text-[10px] font-semibold transition-colors",
            showLogs
              ? "bg-primary/20 text-primary ring-1 ring-primary/40"
              : "bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground",
          )}
        >
          <Activity className="size-3" />
          Loglar
        </button>

        <button
          type="button"
          onClick={liveTranscript.toggleSimulation}
          data-tip={state.simulating ? "Örnek diyalog akışını durdur" : "Örnek müşteri-temsilci diyalog akışını başlat"}
          className={cn(
            "flex items-center gap-1 rounded-lg px-2 py-0.5 text-[10px] font-semibold transition-colors",
            state.simulating
              ? "bg-violet-500/20 text-violet-400 ring-1 ring-violet-500/40"
              : "bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground",
          )}
        >
          <Sparkles className="size-3" />
          {state.simulating ? "Durdur" : "Canlı Örnek"}
        </button>

        {size === "compact" && !collapsed && (
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            aria-label={expanded ? "Küçült" : "Genişlet"}
            data-tip={expanded ? "Küçült" : "Genişlet"}
            className="flex size-6 items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            {expanded ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
          </button>
        )}

        <button
          type="button"
          onClick={() => liveTranscript.setEnabled(!state.enabled)}
          aria-label={state.enabled ? "Canlı transkripti kapat" : "Canlı transkripti aç"}
          data-tip={state.enabled ? "Canlı transkripti kapat" : "Canlı transkripti aç"}
          className={cn(
            "flex size-6 items-center justify-center rounded-lg transition-colors",
            state.enabled
              ? "bg-primary/15 text-primary hover:bg-primary/25"
              : "bg-muted/60 text-muted-foreground hover:bg-accent hover:text-foreground",
          )}
        >
          {state.enabled ? <Captions className="size-3.5" /> : <CaptionsOff className="size-3.5" />}
        </button>
      </div>

      {!collapsed && (
        <div className="relative">
          {/* Speaker attribution switch & live channel status */}
          {state.enabled && (
            <div className="flex flex-wrap items-center justify-between gap-1 border-b border-border/30 bg-muted/15 px-3 py-1 text-[10px] text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <span
                  className={cn(
                    "size-1.5 rounded-full",
                    state.remoteSttStatus === "ready" || state.remoteSttStatus === "transcribing"
                      ? "bg-emerald-500"
                      : state.remoteSttStatus === "loading"
                        ? "animate-pulse bg-amber-500"
                        : "bg-muted-foreground/50",
                  )}
                />
                <span>
                  {state.remoteSttStatus === "loading"
                    ? `Karşı taraf modeli (%${state.remoteSttProgress})`
                    : state.remoteSttStatus === "transcribing"
                      ? "Karşı taraf çevriliyor..."
                      : state.remoteSttStatus === "ready"
                        ? "Çift kanal STT aktif"
                        : "Mikrofon yönü:"}
                </span>
              </span>
              <div className="flex items-center gap-0.5 rounded-md bg-background/80 p-0.5 ring-1 ring-border/50">
                {(
                  [
                    { key: "auto", label: "Oto" },
                    { key: "agent", label: "Sen" },
                    { key: "customer", label: "Müşteri" },
                  ] as { key: SpeakerMode; label: string }[]
                ).map((m) => (
                  <button
                    key={m.key}
                    type="button"
                    onClick={() => liveTranscript.setSpeakerMode(m.key)}
                    className={cn(
                      "rounded px-1.5 py-0.5 font-medium transition-colors",
                      state.speakerMode === m.key
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {m.label}
                  </button>
                ))}
              </div>
            </div>
          )}

          {showLogs && (
            <div className="border-b border-border/50 bg-background/95 px-3 py-2 text-[10px]">
              <div className="mb-1.5 grid grid-cols-2 gap-2">
                <div className="rounded-lg bg-muted/40 p-1.5 ring-1 ring-border/40">
                  <div className="flex items-center justify-between text-muted-foreground">
                    <span>Müşteri Ses (WebRTC)</span>
                    <span className="font-mono">{(state.remoteRms * 100).toFixed(1)}%</span>
                  </div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
                    <div
                      className="h-full bg-amber-500 transition-all duration-150"
                      style={{ width: `${Math.min(100, Math.round(state.remoteRms * 450))}%` }}
                    />
                  </div>
                  <div className="mt-1 text-[9px] text-muted-foreground">
                    Kanal: {state.hasRemoteTrack ? "Bağlı (16kHz PCM)" : "Bekleniyor"} · Motor: {state.remoteSttStatus}
                  </div>
                </div>
                <div className="rounded-lg bg-muted/40 p-1.5 ring-1 ring-border/40">
                  <div className="flex items-center justify-between text-muted-foreground">
                    <span>Temsilci Ses (Mikrofon)</span>
                    <span className="font-mono">{(state.localRms * 100).toFixed(1)}%</span>
                  </div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
                    <div
                      className="h-full bg-primary transition-all duration-150"
                      style={{ width: `${Math.min(100, Math.round(state.localRms * 450))}%` }}
                    />
                  </div>
                  <div className="mt-1 text-[9px] text-muted-foreground">
                    Kanal: {state.hasLocalTrack ? "Bağlı" : "Varsayılan"} · WebSpeech: {state.listening ? "Aktif" : "Kapalı"}
                  </div>
                </div>
              </div>
              <div className="max-h-28 space-y-1 overflow-y-auto rounded-lg bg-muted/30 p-1.5 font-mono text-[9.5px] ring-1 ring-border/40">
                {state.logs.length === 0 ? (
                  <div className="text-muted-foreground">Henüz log kaydı yok.</div>
                ) : (
                  state.logs.slice(-20).map((lg) => (
                    <div
                      key={lg.id}
                      className={cn(
                        "leading-tight",
                        lg.level === "error"
                          ? "text-destructive"
                          : lg.level === "warn"
                            ? "text-amber-500"
                            : lg.level === "ok"
                              ? "text-emerald-500"
                              : "text-muted-foreground",
                      )}
                    >
                      [{lg.time}] {lg.text}
                    </div>
                  ))
                )}
              </div>
            </div>
          )}

          {state.error && (
            <div className="border-b border-destructive/20 bg-destructive/10 px-3 py-1.5 text-[11px] text-destructive">
              {state.error}
            </div>
          )}

          {/* Spotify-Lyrics Flowing Stream Container */}
          <div
            ref={scrollRef}
            onScroll={onScroll}
            className={cn(
              "space-y-3 overflow-y-auto scroll-smooth px-3.5 py-4 [mask-image:linear-gradient(to_bottom,transparent_0%,black_10%,black_90%,transparent_100%)]",
              isLarge ? "max-h-64 min-h-[11rem]" : "max-h-48 min-h-[8.5rem]",
            )}
          >
            {!state.enabled ? (
              <div className="flex h-28 flex-col items-center justify-center text-center text-xs text-muted-foreground">
                <span>Canlı transkript kapalı.</span>
                <button
                  type="button"
                  onClick={() => liveTranscript.setEnabled(true)}
                  className="mt-1.5 font-semibold text-primary hover:underline"
                >
                  Yeniden aç
                </button>
              </div>
            ) : !hasContent ? (
              <div className="flex h-28 flex-col items-center justify-center gap-2 text-center text-xs text-muted-foreground">
                <span className="max-w-[16rem] leading-relaxed">
                  {state.supported
                    ? "Konuşmalar başladığında sözler burada canlı olarak kayarak akar..."
                    : "Tarayıcı ses tanımayı desteklemiyor; üstteki Canlı Örnek butonuyla akışı test edebilirsin."}
                </span>
              </div>
            ) : (
              <>
                {state.lines.map((line, index) => {
                  const isAgent = line.speaker === "agent";
                  const isLatestFinal = !hasInterim && index === state.lines.length - 1;
                  const distanceFromEnd = state.lines.length - 1 - index + (hasInterim ? 1 : 0);

                  // Spotify lyrics effect: the active/latest line is bright and
                  // larger; older lines softly dim above it, but brighten on hover
                  // or when the agent scrolls up to read history.
                  const opacityClass = !pinned
                    ? "opacity-90 hover:opacity-100"
                    : isLatestFinal
                      ? "opacity-100"
                      : distanceFromEnd === 1
                        ? "opacity-65 hover:opacity-100"
                        : "opacity-40 hover:opacity-95";

                  return (
                    <div
                      key={line.id}
                      ref={isLatestFinal ? activeLineRef : undefined}
                      className={cn(
                        "group rounded-xl px-2.5 py-1.5 transition-all duration-300 ease-out",
                        isLatestFinal
                          ? isAgent
                            ? "scale-[1.01] bg-primary/12 shadow-sm ring-1 ring-primary/30"
                            : "scale-[1.01] bg-amber-500/12 shadow-sm ring-1 ring-amber-500/30"
                          : "hover:bg-muted/40",
                        opacityClass,
                      )}
                    >
                      <div className="mb-0.5 flex items-center gap-2 text-[10px]">
                        <span
                          className={cn(
                            "inline-flex items-center gap-1 font-bold uppercase tracking-wider",
                            isAgent ? "text-primary" : "text-amber-500",
                          )}
                        >
                          <span
                            className={cn(
                              "size-1.5 rounded-full",
                              isAgent ? "bg-primary" : "bg-amber-500",
                            )}
                          />
                          {isAgent ? "Sen (Temsilci)" : "Müşteri"}
                        </span>
                        <span className="ml-auto font-mono text-[10px] text-muted-foreground tabular-nums">
                          {formatSec(line.atSec)}
                        </span>
                      </div>
                      <p
                        className={cn(
                          "break-words transition-all duration-300",
                          isLatestFinal
                            ? isLarge
                              ? "text-[15px] font-semibold leading-snug text-foreground"
                              : "text-[13.5px] font-semibold leading-snug text-foreground"
                            : isLarge
                              ? "text-[13px] font-medium leading-snug text-foreground/85"
                              : "text-xs font-medium leading-snug text-foreground/85",
                        )}
                      >
                        {line.text}
                      </p>
                    </div>
                  );
                })}

                {/* Live speaking (interim) line - glowing Spotify active lyric */}
                {state.interimCustomer && (
                  <div
                    ref={activeLineRef}
                    className="animate-in fade-in slide-in-from-bottom-1 scale-[1.01] rounded-xl bg-amber-500/15 px-3 py-2 shadow-sm ring-1 ring-amber-500/40 duration-200"
                  >
                    <div className="mb-0.5 flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-wider text-amber-500">
                      <span className="size-1.5 animate-ping rounded-full bg-amber-500" />
                      <span>Müşteri konuşuyor...</span>
                    </div>
                    <p
                      className={cn(
                        "break-words font-bold leading-snug text-foreground",
                        isLarge ? "text-[15px]" : "text-[13.5px]",
                      )}
                    >
                      {state.interimCustomer}
                    </p>
                  </div>
                )}

                {state.interimAgent && (
                  <div
                    ref={activeLineRef}
                    className="animate-in fade-in slide-in-from-bottom-1 scale-[1.01] rounded-xl bg-primary/15 px-3 py-2 shadow-sm ring-1 ring-primary/40 duration-200"
                  >
                    <div className="mb-0.5 flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-wider text-primary">
                      <span className="size-1.5 animate-ping rounded-full bg-primary" />
                      <span>Sen konuşuyorsun...</span>
                    </div>
                    <p
                      className={cn(
                        "break-words font-bold leading-snug text-foreground",
                        isLarge ? "text-[15px]" : "text-[13.5px]",
                      )}
                    >
                      {state.interimAgent}
                    </p>
                  </div>
                )}
              </>
            )}
          </div>

          {/* Scroll-lock pill when the agent scrolls up to read earlier lines */}
          {!pinned && hasContent && (
            <div className="pointer-events-none absolute inset-x-0 bottom-2 flex justify-center">
              <button
                type="button"
                onClick={scrollToBottom}
                className="pointer-events-auto flex items-center gap-1.5 rounded-full bg-primary px-3 py-1 text-[11px] font-semibold text-primary-foreground shadow-lg transition hover:scale-105"
              >
                <ArrowDown className="size-3.5" />
                {missedCount > 0 ? `${missedCount} yeni satır · Canlı akışa dön` : "Canlı akışa dön"}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
