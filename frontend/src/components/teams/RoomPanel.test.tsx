// The side panel drops answers that arrive for a room or a question the
// person has already left: a slow search or file list from the previous
// room never fills the new one.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TeamsGroupDetail, TeamsMediaItem, TeamsMessage } from "@/api/types";

type Pending<T> = { resolve: (v: T) => void };
const searches: { groupId: number; q: string; p: Pending<TeamsMessage[]> }[] = [];
const lists: { groupId: number; p: Pending<{ items: TeamsMediaItem[]; more: boolean }> }[] = [];

vi.mock("@/api/client", () => ({
  api: {
    teamsSearch: (groupId: number, q: string) => new Promise((resolve) => searches.push({ groupId, q, p: { resolve } })),
    teamsMedia: (groupId: number) => new Promise((resolve) => lists.push({ groupId, p: { resolve } })),
  },
  ApiError: class ApiError extends Error {},
}));

import RoomPanel from "./RoomPanel";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function room(id: number): TeamsGroupDetail {
  return { id, kind: "group", name: `Oda ${id}`, description: "", hasAvatar: false, postPolicy: "everyone", myRole: "member", canPost: true, canManage: false, canAdd: false, canInvite: false, canDelete: false, muted: false, mute: "none", unread: 0, memberCount: 1, updatedAt: "", members: [], invited: [], editable: false };
}

function hit(id: number, body: string): TeamsMessage {
  return { id, groupId: 0, kind: "text", body, sender: { id: 3, name: "Ayşe", hasAvatar: false }, deleted: false, mine: false, canDelete: false, reactions: [], mentions: [], mentionsAll: false, attachments: [], createdAt: "2026-10-01T10:00:00+03:00" };
}

function file(id: number, name: string): TeamsMediaItem {
  return { id, kind: "file", name, mime: "application/pdf", size: 10, hasThumb: false, ready: true, messageId: id, sender: "Ayşe", createdAt: "2026-10-01T10:00:00+03:00" };
}

let host: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.useFakeTimers();
  searches.length = 0;
  lists.length = 0;
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.useRealTimers();
});

function panel(mode: "search" | "media", groupId: number) {
  act(() => {
    root.render(<RoomPanel mode={mode} group={room(groupId)} selfId={1} onChanged={() => undefined} onAdd={() => undefined} onInvite={() => undefined} onJump={() => undefined} onClose={() => undefined} />);
  });
}

function typeSearch(value: string) {
  const el = host.querySelector("input")!;
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  act(() => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  act(() => {
    vi.advanceTimersByTime(300);
  });
}

describe("RoomPanel", () => {
  it("drops a search answer for the room left behind", async () => {
    panel("search", 1);
    typeSearch("fatura");
    expect(searches.map((s) => s.groupId)).toEqual([1]);
    panel("search", 2);
    typeSearch("fatura");
    expect(searches.map((s) => s.groupId)).toEqual([1, 2]);
    await act(async () => searches[1].p.resolve([hit(20, "ikinci odanın faturası")]));
    await act(async () => searches[0].p.resolve([hit(10, "birinci odanın faturası")]));
    expect(host.textContent).toContain("ikinci odanın");
    expect(host.textContent).not.toContain("birinci odanın");
  });

  it("drops an older query's answer that arrives last", async () => {
    panel("search", 1);
    typeSearch("fat");
    typeSearch("fatura");
    await act(async () => searches[1].p.resolve([hit(2, "yeni arama")]));
    await act(async () => searches[0].p.resolve([hit(1, "eski arama")]));
    expect(host.textContent).toContain("yeni arama");
    expect(host.textContent).not.toContain("eski arama");
  });

  it("drops a file list for the room left behind", async () => {
    panel("media", 1);
    // Files tab.
    const tab = [...host.querySelectorAll("button")].find((b) => b.textContent === "Dosyalar")!;
    act(() => tab.click());
    panel("media", 2);
    const last = lists[lists.length - 1];
    expect(last.groupId).toBe(2);
    await act(async () => last.p.resolve({ items: [file(20, "ikinci.pdf")], more: false }));
    for (const l of lists.slice(0, -1)) await act(async () => l.p.resolve({ items: [file(10, "birinci.pdf")], more: false }));
    expect(host.textContent).toContain("ikinci.pdf");
    expect(host.textContent).not.toContain("birinci.pdf");
  });
});
