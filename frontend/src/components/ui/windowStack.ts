// Shared behaviour of the panel's windows (forms and confirmations): only
// the topmost open window answers Escape and keeps Tab inside itself, focus
// moves into a window when it opens and goes back to where it was when it
// closes.

import { useCallback, useEffect, useRef, type RefObject } from "react";

// Open windows, oldest first.
const openWindows: symbol[] = [];

// useTopmost registers an open window and returns a check for whether it is
// the one on top.
export function useTopmost(open: boolean): () => boolean {
  const me = useRef(Symbol("window"));
  useEffect(() => {
    if (!open) return;
    const id = me.current;
    openWindows.push(id);
    return () => {
      const i = openWindows.lastIndexOf(id);
      if (i >= 0) openWindows.splice(i, 1);
    };
  }, [open]);
  return useCallback(() => openWindows[openWindows.length - 1] === me.current, []);
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusables(box: HTMLElement): HTMLElement[] {
  return [...box.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => el.getClientRects().length > 0);
}

// useDialogFocus moves focus into the window when it opens (to start, or to
// the window itself when nothing inside asked for it), keeps Tab and
// Shift+Tab inside while it is on top, and returns focus to the element that
// had it before once the window closes.
export function useDialogFocus(open: boolean, box: RefObject<HTMLElement | null>, isTop: () => boolean, start?: RefObject<HTMLElement | null>) {
  useEffect(() => {
    if (!open) return;
    const before = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const el = box.current;
    if (el && !el.contains(document.activeElement)) (start?.current ?? el).focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Tab" || !el || !isTop()) return;
      const items = focusables(el);
      if (items.length === 0) {
        e.preventDefault();
        el.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      if (!el.contains(active)) {
        e.preventDefault();
        first.focus();
      } else if (e.shiftKey && (active === first || active === el)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      if (before && document.contains(before)) before.focus();
    };
  }, [open, box, isTop, start]);
}

// useDirty reports whether value differs from what it was when the form
// first rendered, so a window can ask before dropping unsaved changes.
export function useDirty(value: unknown): boolean {
  const first = useRef<string | null>(null);
  const now = JSON.stringify(value);
  if (first.current === null) first.current = now;
  return now !== first.current;
}
