import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";

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
  const observer = useMemo(() => {
    if (typeof ResizeObserver === "undefined") return null;
    let frame = 0;
    return new ResizeObserver((entries) => {
      let changed = false;
      for (const e of entries) {
        const key = (e.target as HTMLElement).dataset.wkey;
        const h = Math.round(e.borderBoxSize?.[0]?.blockSize ?? e.contentRect.height);
        if (key !== undefined && h > 0 && heights.current.get(key) !== h) {
          heights.current.set(key, h);
          changed = true;
        }
      }
      if (changed && !frame) {
        frame = requestAnimationFrame(() => {
          frame = 0;
          setMeasured((n) => n + 1);
        });
      }
    });
  }, []);
  useEffect(() => () => observer?.disconnect(), [observer]);

  useLayoutEffect(() => {
    const el = scroller.current;
    if (whole || !el) return;
    let frame = 0;
    const read = () => {
      frame = 0;
      // The list may start below other content inside the scroller.
      const offset = box.current ? box.current.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop : 0;
      setView((v) => {
        const top = el.scrollTop - offset;
        return v.top === top && v.height === el.clientHeight ? v : { top, height: el.clientHeight };
      });
    };
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(read);
    };
    read();
    el.addEventListener("scroll", onScroll, { passive: true });
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(onScroll);
    resize?.observe(el);
    return () => {
      el.removeEventListener("scroll", onScroll);
      resize?.disconnect();
      if (frame) cancelAnimationFrame(frame);
    };
  }, [scroller, whole]);

  const range = useMemo(() => {
    if (whole) return { start: 0, end: items.length, before: 0, after: 0 };
    const lo = view.top - overscan;
    const hi = view.top + view.height + overscan;
    let y = 0;
    let start = -1;
    let before = 0;
    let end = items.length;
    for (let i = 0; i < items.length; i++) {
      const h = heights.current.get(String(keyOf(items[i]))) ?? estimate;
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
      start = items.length;
      before = y;
    }
    let after = 0;
    for (let i = end; i < items.length; i++) after += heights.current.get(String(keyOf(items[i]))) ?? estimate;
    return { start, end, before, after };
    // measured: a row's height changed, so the sums move.
  }, [whole, items, keyOf, estimate, overscan, view, measured]); // eslint-disable-line react-hooks/exhaustive-deps -- heights live in a ref; measured is what tells the sums to redo

  return (
    <div ref={box}>
      {range.before > 0 && <div style={{ height: range.before }} aria-hidden />}
      {items.slice(range.start, range.end).map((item, i) => {
        const key = String(keyOf(item));
        return (
          <Measured key={key} id={key} observer={whole ? null : observer}>
            {render(item, range.start + i)}
          </Measured>
        );
      })}
      {range.after > 0 && <div style={{ height: range.after }} aria-hidden />}
    </div>
  );
}

function Measured({ id, observer, children }: { id: string; observer: ResizeObserver | null; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || !observer) return;
    observer.observe(el);
    return () => observer.unobserve(el);
  }, [observer]);
  return (
    <div ref={ref} data-wkey={id}>
      {children}
    </div>
  );
}
