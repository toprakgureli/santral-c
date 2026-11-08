// @vitest-environment jsdom
import { act, useRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import { WindowedList } from "./WindowedList";

interface Item {
  id: number;
  label: string;
}

function makeItems(count: number): Item[] {
  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    label: `Chat ${i + 1}`,
  }));
}

function Harness({ items }: { items: Item[] }) {
  const scroller = useRef<HTMLDivElement | null>(null);
  return (
    <div ref={scroller} data-testid="scroller">
      <WindowedList
        scroller={scroller}
        items={items}
        keyOf={(item) => item.id}
        estimate={77}
        render={(item) => <div data-testid={`row-${item.id}`}>{item.label}</div>}
      />
    </div>
  );
}

describe("WindowedList", () => {
  let container: HTMLDivElement | null = null;
  let root: Root | null = null;

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount();
      });
      root = null;
    }
    container?.remove();
    container = null;
  });

  it("renders a full viewport + overscan window for 120+ items even before clientHeight is measured", async () => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);

    const items = makeItems(120);
    await act(async () => {
      root?.render(<Harness items={items} />);
    });

    expect(container.querySelector('[data-testid="row-1"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="row-15"]')).not.toBeNull();
  });

  it("clamps stale scrollTop when switching from a large list to a smaller 100-item list so rows never vanish", async () => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);

    const large = makeItems(300);
    await act(async () => {
      root?.render(<Harness items={large} />);
    });

    const scroller = container.querySelector('[data-testid="scroller"]') as HTMLDivElement;
    await act(async () => {
      scroller.scrollTop = 18000;
      scroller.dispatchEvent(new Event("scroll"));
    });

    // Switch to a 100-item list (total height 7700px < 18000px)
    const medium = makeItems(100);
    await act(async () => {
      root?.render(<Harness items={medium} />);
    });

    expect(container.querySelector('[data-testid="row-100"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="row-85"]')).not.toBeNull();
  });
});
