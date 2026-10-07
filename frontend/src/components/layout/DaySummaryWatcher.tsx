// DaySummaryWatcher shows the day's summary: when the person presses
// "Mesai Bitir" (the shift ends from inside the summary), and on its own
// five minutes before the working day ends, once a day, for whoever is
// still on shift.

import { useEffect, useState } from "react";
import { dayKey } from "@/lib/time";
import { userKey } from "@/lib/userStorage";
import { useShift } from "@/shift/ShiftContext";
import DaySummary from "./DaySummary";

// OPEN_DAY_SUMMARY is announced on window to show the summary before
// ending the shift.
export const OPEN_DAY_SUMMARY = "santral:day-summary";

export function openDaySummary() {
  window.dispatchEvent(new Event(OPEN_DAY_SUMMARY));
}

const SHOWN = "santral.daysummary.shown";

function shownToday(): boolean {
  try {
    return localStorage.getItem(userKey(SHOWN)) === dayKey(Date.now());
  } catch {
    return false;
  }
}

function markShown() {
  try {
    localStorage.setItem(userKey(SHOWN), dayKey(Date.now()));
  } catch {
    // shown again on the next load; harmless
  }
}

export default function DaySummaryWatcher() {
  const shift = useShift();
  const [open, setOpen] = useState<null | "ending" | "auto">(null);

  useEffect(() => {
    const onOpen = () => setOpen("ending");
    window.addEventListener(OPEN_DAY_SUMMARY, onOpen);
    return () => window.removeEventListener(OPEN_DAY_SUMMARY, onOpen);
  }, []);

  const summaryAt = shift.status?.summaryAt ? Date.parse(shift.status.summaryAt) : 0;
  const autoEndAt = shift.status?.autoEndAt ? Date.parse(shift.status.autoEndAt) : 0;
  useEffect(() => {
    if (!shift.active || !summaryAt) return;
    const check = () => {
      const now = Date.now();
      if (now >= summaryAt && (!autoEndAt || now < autoEndAt) && !shownToday()) {
        markShown();
        setOpen((cur) => cur ?? "auto");
      }
    };
    check();
    const t = window.setInterval(check, 20000);
    return () => window.clearInterval(t);
  }, [shift.active, summaryAt, autoEndAt]);

  if (!open) return null;
  return <DaySummary ending={open === "ending"} onClose={() => setOpen(null)} onEnd={shift.end} />;
}
