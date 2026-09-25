// VoiceMixer is the sound strip of a call: one quiet row with both voices
// as small bars and the other side's loudness, which opens into a slider
// when tapped. It comes in two sizes, the floating call bar's and the
// dashboard's, and remembers whether it was left open.

import { useState } from "react";
import { ChevronDown, Minus, Plus, Volume2 } from "lucide-react";
import VoiceBars from "@/components/layout/VoiceBars";
import { GAIN_MAX } from "@/softphone/audioGraph";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { cn } from "@/lib/utils";

const OPEN_KEY = "santral.mixer-open";

function loadOpen(): boolean {
  try {
    return localStorage.getItem(OPEN_KEY) === "1";
  } catch {
    return false;
  }
}

export default function VoiceMixer({ size = "compact", className }: { size?: "compact" | "large"; className?: string }) {
  const phone = useSoftphoneContext();
  const [open, setOpen] = useState(loadOpen);
  const large = size === "large";
  const pct = Math.round(phone.remoteGain * 100);
  const toggle = () => {
    setOpen((v) => {
      try {
        localStorage.setItem(OPEN_KEY, v ? "0" : "1");
      } catch {
        // storage unavailable; the choice still holds for this page
      }
      return !v;
    });
  };
  const barsClass = large ? "h-6 w-28" : "h-4 w-16";
  const bars = large ? 22 : 14;

  return (
    <div className={cn("rounded-xl bg-muted/40 ring-1 ring-border/40", className)}>
      <button
        type="button"
        onClick={toggle}
        aria-expanded={open}
        title={open ? "Ses ayarını gizle" : "Karşı tarafın sesini ayarla"}
        className={cn("flex w-full items-center gap-2 rounded-xl text-left transition-colors hover:bg-accent/50", large ? "px-3 py-2" : "px-2 py-1.5")}
      >
        <Lbl large={large}>Karşı</Lbl>
        <VoiceBars read={() => phone.spectrum("remote")} token="--primary" bars={bars} className={barsClass} />
        <Lbl large={large} muted={phone.muted}>Sen</Lbl>
        <VoiceBars read={() => (phone.muted ? null : phone.spectrum("local"))} token="--success" bars={bars} className={barsClass} />
        <span className="ml-auto flex items-center gap-1.5 text-muted-foreground">
          <Volume2 className={large ? "size-4" : "size-3.5"} />
          <span className={cn("font-semibold tabular-nums", large ? "text-sm" : "text-xs", pct > 100 ? "text-primary" : pct === 0 ? "text-destructive" : "text-foreground")}>%{pct}</span>
          <ChevronDown className={cn("transition-transform", large ? "size-4" : "size-3.5", open && "rotate-180")} />
        </span>
      </button>
      {open && (
        <div className={cn("flex items-center gap-2 border-t border-border/40", large ? "px-3 py-2" : "px-2 py-1.5")} title="Karşı tarafın sesi. %100 geldiği gibi; üstü yükseltir.">
          <button type="button" onClick={() => phone.setRemoteGain(phone.remoteGain - 0.1)} aria-label="Sesi kıs" className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-card text-muted-foreground ring-1 ring-border/50 transition-colors hover:bg-accent hover:text-foreground">
            <Minus className="size-3" />
          </button>
          <input
            type="range"
            min={0}
            max={GAIN_MAX * 100}
            step={5}
            value={pct}
            onChange={(e) => phone.setRemoteGain(Number(e.target.value) / 100)}
            aria-label="Karşı tarafın sesi"
            className="h-1.5 min-w-0 flex-1 cursor-pointer accent-primary"
          />
          <button type="button" onClick={() => phone.setRemoteGain(phone.remoteGain + 0.1)} aria-label="Sesi aç" className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-card text-muted-foreground ring-1 ring-border/50 transition-colors hover:bg-accent hover:text-foreground">
            <Plus className="size-3" />
          </button>
          <button type="button" onClick={() => phone.setRemoteGain(1)} disabled={pct === 100} title="Geldiği gibi (%100)" className="shrink-0 rounded-lg px-1.5 text-[0.65rem] font-medium text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-40">
            Sıfırla
          </button>
        </div>
      )}
    </div>
  );
}

function Lbl({ large, muted, children }: { large: boolean; muted?: boolean; children: React.ReactNode }) {
  return <span className={cn("shrink-0 font-medium uppercase tracking-wide", large ? "text-[0.65rem]" : "text-[0.6rem]", muted ? "text-muted-foreground/60 line-through" : "text-muted-foreground")}>{children}</span>;
}
