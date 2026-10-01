import { createContext, forwardRef, useCallback, useContext, useEffect, useId, useRef, useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";
import { createPortal } from "react-dom";
import { CalendarDays, TriangleAlert, type LucideIcon } from "lucide-react";
import { useDialogFocus, useTopmost } from "@/components/ui/windowStack";
import { cn } from "@/lib/utils";

export { useDirty } from "@/components/ui/windowStack";

const buttonBase =
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-xl text-sm font-medium whitespace-nowrap h-10 px-4 " +
  "transition-[color,background-color,border-color,box-shadow,transform] duration-200 ease-out " +
  "outline-none focus-visible:ring-4 focus-visible:ring-ring/25 active:scale-[0.985] " +
  "disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0";

const buttonVariants: Record<string, string> = {
  primary: "bg-primary text-primary-foreground shadow-sm hover:bg-primary/90 hover:shadow-md",
  secondary: "border border-border/70 bg-card text-foreground shadow-sm hover:border-border hover:bg-accent",
  danger: "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90 focus-visible:ring-destructive/25",
  ghost: "text-muted-foreground hover:bg-accent/70 hover:text-foreground",
};

export const Button = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger" | "ghost" }>(function Button(
  { variant = "primary", className = "", ...props },
  ref,
) {
  return <button ref={ref} className={cn(buttonBase, buttonVariants[variant], className)} {...props} />;
});

const fieldBase =
  "flex h-10 w-full min-w-0 rounded-xl border border-border/70 bg-muted/40 px-3.5 text-sm text-foreground shadow-sm outline-none " +
  "placeholder:text-muted-foreground/70 transition-[color,background-color,border-color,box-shadow] duration-200 ease-out " +
  "hover:border-border hover:bg-muted/60 focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20 " +
  "disabled:pointer-events-none disabled:opacity-50";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className = "", ...props },
  ref,
) {
  return <input ref={ref} className={cn(fieldBase, className)} {...props} />;
});

export function Select({ className = "", children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={cn(fieldBase, "bg-card", className)} {...props}>
      {children}
    </select>
  );
}

// toDMY renders an ISO day (YYYY-MM-DD) as gg.aa.yyyy; fromDMY parses it back
// and returns null when the text is not a real date.
function toDMY(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  return m ? `${m[3]}.${m[2]}.${m[1]}` : "";
}
function fromDMY(text: string): string | null {
  const t = text.trim();
  if (t === "") return "";
  const m = /^(\d{1,2})[./-](\d{1,2})[./-](\d{4})$/.exec(t);
  if (!m) return null;
  const d = Number(m[1]), mo = Number(m[2]), y = Number(m[3]);
  const date = new Date(y, mo - 1, d);
  if (date.getFullYear() !== y || date.getMonth() !== mo - 1 || date.getDate() !== d) return null;
  const p = (n: number) => String(n).padStart(2, "0");
  return `${y}-${p(mo)}-${p(d)}`;
}

