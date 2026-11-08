import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";

// WindowedList draws only the rows near the visible part of a long,
// scrolling list; the rest are stood in for by two empty blocks of the
// right height, so the scrollbar still matches the whole list. Rows may
// differ in height: each drawn row is measured and remembered by its key,
// rows not yet seen count as `estimate` pixels. A list of fewer than
// `from` rows is drawn whole.
//
// scroller is the element that scrolls; the list may sit anywhere inside
// it, below other content.
export function WindowedList<T>({
  scroller,
  items,
  keyOf,
  estimate,
  render,
  overscan = 600,
  from = 80,
}: {
  scroller: RefObject<HTMLElement | null>;
  items: T[];
  keyOf: (item: T) => string | number;
  estimate: number;
  render: (item: T, index: number) => ReactNode;
  overscan?: number;
  from?: number;
}) {
  const whole = items.length < from;
  const heights = useRef(new Map<string, number>());
  const box = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ top: 0, height: 0 });
  const [measured, setMeasured] = useState(0);

  // One observer for every drawn row; a changed height redraws once per frame.
  const observerRef = useRef<ResizeObserver | null>(null);
  const measureFrameRef = useRef(0);
  const observeRow = useCallback((el: HTMLElement, on: boolean) => {
    if (typeof ResizeObserver === "undefined") return;
    if (!on) {
      observerRef.current?.unobserve(el);
      return;
    }
    if (!observerRef.current) {
      observerRef.current = new ResizeObserver((entries) => {
        let changed = false;
        for (const e of entries) {
          const key = (e.target as HTMLElement).dataset.wkey;
          const h = Math.round(e.borderBoxSize?.[0]?.blockSize ?? e.contentRect.height);
          if (key !== undefined && h > 0 && heights.current.get(key) !== h) {
            heights.current.set(key, h);
            changed = true;
          }
        }
        if (changed && !measureFrameRef.current) {
          measureFrameRef.current = requestAnimationFrame(() => {
            measureFrameRef.current = 0;
            setMeasured((n) => n + 1);
          });
        }
      });
    }
    observerRef.current.observe(el);
  }, []);

  useEffect(
    () => () => {
      observerRef.current?.disconnect();
      observerRef.current = null;
      if (measureFrameRef.current) {
        cancelAnimationFrame(measureFrameRef.current);
        measureFrameRef.current = 0;
      }
    },
    [],
  );

  useLayoutEffect(() => {
    const el = scroller.current;
    if (whole || !el) return;
    let frame = 0;
    const read = () => {
      frame = 0;
      const offset = offsetInside(box.current, el);
      setView((v) => {
        const top = Math.max(0, el.scrollTop - offset);
        const height = el.clientHeight;
        return v.top === top && v.height === height ? v : { top, height };
      });
    };
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(read);
    };
    read();
    el.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll, { passive: true });
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(onScroll);
    resize?.observe(el);
    if (box.current) resize?.observe(box.current);
    return () => {
      el.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      resize?.disconnect();
      if (frame) cancelAnimationFrame(frame);
    };
  }, [scroller, whole, items.length]);

  const range = useMemo(() => {
    if (whole) return { start: 0, end: items.length, before: 0, after: 0 };
    const rowHeights = new Array<number>(items.length);
    let totalHeight = 0;
    for (let i = 0; i < items.length; i++) {
      const h = heights.current.get(String(keyOf(items[i]))) ?? estimate;
      rowHeights[i] = h;
      totalHeight += h;
    }
    const fallbackHeight = typeof window !== "undefined" && window.innerHeight > 0 ? window.innerHeight : 800;
    const viewHeight = view.height > 0 ? view.height : fallbackHeight;
    const maxTop = Math.max(0, totalHeight - viewHeight);
    const effectiveTop = Math.max(0, Math.min(view.top, maxTop));
    const lo = effectiveTop - overscan;
    const hi = effectiveTop + viewHeight + overscan;
    let y = 0;
    let start = -1;
    let before = 0;
    let end = items.length;
    for (let i = 0; i < items.length; i++) {
      const h = rowHeights[i];
      if (start < 0 && y + h > lo) {
        start = i;
        before = y;
      }
      if (start >= 0 && y >= hi) {
        end = i;
        break;
      }
      y += h;
    }
    if (start < 0) {
      start = 0;
      before = 0;
    }
    let after = 0;
    for (let i = end; i < items.length; i++) after += rowHeights[i];
    return { start, end, before, after };
    // measured: a row's height changed, so the sums move.
  }, [whole, items, keyOf, estimate, overscan, view, measured]); // eslint-disable-line react-hooks/exhaustive-deps -- heights live in a ref; measured is what tells the sums to redo

  return (
    <div ref={box}>
      {range.before > 0 && <div style={{ height: range.before }} aria-hidden />}
      {items.slice(range.start, range.end).map((item, i) => {
        const key = String(keyOf(item));
        return (
          <Measured key={key} id={key} observe={whole ? null : observeRow}>
            {render(item, range.start + i)}
          </Measured>
        );
      })}
      {range.after > 0 && <div style={{ height: range.after }} aria-hidden />}
    </div>
  );
}

function offsetInside(boxEl: HTMLElement | null, scrollerEl: HTMLElement): number {
  if (!boxEl) return 0;
  if (boxEl.offsetParent === scrollerEl) return Math.max(0, boxEl.offsetTop);
  if (boxEl.offsetParent && boxEl.offsetParent === scrollerEl.offsetParent) {
    return Math.max(0, boxEl.offsetTop - scrollerEl.offsetTop);
  }
  return 0;
}

function Measured({ id, observe, children }: { id: string; observe: ((el: HTMLElement, on: boolean) => void) | null; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || !observe) return;
    observe(el, true);
    return () => observe(el, false);
  }, [observe, id]);
  return (
    <div ref={ref} data-wkey={id}>
      {children}
    </div>
  );
}
