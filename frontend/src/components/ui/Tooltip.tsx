// TooltipLayer draws the app's own tooltip for any element carrying a
// data-tip attribute, in place of the browser's. One layer at the root
// listens for hover and keyboard focus, waits a moment, then shows the
// text above the element (below when there is no room, or beside it when
// data-tip-side says so). Nothing else needs to import anything: an
// element only needs data-tip="...".

import { useLayoutEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

type Tip = { text: string; x: number; y: number; side: "top" | "bottom" | "right" };

const DELAY = 220;
const GAP = 8;
const EDGE = 8;

export default function TooltipLayer() {
  const [tip, setTip] = useState<Tip | null>(null);
  const box = useRef<HTMLDivElement>(null);
  // The box is measured after it renders, then slid inside the viewport;
  // the arrow keeps pointing at the element.
  const [shift, setShift] = useState(0);

  useLayoutEffect(() => {
    const el = box.current;
    if (!el || !tip) return;
    setShift(0);
    const r = el.getBoundingClientRect();
    let dx = 0;
    if (r.left < EDGE) dx = EDGE - r.left;
    else if (r.right > window.innerWidth - EDGE) dx = window.innerWidth - EDGE - r.right;
    if (dx !== 0) setShift(dx);
  }, [tip]);

  useLayoutEffect(() => {
    let timer = 0;
    let current: HTMLElement | null = null;

    const place = (el: HTMLElement) => {
      const text = el.getAttribute("data-tip");
      if (!text) return;
      const r = el.getBoundingClientRect();
      const wanted = el.getAttribute("data-tip-side");
      let side: Tip["side"] = wanted === "right" && r.right + 160 < window.innerWidth ? "right" : "top";
      let x = r.left + r.width / 2;
      let y = r.top - GAP;
      if (side === "right") {
        x = r.right + GAP;
        y = r.top + r.height / 2;
      } else if (r.top < 48) {
        side = "bottom";
        y = r.bottom + GAP;
      }
      setTip({ text, x, y, side });
    };

    const show = (el: HTMLElement) => {
      current = el;
      window.clearTimeout(timer);
      timer = window.setTimeout(() => current === el && place(el), DELAY);
    };
    const hide = () => {
      current = null;
      window.clearTimeout(timer);
      setTip(null);
    };

    const onOver = (e: Event) => {
      const el = (e.target as Element | null)?.closest?.("[data-tip]") as HTMLElement | null;
      if (!el || el === current) return;
      if (el.getAttribute("data-tip")) show(el);
      else hide();
    };
    const onOut = (e: MouseEvent) => {
      const to = e.relatedTarget as Element | null;
      if (current && !(to && current.contains(to))) hide();
    };
    const onDown = () => hide();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") hide();
    };

    document.addEventListener("pointerover", onOver);
    document.addEventListener("focusin", onOver);
    document.addEventListener("pointerout", onOut);
    document.addEventListener("focusout", hide);
    document.addEventListener("pointerdown", onDown, true);
    document.addEventListener("keydown", onKey);
    window.addEventListener("scroll", hide, true);
    window.addEventListener("resize", hide);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener("pointerover", onOver);
      document.removeEventListener("focusin", onOver);
      document.removeEventListener("pointerout", onOut);
      document.removeEventListener("focusout", hide);
      document.removeEventListener("pointerdown", onDown, true);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", hide, true);
      window.removeEventListener("resize", hide);
    };
  }, []);

  if (!tip) return null;
  const vertical = tip.side !== "right";
  return (
    <div
      ref={box}
      role="tooltip"
      className={cn(
        "animate-in fade-in pointer-events-none fixed z-[90] flex items-center duration-100",
        tip.side === "top" && "flex-col",
        tip.side === "bottom" && "flex-col-reverse",
      )}
      style={{
        left: tip.x + shift,
        top: tip.y,
        transform: vertical ? `translate(-50%, ${tip.side === "top" ? "-100%" : "0"})` : "translateY(-50%)",
      }}
    >
      {tip.side === "right" && <span className="size-2 rotate-45 rounded-[2px] border-b border-l border-border bg-popover" style={{ marginRight: -5 }} />}
      <span className="w-max max-w-[280px] rounded-xl border border-border bg-popover px-3 py-1.5 text-center text-xs font-medium leading-snug text-popover-foreground shadow-lg">{tip.text}</span>
      {tip.side === "top" && <span className="size-2 rotate-45 rounded-[2px] border-r border-b border-border bg-popover" style={{ marginTop: -5, marginLeft: -shift * 2 }} />}
      {tip.side === "bottom" && <span className="size-2 rotate-45 rounded-[2px] border-t border-l border-border bg-popover" style={{ marginBottom: -5, marginLeft: -shift * 2 }} />}
    </div>
  );
}