// DateField is a day picker that always reads gg.aa.yyyy whatever the
// browser's locale: a text box that accepts typed dates plus a calendar
// button that opens the native picker. Values in and out are YYYY-MM-DD.
export function DateField({
  value,
  onChange,
  min,
  max,
  className = "",
  title,
}: {
  value: string;
  onChange: (iso: string) => void;
  min?: string;
  max?: string;
  className?: string;
  title?: string;
}) {
  const [text, setText] = useState(() => toDMY(value));
  const picker = useRef<HTMLInputElement>(null);
  useEffect(() => {
    setText(toDMY(value));
  }, [value]);

  function commit() {
    const iso = fromDMY(text);
    if (iso === null || (iso !== "" && ((min && iso < min) || (max && iso > max)))) {
      setText(toDMY(value));
      return;
    }
    if (iso !== value) onChange(iso);
  }

  function openPicker() {
    const p = picker.current;
    if (!p) return;
    try {
      p.showPicker();
    } catch {
      p.click();
    }
  }

  return (
    <div className={cn("relative", className)} data-tip={title}>
      <input
        className={cn(fieldBase, "pr-9 font-mono tabular-nums")}
        value={text}
        placeholder="gg.aa.yyyy"
        inputMode="numeric"
        onChange={(e) => setText(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") commit();
        }}
      />
      <input
        ref={picker}
        type="date"
        tabIndex={-1}
        aria-hidden
        className="pointer-events-none absolute inset-0 h-full w-full opacity-0"
        value={value}
        min={min}
        max={max}
        onChange={(e) => e.target.value && onChange(e.target.value)}
      />
      <button
        type="button"
        onClick={openPicker}
        aria-label="Takvimden seç"
        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground" data-tip="Takvimden seç">
        <CalendarDays className="size-4" />
      </button>
    </div>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

export function Card({ title, icon: Icon, actions, children }: { title?: string; icon?: LucideIcon; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col rounded-2xl bg-card text-card-foreground shadow-sm ring-1 ring-border/60">
      {(title || actions) && (
        <header className="flex items-center justify-between gap-4 border-b border-border/60 px-5 py-3.5">
          {title && (
            <h2 className="flex items-center gap-2.5 text-sm font-semibold tracking-tight">
              {Icon && <span className="flex size-8 items-center justify-center rounded-xl bg-primary/10 text-primary"><Icon className="size-4" /></span>}
              {title}
            </h2>
          )}
          {actions}
        </header>
      )}
      <div className="p-5">{children}</div>
    </section>
  );
}

const badgeTones: Record<string, string> = {
  slate: "bg-muted text-muted-foreground",
  green: "bg-success/15 text-success",
  red: "bg-destructive/15 text-destructive",
  amber: "bg-warning/15 text-warning",
  blue: "bg-sky-500/12 text-sky-600 dark:text-sky-400",
  violet: "bg-violet-500/12 text-violet-500",
};

export function Badge({ tone = "slate", children }: { tone?: "slate" | "green" | "red" | "amber" | "blue" | "violet"; children: ReactNode }) {
  return <span className={cn("inline-block rounded-full px-2 py-0.5 text-xs font-medium", badgeTones[tone])}>{children}</span>;
}

// Windows are drawn at the top of the page, not where they are opened, so a
// parent that clips or transforms its content (a chat composer, a card
// with a blur) cannot trap them. An overlay that opens windows of its own
// says how high it sits through Layer, and a window opened from it (or
// from another window) stays above it.
export const Layer = createContext(0);

function useLayer(base: number): number {
  return Math.max(base, useContext(Layer) + 1);
}

// Modal is a window over the page. Escape or a click outside closes it;
// when dirty is set, it asks first so unsaved changes are not lost by
// accident (the window's own cancel button still closes at once).
export function Modal({
  open,
  onClose,
  title,
  description,
  size = "md",
  footer,
  dirty = false,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  size?: "md" | "lg";
  footer?: ReactNode;
  dirty?: boolean;
  children: ReactNode;
}) {
  const z = useLayer(50);
  const isTop = useTopmost(open);
  const box = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const descId = useId();
  const [asking, setAsking] = useState(false);
  // A press that starts inside and ends outside (selecting text) must not
  // close the window, so the press has to start on the backdrop too.
  const pressedOutside = useRef(false);
  useDialogFocus(open, box, isTop);
  const requestClose = useCallback(() => {
    if (dirty) setAsking(true);
    else onClose();
  }, [dirty, onClose]);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || !isTop()) return;
      e.preventDefault();
      requestClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, isTop, requestClose]);
  useEffect(() => {
    if (!open) setAsking(false);
  }, [open]);
  if (!open) return null;
  const width = size === "lg" ? "max-w-3xl" : "max-w-md";
  return createPortal(
    <Layer.Provider value={z}>
    <div
      className="fixed inset-0 flex items-center justify-center bg-black/40 p-4"
      style={{ zIndex: z }}
      onMouseDown={(e) => { pressedOutside.current = e.target === e.currentTarget; }}
      onClick={(e) => { if (pressedOutside.current && e.target === e.currentTarget) requestClose(); }}
    >
      <div
        ref={box}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
        tabIndex={-1}
        className={cn("flex max-h-[88vh] w-full flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-xl outline-none", width)}
      >
        <header className="border-b border-border/60 px-5 py-4">
          <h2 id={titleId} className="text-base font-semibold">{title}</h2>
          {description && <p id={descId} className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
        </header>
        <div className="flex-1 overflow-y-auto p-5">{children}</div>
        {footer && <footer className="flex items-center justify-end gap-2 border-t border-border/60 px-5 py-3">{footer}</footer>}
      </div>
      <ConfirmDialog
        open={asking}
        tone="warning"
        title="Kaydedilmemiş değişiklikler var"
        description="Kapatırsan bu pencerede yaptığın değişiklikler kaybolur."
        confirmLabel="Kapat"
        cancelLabel="Düzenlemeye dön"
        onCancel={() => setAsking(false)}
        onConfirm={() => { setAsking(false); onClose(); }}
      />
    </div>
    </Layer.Provider>,
    document.body,
  );
}

