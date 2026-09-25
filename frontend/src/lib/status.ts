// One palette for an agent's status, used the same way on every page so
// the eye learns it once: available green, talking red (the line is
// busy), break blue, backoffice violet, do-not-disturb amber, off or
// unregistered grey.

export type StatusKey = "available" | "talking" | "break" | "backoffice" | "dnd" | "unregistered" | "off";
export type StatusTone = "green" | "red" | "blue" | "violet" | "amber" | "slate";

export const STATUS_COLOR: Record<StatusKey, { label: string; tone: StatusTone; dot: string; chip: string; bar: string }> = {
  available: { label: "Müsait", tone: "green", dot: "bg-success", chip: "bg-success/10 text-success", bar: "bg-success" },
  talking: { label: "Görüşmede", tone: "red", dot: "bg-destructive", chip: "bg-destructive/10 text-destructive", bar: "bg-destructive" },
  break: { label: "Molada", tone: "blue", dot: "bg-sky-500", chip: "bg-sky-500/10 text-sky-600 dark:text-sky-400", bar: "bg-sky-500" },
  backoffice: { label: "Backoffice", tone: "violet", dot: "bg-violet-500", chip: "bg-violet-500/10 text-violet-500", bar: "bg-violet-500" },
  dnd: { label: "Rahatsız etmeyin", tone: "amber", dot: "bg-warning", chip: "bg-warning/12 text-warning", bar: "bg-warning" },
  unregistered: { label: "Kayıtsız", tone: "slate", dot: "bg-muted-foreground/50", chip: "bg-muted text-muted-foreground", bar: "bg-muted-foreground/40" },
  off: { label: "Mesai dışı", tone: "slate", dot: "bg-muted-foreground/40", chip: "bg-muted text-muted-foreground", bar: "bg-border" },
};

// pbxStatusKey maps the hosted PBX's status words to the palette.
export function pbxStatusKey(status: string): StatusKey {
  switch (status) {
    case "AVAILABLE": return "available";
    case "TALKING": return "talking";
    case "BREAK": return "break";
    case "BACKOFFICE": return "backoffice";
    case "SS_DND": return "dnd";
    case "OFF_SHIFT": return "off";
    default: return "unregistered";
  }
}

// dotClass and chipClass resolve a tone alone, for places that only have one.
export function dotClass(tone: StatusTone): string {
  return tone === "green" ? "bg-success" : tone === "red" ? "bg-destructive" : tone === "blue" ? "bg-sky-500" : tone === "violet" ? "bg-violet-500" : tone === "amber" ? "bg-warning" : "bg-muted-foreground/50";
}

export function chipClass(tone: StatusTone): string {
  return tone === "green" ? "bg-success/10 text-success" : tone === "red" ? "bg-destructive/10 text-destructive" : tone === "blue" ? "bg-sky-500/10 text-sky-600 dark:text-sky-400" : tone === "violet" ? "bg-violet-500/10 text-violet-500" : tone === "amber" ? "bg-warning/12 text-warning" : "bg-muted/70 text-muted-foreground";
}
