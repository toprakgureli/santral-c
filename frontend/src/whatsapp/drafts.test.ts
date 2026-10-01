// Drafts per conversation: kept while the agent moves between chats, dropped
// once empty, and never shown to the next person signed in on the tab.
import { afterEach, describe, expect, it } from "vitest";
import { setStorageUser } from "@/lib/userStorage";
import { clearDraft, clearDrafts, getDraft, saveDraft } from "@/whatsapp/drafts";
import type { WAMessage } from "@/whatsapp/types";

const quoted = { id: 7, conversationId: 1, direction: "in", kind: "text", body: "Siparişim nerede?", status: "received", createdAt: "2026-10-01T09:00:00Z", sender: { kind: "customer" } } as WAMessage;

afterEach(() => {
  clearDrafts();
  setStorageUser(0);
});

describe("drafts", () => {
  it("keeps one draft per conversation", () => {
    setStorageUser(5);
    saveDraft(1, { text: "Merhaba, kontrol ediyorum", mode: "message", replyTo: quoted });
    saveDraft(2, { text: "iç not", mode: "note", replyTo: null });
    expect(getDraft(1)).toEqual({ text: "Merhaba, kontrol ediyorum", mode: "message", replyTo: quoted });
    expect(getDraft(2)?.mode).toBe("note");
    expect(getDraft(3)).toBeUndefined();
  });

  it("drops a draft once it is empty, as after a send", () => {
    setStorageUser(5);
    saveDraft(1, { text: "yazıyorum", mode: "message", replyTo: null });
    saveDraft(1, { text: "  ", mode: "message", replyTo: null });
    expect(getDraft(1)).toBeUndefined();
    // an answer picked with nothing typed yet still counts
    saveDraft(1, { text: "", mode: "message", replyTo: quoted });
    expect(getDraft(1)?.replyTo?.id).toBe(7);
    clearDraft(1);
    expect(getDraft(1)).toBeUndefined();
  });

  it("belongs to the person who wrote it", () => {
    setStorageUser(5);
    saveDraft(1, { text: "müşteriye cevap", mode: "message", replyTo: null });
    setStorageUser(6);
    expect(getDraft(1)).toBeUndefined();
    setStorageUser(5);
    expect(getDraft(1)?.text).toBe("müşteriye cevap");
  });
});