export function Spinner() {
  return <div className="size-5 animate-spin rounded-full border-2 border-muted border-t-primary" />;
}

// Skeleton is a pulsing placeholder block shown while data loads.
export function Skeleton({ className = "" }: { className?: string }) {
  return <div className={`animate-pulse rounded-md bg-muted/70 ${className}`} />;
}

export function ErrorText({ children }: { children: ReactNode }) {
  if (!children) return null;
  return <p className="text-sm text-destructive">{children}</p>;
}

// FieldGroup labels a control that is not a single input (pills, buttons), so
// the caption is not a <label> that would steal focus on click.
export function FieldGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <span className="block text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}

// FieldHint is the small explanatory line under a field.
export function FieldHint({ children }: { children: ReactNode }) {
  return <p className="text-xs leading-relaxed text-muted-foreground">{children}</p>;
}

// CharCount shows used / max characters and turns amber near the limit and
// red at it. It counts code points so Turkish letters are not double counted.
export function CharCount({ value, max, className }: { value: string; max: number; className?: string }) {
  const used = [...value].length;
  const left = max - used;
  return (
    <span
      aria-live="polite"
      className={cn(
        "block text-[0.6875rem] tabular-nums transition-colors duration-200",
        left <= 0 ? "font-medium text-destructive" : left <= max * 0.1 ? "text-warning" : "text-muted-foreground/60",
        className,
      )}
    >
      {used} / {max}
    </span>
  );
}

export function Separator({ className }: { className?: string }) {
  return <hr className={cn("border-0 border-t border-border/60", className)} />;
}

// Notice is a quiet inline info box (system role note, copy hint).
export function Notice({ icon, children }: { icon?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-start gap-2 rounded-xl border border-border/60 bg-muted/50 px-3 py-2.5 text-xs text-muted-foreground">
      {icon && <span className="mt-0.5 shrink-0 [&_svg]:size-4">{icon}</span>}
      <span>{children}</span>
    </div>
  );
}

// EmptyState fills a list that has nothing to show.
export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 px-6 py-12 text-center">
      {icon && <span className="flex size-11 items-center justify-center rounded-2xl bg-muted text-muted-foreground [&_svg]:size-5">{icon}</span>}
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        {description && <p className="mx-auto max-w-sm text-xs leading-relaxed text-muted-foreground">{description}</p>}
      </div>
      {action}
    </div>
  );
}

