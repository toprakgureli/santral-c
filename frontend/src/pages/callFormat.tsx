import { Badge } from "../components/ui";

const directionLabel: Record<string, string> = {
  inbound: "Gelen",
  outbound: "Giden",
  internal: "Dahili",
};

const dispositionLabel: Record<string, string> = {
  in_progress: "Sürüyor",
  answered: "Cevaplandı",
  no_answer: "Cevapsız",
  busy: "Meşgul",
  failed: "Başarısız",
  canceled: "İptal",
  voicemail: "Sesli mesaj",
  elsewhere: "Başkası cevapladı", // rang here too, a teammate picked it up
  repeat: "Tekrar çaldı", // the queue offered the same call again, counted once
};

const dispositionTone: Record<string, "slate" | "green" | "red" | "amber" | "blue"> = {
  answered: "green",
  in_progress: "blue",
  no_answer: "amber",
  busy: "amber",
  failed: "red",
  canceled: "slate",
  voicemail: "slate",
  elsewhere: "slate",
  repeat: "slate",
};

export function Direction({ value }: { value: string }) {
  return <Badge tone={value === "inbound" ? "green" : value === "outbound" ? "blue" : "slate"}>{directionLabel[value] ?? value}</Badge>;
}

export function CallDisposition({ value }: { value: string }) {
  return <Badge tone={dispositionTone[value] ?? "slate"}>{dispositionLabel[value] ?? value}</Badge>;
}

// SHORT_LONG_SECONDS splits an answered call into a short vs a long
// conversation (kept in sync with the backend breakdown).
export const SHORT_LONG_SECONDS = 30;

export type QualityTone = "green" | "amber" | "slate" | "blue";

export interface CallQuality {
  tone: QualityTone;
  label: string;
  dot: string;
  text: string;
  border: string;
}

// callQuality classifies a call by disposition and length: answered calls of at
// least five seconds are real (green), shorter answered calls are brief (amber),
// and unanswered calls are grey.
export function callQuality(disposition: string, durationSeconds: number): CallQuality {
  if (disposition === "in_progress") {
    return { tone: "blue", label: "Sürüyor", dot: "bg-primary", text: "text-primary", border: "border-primary/60" };
  }
  if (disposition === "answered") {
    if (durationSeconds >= SHORT_LONG_SECONDS) {
      return { tone: "green", label: "Geçerli çağrı", dot: "bg-success", text: "text-success", border: "border-success/70" };
    }
    return { tone: "amber", label: "Geçersiz çağrı", dot: "bg-warning", text: "text-warning", border: "border-warning/70" };
  }
  return { tone: "slate", label: dispositionLabel[disposition] ?? "Cevapsız", dot: "bg-muted-foreground/50", text: "text-muted-foreground", border: "border-muted-foreground/30" };
}

// formatClock renders a running duration as H:MM:SS (or MM:SS under an hour).
export function formatClock(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  const mm = String(m).padStart(2, "0");
  const ss = String(s).padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

export function formatDuration(seconds: number): string {
  if (!seconds) return "0:00";
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

// formatStamp renders ISO or Bulutsantralim ("2017-08-03 12:30:32 +0300")
// timestamps, falling back to the raw value when unparseable.
export function formatStamp(value: string): string {
  if (!value) return "—";
  const d = new Date(value.replace(" ", "T"));
  if (isNaN(d.getTime())) return value;
  return d.toLocaleString("tr-TR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}
