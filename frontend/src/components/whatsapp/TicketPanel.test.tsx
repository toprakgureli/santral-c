// The contact card follows the open chat: an answer about an earlier
// customer that arrives late is dropped, and a change the server refuses
// goes back to what the server holds.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WAConversation, WAHistoryItem } from "@/whatsapp/types";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const api = vi.hoisted(() => ({
  history: vi.fn(),
  updateTicket: vi.fn(),
  updateContact: vi.fn(),
}));
vi.mock("@/whatsapp/api", () => ({ waApi: api }));
vi.mock("@/whatsapp/WhatsAppContext", () => ({ useWhatsApp: () => ({ muted: () => false, setConvPref: async () => undefined }) }));
vi.mock("@/auth/AuthContext", () => ({ useAuth: () => ({ user: null }) }));
vi.mock("@/softphone/SoftphoneContext", () => ({ useSoftphoneContext: () => ({ status: "idle", call: async () => undefined }) }));

const { default: TicketPanel } = await import("@/components/whatsapp/TicketPanel");
const { ApiError } = await import("@/api/client");

function conv(id: number, name: string): WAConversation {
  return {
    id, channelId: 1, channelName: "Destek", unread: 0, teamReadId: 0, version: 1,
    contact: { id: id * 10, waId: `90532000000${id}`, name, profileName: name, display: name, tags: [], note: "", optedOut: false, blocked: false },
    ticket: { id: id * 100, number: id, status: "open", priority: "normal", category: "", tags: [], waitingCount: 0, reopenCount: 0, createdAt: "2026-10-01T09:00:00Z", participants: [] },
  };
}

function past(conversationId: number, number: number): WAHistoryItem {
  return { conversationId, ticketId: number * 1000, number, channelName: "Destek", status: "resolved", owner: "Ayşe", createdAt: "2026-09-01T09:00:00Z", messages: 3 };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let host: HTMLDivElement;
let root: Root;

beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  vi.clearAllMocks();
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

const show = (c: WAConversation) => act(() => root.render(<TicketPanel conv={c} canEditContact canEditTicket onOpen={() => undefined} onClose={() => undefined} />));

const openPast = () => {
  const fold = [...host.querySelectorAll("button")].find((b) => b.textContent?.includes("Önceki sohbetler"));
  act(() => fold?.click());
};

describe("TicketPanel", () => {
  it("drops the past chats of a customer no longer shown", async () => {
    const first = deferred<WAHistoryItem[]>();
    const second = deferred<WAHistoryItem[]>();
    api.history.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    show(conv(1, "Ali"));
    show(conv(2, "Veli"));
    await act(async () => second.resolve([past(2, 22)]));
    await act(async () => first.resolve([past(1, 11), past(1, 12)]));
    openPast();
    expect(host.textContent).toContain("#22");
    expect(host.textContent).not.toContain("#11");
    expect(host.textContent).not.toContain("#12");
  });

  it("puts the status back when the server refuses the change", async () => {
    api.history.mockResolvedValue([]);
    api.updateTicket.mockRejectedValue(new ApiError(409, "CONFLICT", "Sohbet bu arada kapatıldı. Değiştirmek için önce yeniden aç."));
    show(conv(3, "Ayşe"));
    const pending = [...host.querySelectorAll("button")].find((b) => b.textContent?.includes("Müşteri bekleniyor"));
    expect(pending).toBeTruthy();
    await act(async () => pending?.click());
    expect(host.textContent).toContain("Sohbet bu arada kapatıldı");
    // the "open" button is the chosen one again
    const open = [...host.querySelectorAll("button")].find((b) => b.textContent?.trim() === "Açık");
    expect(open?.className).toContain("emerald");
    expect(pending?.className).not.toContain("sky-500/15");
  });
});
