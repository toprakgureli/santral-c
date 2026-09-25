// VoiceMixer is the sound panel of a call: both voices as bars, the other
// side above and the agent below, and a slider that turns the other side
// up from silent to well past full. It comes in two sizes, the floating
// call bar's and the dashboard's.

import { Minus, Plus, Volume2 } from "lucide-react";
import VoiceBars from "@/components/layout/VoiceBars";
import { GAIN_MAX } from "@/softphone/audioGraph";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { cn } from "@/lib/utils";

const REMOTE: [string, string] = ["#a78bfa", "#6366f1"];
const LOCAL: [string, string] = ["#6ee7b7", "#10b981"];

export default function VoiceMixer({ size = "compact", className }: { size?: "compact" | "large"; className?: string }) {
  const phone = useSoftphoneContext();
  const large = size === "large";
  const pct = Math.round(phone.remoteGain * 100);

  return (
    <div className={cn("space-y-2", className)}>
      <div className={cn("space-y-1.5 rounded-2xl bg-muted/40 ring-1 ring-border/40", large ? "px-4 py-3" : "px-2.5 py-2")}>
        <Row label="Karşı taraf" large={large} tone="text-violet-500">
          <VoiceBars read={() => phone.spectrum("remote")} colors={REMOTE} bars={large ? 40 : 24} className={large ? "h-12" : "h-6"} />
        </Row>
        <Row label="Sen" large={large} tone="text-success" muted={phone.muted}>
          <VoiceBars read={() => (phone.muted ? null : phone.spectrum("local"))} colors={LOCAL} bars={large ? 40 : 24} className={large ? "h-12" : "h-6"} />
        </Row>
      </div>
      <div className={cn("flex items-center gap-2", large && "gap-3")} title="Karşı tarafın sesi. %100 geldiği gibi; üstü yükseltir.">
        <span className={cn("flex shrink-0 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground", large ? "size-8" : "size-6 rounded-lg")}>
          <Volume2 className={large ? "size-4" : "size-3.5"} />
        </span>
        <button type="button" onClick={() => phone.setRemoteGain(phone.remoteGain - 0.1)} aria-label="Sesi kıs" className={cn("flex shrink-0 items-center justify-center rounded-lg bg-muted/70 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", large ? "size-8 rounded-xl" : "size-6")}>
          <Minus className={large ? "size-4" : "size-3"} />
        </button>
        <input
          type="range"
          min={0}
          max={GAIN_MAX * 100}
          step={5}
          value={pct}
          onChange={(e) => phone.setRemoteGain(Number(e.target.value) / 100)}
          aria-label="Karşı tarafın sesi"
          className={cn("min-w-0 flex-1 cursor-pointer accent-violet-500", large ? "h-2" : "h-1.5")}
        />
        <button type="button" onClick={() => phone.setRemoteGain(phone.remoteGain + 0.1)} aria-label="Sesi aç" className={cn("flex shrink-0 items-center justify-center rounded-lg bg-muted/70 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", large ? "size-8 rounded-xl" : "size-6")}>
          <Plus className={large ? "size-4" : "size-3"} />
        </button>
        <span className={cn("shrink-0 text-right font-semibold tabular-nums", large ? "w-14 text-sm" : "w-11 text-[0.7rem]", pct > 100 ? "text-violet-500" : "text-muted-foreground")}>%{pct}</span>
      </div>
      {large && pct > 100 && <p className="text-center text-[0.7rem] text-muted-foreground">Ses yükseltiliyor. Cızırtı duyarsan biraz geri al.</p>}
    </div>
  );
}

function Row({ label, large, tone, muted, children }: { label: string; large: boolean; tone: string; muted?: boolean; children: React.ReactNode }) {
  return (
    <div className={cn("grid items-center gap-x-3", large ? "grid-cols-[4.5rem_1fr]" : "grid-cols-[3.25rem_1fr]")}>
      <span className={cn("truncate font-medium", large ? "text-xs" : "text-[0.65rem]", muted ? "text-muted-foreground line-through" : tone)}>{label}</span>
      {children}
    </div>
  );
}
