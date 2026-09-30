// Dates and times as the panel shows them: always in Istanbul time, the
// company's clock, whatever the viewer's computer is set to. "Today" and
// "yesterday" are Istanbul days too, and so are the dates the filters send.

export const ZONE = "Europe/Istanbul";

export type TimeInput = string | number | Date;

function toDate(value: TimeInput): Date {
  return value instanceof Date ? value : new Date(value);
}

// fmt formats a moment in Turkish, in Istanbul time.
export function fmt(value: TimeInput, options: Intl.DateTimeFormatOptions): string {
  return toDate(value).toLocaleString("tr-TR", { ...options, timeZone: ZONE });
}

// dayKey is the Istanbul calendar day of a moment, as YYYY-MM-DD.
export function dayKey(value: TimeInput): string {
  return toDate(value).toLocaleDateString("en-CA", { timeZone: ZONE, year: "numeric", month: "2-digit", day: "2-digit" });
}

// todayKey is today's Istanbul day as YYYY-MM-DD.
export function todayKey(): string {
  return dayKey(Date.now());
}

// addDays moves a YYYY-MM-DD day by n days (n may be negative).
export function addDays(key: string, n: number): string {
  const [y, m, d] = key.split("-").map(Number);
  const t = new Date(Date.UTC(y, m - 1, d + n));
  return t.toISOString().slice(0, 10);
}

export function sameDay(a: TimeInput, b: TimeInput): boolean {
  return dayKey(a) === dayKey(b);
}

export function isToday(value: TimeInput): boolean {
  return dayKey(value) === todayKey();
}

export function isYesterday(value: TimeInput): boolean {
  return dayKey(value) === addDays(todayKey(), -1);
}

function thisYear(value: TimeInput): boolean {
  return dayKey(value).slice(0, 4) === todayKey().slice(0, 4);
}

// clockTime is "18:30", or "" when there is no moment.
export function clockTime(value?: TimeInput | null): string {
  if (value == null || value === "") return "";
  return fmt(value, { hour: "2-digit", minute: "2-digit" });
}

// clockTimeSeconds is "18:30:05".
export function clockTimeSeconds(value: TimeInput): string {
  return fmt(value, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

// shortDate is "30.09". Built from its parts: the browser's own Turkish
// pattern for a day and month is "30/09", unlike the full "30.09.2026".
export function shortDate(value: TimeInput): string {
  const [, month, day] = dayKey(value).split("-");
  return `${day}.${month}`;
}

// numericDate is "30.09.2026".
export function numericDate(value: TimeInput): string {
  return fmt(value, { day: "2-digit", month: "2-digit", year: "numeric" });
}

// shortDateTime is "30.09 18:30".
export function shortDateTime(value: TimeInput): string {
  return `${shortDate(value)} ${clockTime(value)}`;
}

// numericDateTime is "30.09.2026 18:30".
export function numericDateTime(value: TimeInput): string {
  return `${numericDate(value)} ${clockTime(value)}`;
}

// longDate is "30 Eylül", with the year only when it is not this year.
export function longDate(value: TimeInput): string {
  return fmt(value, { day: "numeric", month: "long", year: thisYear(value) ? undefined : "numeric" });
}

// shortMonthDate is "30 Eyl", with the year only when it is not this year.
export function shortMonthDate(value: TimeInput): string {
  return fmt(value, { day: "numeric", month: "short", year: thisYear(value) ? undefined : "numeric" });
}

// dayName is "Bugün", "Dün" or the long date.
export function dayName(value: TimeInput): string {
  if (isToday(value)) return "Bugün";
  if (isYesterday(value)) return "Dün";
  return longDate(value);
}
