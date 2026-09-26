// Small pieces the WhatsApp settings screens share: a switch row, a word
// list input, device and people pickers, a copy-to-clipboard field.

import { useState, type ReactNode } from "react";
import { Check, Copy, X } from "lucide-react";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import type { WAAgent, WAChannel } from "@/whatsapp/types";

export function Switch({ on, onChange, disabled, label }: { on: boolean; onChange: (v: boolean) => void; disabled?: boolean; label?: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={cn("relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50", on ? "bg-primary" : "bg-muted-foreground/25")}
    >
      <span className={cn("inline-block size-5 rounded-full shadow-sm transition-transform", on ? "translate-x-5 bg-primary-foreground" : "translate-x-0.5 bg-white")} />
    </button>
  );
}

export function SwitchRow({ title, sub, on, onChange, disabled }: { title: string; sub?: ReactNode; on: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <div className="flex items-center gap-4 rounded-2xl bg-muted/30 px-4 py-3">
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{title}</p>
        {sub && <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{sub}</p>}
      </div>
      <Switch on={on} onChange={onChange} disabled={disabled} label={title} />
    </div>
  );
}

export function Words({ values: given, onChange, placeholder, disabled }: { values: string[]; onChange: (v: string[]) => void; placeholder?: string; disabled?: boolean }) {
  const values = given ?? [];
  const [draft, setDraft] = useState("");
  const add = () => {
    const v = draft.trim();
    if (!v) return;
    setDraft("");
    if (!values.some((x) => x.toLocaleLowerCase("tr") === v.toLocaleLowerCase("tr"))) onChange([...values, v]);
  };
  return (
    <div className={cn("flex min-h-10 flex-wrap items-center gap-1.5 rounded-xl border border-border/60 bg-card px-2 py-1.5", disabled && "opacity-60")}>
      {values.map((v) => (
        <span key={v} className="flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
          {v}
          {!disabled && <button type="button" onClick={() => onChange(values.filter((x) => x !== v))} aria-label={`${v} kaldır`}><X className="size-3" /></button>}
        </span>
      ))}
      {!disabled && (
        <input value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" || e.key === ",") { e.preventDefault(); add(); } }} onBlur={add} placeholder={values.length ? "" : placeholder} className="h-7 min-w-24 flex-1 bg-transparent px-1 text-sm outline-none" />
      )}
    </div>
  );
}

export function DevicePicker({ channels, value, onChange, disabled }: { channels: WAChannel[]; value: number[]; onChange: (v: number[]) => void; disabled?: boolean }) {
  if (channels.length === 0) return <p className="text-xs text-muted-foreground">Henüz cihaz yok.</p>;
  return (
    <div className="flex flex-wrap gap-1.5">
      {channels.map((c) => {
        const on = value.includes(c.id);
        return (
          <button key={c.id} type="button" disabled={disabled} onClick={() => onChange(on ? value.filter((x) => x !== c.id) : [...value, c.id])} className={cn("flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium ring-1 transition-colors disabled:opacity-60", on ? "bg-emerald-500/12 text-emerald-700 ring-emerald-600/30 dark:text-emerald-400" : "text-muted-foreground ring-border/60 hover:bg-accent")}>
            <span className={cn("flex size-4 items-center justify-center rounded-full border", on ? "border-emerald-600 bg-emerald-600 text-white" : "border-muted-foreground/40")}>{on && <Check className="size-3" />}</span>
            {c.name}
          </button>
        );
      })}
    </div>
  );
}

export function PeoplePicker({ people, value, onChange, disabled }: { people: WAAgent[]; value: number[]; onChange: (v: number[]) => void; disabled?: boolean }) {
  const [q, setQ] = useState("");
  const shown = people.filter((p) => !q || p.name.toLocaleLowerCase("tr").includes(q.toLocaleLowerCase("tr"))).sort((a, b) => a.name.localeCompare(b.name, "tr"));
  return (
    <div className="space-y-2">
      <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="İsim ara" className="h-9 w-full rounded-xl border border-border/60 bg-muted/40 px-3 text-sm outline-none focus:border-ring/50" />
      <div className="grid max-h-64 gap-1 overflow-y-auto sm:grid-cols-2">
        {shown.map((p) => {
          const on = value.includes(p.id);
          return (
            <button key={p.id} type="button" disabled={disabled} onClick={() => onChange(on ? value.filter((x) => x !== p.id) : [...value, p.id])} className={cn("flex items-center gap-2.5 rounded-xl px-2 py-1.5 text-left transition-colors", on ? "bg-primary/10 ring-1 ring-primary/30" : "hover:bg-accent/60")}>
              <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
              <span className="min-w-0 flex-1 truncate text-sm">{p.name}</span>
              <span className={cn("flex size-4 items-center justify-center rounded border", on ? "border-primary bg-primary text-primary-foreground" : "border-muted-foreground/40")}>{on && <Check className="size-3" />}</span>
            </button>
          );
        })}
        {shown.length === 0 && <p className="px-2 py-3 text-xs text-muted-foreground">Kimse bulunamadı. WhatsApp yetkisi olan kişiler burada görünür.</p>}
      </div>
    </div>
  );
}

export function CopyField({ label, value, secret }: { label: string; value: string; secret?: boolean }) {
  const [done, setDone] = useState(false);
  const [show, setShow] = useState(!secret);
  return (
    <div className="space-y-1">
      <p className="text-[0.7rem] font-medium text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2 rounded-xl border border-border/60 bg-muted/30 px-3 py-2">
        <code className="min-w-0 flex-1 truncate font-mono text-xs">{show ? value : "•".repeat(Math.min(24, value.length))}</code>
        {secret && <button type="button" onClick={() => setShow((v) => !v)} className="text-[0.7rem] font-medium text-muted-foreground hover:text-foreground">{show ? "Gizle" : "Göster"}</button>}
        <button type="button" onClick={() => { void navigator.clipboard?.writeText(value); setDone(true); window.setTimeout(() => setDone(false), 1500); }} data-tip="Kopyala" className="flex items-center gap-1 rounded-lg px-2 py-1 text-[0.7rem] font-semibold text-primary hover:bg-primary/10">
          {done ? <Check className="size-3.5" /> : <Copy className="size-3.5" />} {done ? "Kopyalandı" : "Kopyala"}
        </button>
      </div>
    </div>
  );
}

export function FormField({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="block text-xs font-medium text-muted-foreground">{label}</span>
      {children}
      {hint && <span className="block text-[0.7rem] leading-relaxed text-muted-foreground/80">{hint}</span>}
    </label>
  );
}

export const inputCls = "h-10 w-full rounded-xl border border-border/60 bg-card px-3 text-sm outline-none transition focus:border-ring/50 focus:ring-4 focus:ring-ring/15 disabled:opacity-60";
export const areaCls = "w-full resize-y rounded-xl border border-border/60 bg-card px-3 py-2 text-sm outline-none transition focus:border-ring/50 focus:ring-4 focus:ring-ring/15 disabled:opacity-60";

export function DeviceChips({ channels, ids }: { channels: WAChannel[]; ids: number[] }) {
  if (ids.length === 0) return <span className="text-[0.7rem] text-warning">Hiçbir cihazda değil</span>;
  return (
    <span className="flex flex-wrap gap-1">
      {ids.map((id) => {
        const c = channels.find((x) => x.id === id);
        return <span key={id} className="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[0.65rem] font-medium text-emerald-700 dark:text-emerald-400">{c?.name ?? `#${id}`}</span>;
      })}
    </span>
  );
}
