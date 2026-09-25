import { useEffect, useState } from "react";
import { LogIn, LogOut } from "lucide-react";
import { useShift } from "@/shift/ShiftContext";
import { formatClock } from "@/pages/callFormat";
import { cn } from "@/lib/utils";

function hhmm(iso?: string) {
  if (!iso) return "";
  return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit", timeZone: "Europe/Istanbul" });
}

// ShiftButton starts and ends the agent's shift from the topbar. While on
// shift it shows the elapsed time, and after the working day is over it warns
// that the server will close the shift on its own.
export default function ShiftButton() {
  const shift = useShift();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);

  if (shift.loading) return null;

  const open = shift.status?.shift;
  if (!open) {
    return (
      <button
        type="button"
        onClick={() => void shift.start()}
        disabled={shift.busy}
        data-tip="Mesaiyi başlat; çağrı ekranı açılır"
        className={cn(
          "flex h-9 items-center gap-2 rounded-xl bg-success px-3 text-sm font-medium text-white shadow-sm transition hover:opacity-90",
          "disabled:pointer-events-none disabled:opacity-50",
        )}
      >
        <LogIn className="size-4" />
        <span>Mesai Başlat</span>
      </button>
    );
  }

  const elapsed = Math.max(0, Math.floor((now - Date.parse(open.startedAt)) / 1000));
  const reminderAt = shift.status?.reminderAt ? Date.parse(shift.status.reminderAt) : 0;
  const overtime = reminderAt > 0 && now >= reminderAt;

  return (
    <div className="flex items-center gap-2">
      {overtime && (
        <span className="hidden items-center rounded-lg bg-warning/15 px-2 py-1 text-xs font-medium text-warning md:flex" data-tip="Mesai saati doldu">
          {hhmm(shift.status?.reminderAt)} geçti · {hhmm(shift.status?.autoEndAt)}&apos;de otomatik biter
        </span>
      )}
      <span className="hidden font-mono text-sm tabular-nums text-muted-foreground sm:block" data-tip={`Mesai ${hhmm(open.startedAt)} başladı`}>
        {formatClock(elapsed)}
      </span>
      <button
        type="button"
        onClick={() => void shift.end()}
        disabled={shift.busy}
        data-tip="Mesaiyi bitir"
        className={cn(
          "flex h-9 items-center gap-2 rounded-xl bg-destructive px-3 text-sm font-medium text-destructive-foreground shadow-sm transition hover:opacity-90",
          "disabled:pointer-events-none disabled:opacity-50",
        )}
      >
        <LogOut className="size-4" />
        <span>Mesai Bitir</span>
      </button>
    </div>
  );
}
