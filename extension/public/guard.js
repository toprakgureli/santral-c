// Decides whether a press on the mini widget may act. The widget's host
// element lives in the page's own document, so the page can fade it out,
// shrink it, clip it, move it under something else, or show it only for the
// instant of a click, and so trick the user into pressing a call button
// they never saw. A press counts only when:
//   - the host and its ancestors are fully shown (no display:none, hidden
//     visibility, lowered opacity, filter or clip, no transform on the host),
//   - the widget has a real size,
//   - it has been shown like that for a moment already (SETTLE_MS),
//   - the point pressed (or the middle of the button, for a key press) is
//     on the widget and not on something laid over it.
// This runs in the extension's own script world: the page cannot replace
// getComputedStyle or elementFromPoint for it.

(() => {
  const SETTLE_MS = 500;
  const POLL_MS = 150;

  function createGuard(host, env) {
    const win = env.window;
    const doc = env.document;
    const now = env.now || (() => Date.now());
    let shownSince = 0;

    function plainlyShown() {
      if (!host.isConnected) return false;
      const own = win.getComputedStyle(host);
      if (own.visibility !== "visible" || (own.transform && own.transform !== "none")) return false;
      for (let el = host; el; el = el.parentElement) {
        const cs = win.getComputedStyle(el);
        if (cs.display === "none") return false;
        if (Number(cs.opacity) < 1) return false;
        if (cs.filter && cs.filter !== "none") return false;
        if (cs.clipPath && cs.clipPath !== "none") return false;
      }
      const r = host.getBoundingClientRect();
      return r.width >= 24 && r.height >= 24;
    }

    // tick notes since when the widget has been plainly shown.
    function tick() {
      if (host.style.display === "none") {
        // Hidden by the widget itself (panel tab, panel closed).
        shownSince = 0;
        return;
      }
      if (plainlyShown()) {
        if (!shownSince) shownSince = now();
      } else {
        shownSince = 0;
      }
    }

    function pressPoint(e) {
      const pointer = typeof e.clientX === "number" && (e.clientX !== 0 || e.clientY !== 0 || e.detail > 0);
      if (pointer) return { x: e.clientX, y: e.clientY };
      const target = e.currentTarget && e.currentTarget.getBoundingClientRect ? e.currentTarget : host;
      const r = target.getBoundingClientRect();
      return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
    }

    // allows says whether event e, a real press inside the widget, may act.
    function allows(e) {
      tick();
      if (!shownSince || now() - shownSince < SETTLE_MS) return false;
      const p = pressPoint(e);
      // From the document, a point inside the closed shadow root is the
      // host itself; anything else means the press landed elsewhere.
      return doc.elementFromPoint(p.x, p.y) === host;
    }

    function watch() {
      tick();
      return win.setInterval(tick, POLL_MS);
    }

    return { allows, watch, tick };
  }

  globalThis.santralcGuard = { createGuard, SETTLE_MS };
})();
