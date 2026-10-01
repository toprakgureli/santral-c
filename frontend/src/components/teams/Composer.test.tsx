// The message box keeps an unsent draft per room: switching rooms and
// coming back finds the text and the tags again, editing a line does not
// touch the draft, and sending clears it.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TeamsGroupDetail, TeamsMessage } from "@/api/types";
import type { UploadsApi } from "@/teams/useUploads";
import { forgetDrafts, draftKey, readDraft } from "@/teams/drafts";

vi.mock("@/api/client", () => ({
  api: { teamsTyping: vi.fn(() => Promise.resolve()) },
  ApiError: class ApiError extends Error {},
}));

import Composer, { type Outgoing } from "./Composer";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const SELF = 7;

function room(id: number): TeamsGroupDetail {
  return {
    id,
    kind: "group",
    name: `Oda ${id}`,
    description: "",
    hasAvatar: false,
    postPolicy: "everyone",
    myRole: "member",
    canPost: true,
    canManage: false,
    canAdd: false,
    canInvite: false,
    canDelete: false,
    muted: false,
    mute: "none",
    unread: 0,
    memberCount: 2,
    updatedAt: "2026-10-01T10:00:00+03:00",
    members: [
      { id: SELF, name: "Ben", hasAvatar: false, role: "member", canPost: true, joinedAt: "", deliveredId: 0, readId: 0, historyFrom: 0 },
      { id: 9, name: "Ayşe Yılmaz", hasAvatar: false, role: "member", canPost: true, joinedAt: "", deliveredId: 0, readId: 0, historyFrom: 0 },
    ],
    invited: [],
    editable: false,
  };
}

const uploads: UploadsApi = {
  items: [],
  addFiles: () => undefined,
  remove: () => undefined,
  retry: () => undefined,
  clear: () => undefined,
  busy: false,
  readyIds: () => [],
  error: null,
  clearError: () => undefined,
  trimming: null,
  onTrimCancel: () => undefined,
  onTrimReady: () => undefined,
  editing: null,
  setEditing: () => undefined,
  replace: () => undefined,
};

let host: HTMLDivElement;
let root: Root;
let sent: Outgoing[];

beforeEach(() => {
  forgetDrafts();
  sent = [];
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

// show renders the box for a room, the way the room pane does: one
// instance per room.
function show(groupId: number, editing: TeamsMessage | null = null) {
  act(() => {
    root.render(
      <Composer
        key={groupId}
        group={room(groupId)}
        selfId={SELF}
        reply={null}
        editing={editing}
        onCancelReply={() => undefined}
        onCancelEdit={() => undefined}
        onSend={async (m) => {
          sent.push(m);
        }}
        onEdit={async () => undefined}
        onEditLast={() => undefined}
        uploads={uploads}
      />,
    );
  });
}

function box(): HTMLTextAreaElement {
  const el = host.querySelector("textarea");
  if (!el) throw new Error("no message box");
  return el;
}

function type(value: string) {
  const el = box();
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
  act(() => {
    setter.call(el, value);
    el.setSelectionRange(value.length, value.length);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function press(key: string) {
  await act(async () => {
    box().dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
  });
}

describe("room drafts", () => {
  it("keeps the text per room and brings it back", () => {
    show(1);
    type("yarım kalan cümle");
    show(2);
    expect(box().value).toBe("");
    type("ikinci odaya not");
    show(1);
    expect(box().value).toBe("yarım kalan cümle");
    show(2);
    expect(box().value).toBe("ikinci odaya not");
  });

  it("keeps who the draft tags", async () => {
    show(1);
    type("@Ay");
    // The @ list is open; Enter picks the first match.
    await press("Enter");
    expect(box().value).toBe("@Ayşe Yılmaz ");
    show(2);
    show(1);
    type("@Ayşe Yılmaz merhaba");
    await press("Enter");
    expect(sent).toHaveLength(1);
    expect(sent[0].mentionIds).toEqual([9]);
  });

  it("clears the draft once it is sent", async () => {
    show(1);
    type("gönderilecek");
    await press("Enter");
    expect(sent.map((m) => m.body)).toEqual(["gönderilecek"]);
    expect(box().value).toBe("");
    expect(readDraft(draftKey(SELF, 1)).text).toBe("");
    show(2);
    show(1);
    expect(box().value).toBe("");
  });

  it("editing a line leaves the draft alone", () => {
    show(1);
    type("taslak");
    const line = { id: 5, groupId: 1, kind: "text", body: "eski hali", deleted: false, mine: true, canDelete: true, reactions: [], mentions: [], mentionsAll: false, attachments: [], createdAt: "" } as TeamsMessage;
    act(() => {
      root.render(
        <Composer key={1} group={room(1)} selfId={SELF} reply={null} editing={line} onCancelReply={() => undefined} onCancelEdit={() => undefined} onSend={async () => undefined} onEdit={async () => undefined} onEditLast={() => undefined} uploads={uploads} />,
      );
    });
    expect(box().value).toBe("eski hali");
    expect(readDraft(draftKey(SELF, 1)).text).toBe("taslak");
    show(1, null);
    expect(box().value).toBe("taslak");
  });
});
