import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import { numericDateTime } from "./time";

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
  return numericDateTime(d);
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
