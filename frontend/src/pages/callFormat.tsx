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
};

const dispositionTone: Record<string, "slate" | "green" | "red" | "amber" | "blue"> = {
  answered: "green",
  in_progress: "blue",
  no_answer: "amber",
  busy: "amber",
  failed: "red",
  canceled: "slate",
  voicemail: "slate",
};

export function Direction({ value }: { value: string }) {
  return <Badge tone={value === "inbound" ? "green" : value === "outbound" ? "blue" : "slate"}>{directionLabel[value] ?? value}</Badge>;
}

export function CallDisposition({ value }: { value: string }) {
  return <Badge tone={dispositionTone[value] ?? "slate"}>{dispositionLabel[value] ?? value}</Badge>;
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
