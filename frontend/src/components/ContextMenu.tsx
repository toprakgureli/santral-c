import { useEffect } from "react";

export type MenuItem = { label: string; onClick: () => void; disabled?: boolean; danger?: boolean };

export function ContextMenu({ x, y, items, onClose }: { x: number; y: number; items: MenuItem[]; onClose: () => void }) {
  useEffect(() => {
    const close = () => onClose();
    window.addEventListener("click", close);
    window.addEventListener("contextmenu", close);
    window.addEventListener("scroll", close, true);
    const onEsc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onEsc);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("contextmenu", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("keydown", onEsc);
    };
  }, [onClose]);

  return (
    <div
      className="fixed z-50 min-w-44 overflow-hidden rounded-xl border border-border bg-popover p-1 text-popover-foreground shadow-lg animate-in fade-in zoom-in-95 duration-100"
      style={{ left: Math.min(x, window.innerWidth - 190), top: Math.min(y, window.innerHeight - 10 - items.length * 36) }}
      onClick={(e) => e.stopPropagation()}
      onContextMenu={(e) => e.preventDefault()}
    >
      {items.map((it, i) => (
        <button
          key={i}
          disabled={it.disabled}
          onClick={() => { it.onClick(); onClose(); }}
          className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm disabled:opacity-40 ${
            it.danger ? "text-destructive hover:bg-destructive/10" : "hover:bg-accent"
          }`}
        >
          {it.label}
        </button>
      ))}
    </div>
  );
}