// Pagination is the list footer; it renders nothing when everything fits on
// one page.
export function Pagination({ page, perPage, total, onChange }: { page: number; perPage: number; total: number; onChange: (page: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / perPage));
  if (total <= perPage) return null;
  const from = (page - 1) * perPage + 1;
  const to = Math.min(page * perPage, total);
  return (
    <div className="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-border/60 pt-3">
      <span className="text-xs tabular-nums text-muted-foreground">{from}-{to} / {total} kayıt</span>
      <div className="flex items-center gap-1">
        <Button variant="secondary" className="h-8 px-3 text-xs" disabled={page <= 1} onClick={() => onChange(page - 1)}>Önceki</Button>
        <span className="px-2 text-xs tabular-nums text-muted-foreground">{page} / {pages}</span>
        <Button variant="secondary" className="h-8 px-3 text-xs" disabled={page >= pages} onClick={() => onChange(page + 1)}>Sonraki</Button>
      </div>
    </div>
  );
}

// ConfirmDialog asks before something that cannot be undone. It sits above
// every other modal so it can be opened from inside one.
export function ConfirmDialog({
  open,
  title = "Emin misin?",
  description,
  confirmLabel = "Evet",
  cancelLabel = "Vazgeç",
  tone = "danger",
  busy = false,
  error,
  confirmText,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title?: string;
  description: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: "danger" | "warning";
  busy?: boolean;
  error?: string | null;
  // confirmText, when set, must be typed back before the action unlocks; for
  // steps that cannot be undone.
  confirmText?: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const z = useLayer(80);
  const isTop = useTopmost(open);
  const box = useRef<HTMLDivElement>(null);
  // A step that cannot be undone starts on the safe choice.
  const cancelRef = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  const descId = useId();
  const pressedOutside = useRef(false);
  const [typed, setTyped] = useState("");
  useEffect(() => {
    if (open) setTyped("");
  }, [open]);
  const locked = !!confirmText && typed.trim().toLocaleLowerCase("tr") !== confirmText.trim().toLocaleLowerCase("tr");
  useDialogFocus(open, box, isTop, cancelRef);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || busy || !isTop()) return;
      e.preventDefault();
      onCancel();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, busy, onCancel, isTop]);
  if (!open) return null;
  return createPortal(
    <div
      className="fixed inset-0 flex items-center justify-center bg-black/50 p-4"
      style={{ zIndex: z }}
      onMouseDown={(e) => { pressedOutside.current = e.target === e.currentTarget; }}
      onClick={(e) => { if (pressedOutside.current && e.target === e.currentTarget && !busy) onCancel(); }}
    >
      <div
        ref={box}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descId}
        tabIndex={-1}
        className="animate-in fade-in zoom-in-95 w-full max-w-sm rounded-2xl border border-border bg-card p-6 shadow-2xl outline-none duration-200"
      >
        <div className="flex items-start gap-3">
          <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-xl", tone === "danger" ? "bg-destructive/15 text-destructive" : "bg-warning/15 text-warning")}>
            <TriangleAlert className="size-5" />
          </span>
          <div className="space-y-1">
            <h3 id={titleId} className="text-base font-semibold leading-tight">{title}</h3>
            <div id={descId} className="text-sm leading-relaxed text-muted-foreground">{description}</div>
          </div>
        </div>
        {confirmText && (
          <label className="mt-4 block space-y-1.5 text-xs text-muted-foreground">
            <span>Onaylamak için <b className="font-semibold text-foreground">{confirmText}</b> yaz</span>
            <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus autoComplete="off" onKeyDown={(e) => { if (e.key === "Enter" && !locked && !busy) onConfirm(); }} />
          </label>
        )}
        {error && <p className="mt-3 text-xs text-destructive">{error}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <Button ref={cancelRef} variant="secondary" onClick={onCancel} disabled={busy}>{cancelLabel}</Button>
          <Button variant={tone === "danger" ? "danger" : "primary"} onClick={onConfirm} disabled={busy || locked} className={tone === "warning" ? "bg-warning text-black hover:bg-warning/90" : undefined}>
            {busy ? "Bekle..." : confirmLabel}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
