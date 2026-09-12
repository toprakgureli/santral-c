import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn merges Tailwind classes, letting the last conflicting one win.
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// formatDateTime renders an ISO timestamp as a short Turkish date and time, or
// a dash when there is none.
export function formatDateTime(value?: string | null) {
  if (!value) return "—";
  const d = new Date(value);
  if (isNaN(d.getTime())) return value;
  return d.toLocaleString("tr-TR", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

export function initials(name?: string | null) {
  if (!name) return "—";
  return (
    name
      .split(" ")
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part.charAt(0).toUpperCase())
      .join("") || "—"
  );
}
