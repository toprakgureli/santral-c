// The composer gives back what was being written when the agent returns to
// a chat, in the mode it was written in, and reports every change so the
// draft can be kept.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Composer from "@/components/whatsapp/Composer";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let host: HTMLDivElement;
let root: Root;

beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

function render(props: Partial<Parameters<typeof Composer>[0]>) {
  const all: Parameters<typeof Composer>[0] = {
    canReply: true, canNote: true, canTemplate: false, windowOpen: true, quickReplies: [], vars: {}, replyTo: null,
    onCancelReply: () => undefined, onSend: async () => undefined, onTemplate: () => undefined, onTyping: () => undefined,
    ...props,
  };
  act(() => root.render(<Composer {...all} />));
}

const box = () => host.querySelector("textarea") as HTMLTextAreaElement;

function type(el: HTMLTextAreaElement, value: string) {
  const set = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
  act(() => {
    set?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

describe("Composer drafts", () => {
  it("gives back the text written before leaving the chat", () => {
    const onDraft = vi.fn();
    render({ draft: { text: "Kargonuz yarın çıkıyor", mode: "message" }, onDraft });
    expect(box().value).toBe("Kargonuz yarın çıkıyor");
    expect(onDraft).toHaveBeenLastCalledWith("Kargonuz yarın çıkıyor", "message");
  });

  it("reports every change so the draft follows the box", () => {
    const onDraft = vi.fn();
    render({ onDraft });
    type(box(), "Merhaba");
    expect(onDraft).toHaveBeenLastCalledWith("Merhaba", "message");
  });

  it("never offers a note as a message to the customer", () => {
    render({ canNote: false, draft: { text: "müşteri kızgın, dikkat", mode: "note" } });
    expect(box()?.value ?? "").toBe("");
  });

  it("clears the box and the draft when the message goes", async () => {
    const onDraft = vi.fn();
    const onSend = vi.fn(async () => undefined);
    render({ draft: { text: "Tamamdır", mode: "message" }, onDraft, onSend });
    await act(async () => {
      box().dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(onSend).toHaveBeenCalledWith(expect.objectContaining({ text: "Tamamdır", mode: "message" }));
    expect(box().value).toBe("");
    expect(onDraft).toHaveBeenLastCalledWith("", "message");
  });
});
