import { describe, expect, it } from "vitest";
import type { TeamsEvent, TeamsMessage, TeamsPerson } from "@/api/types";
import { statusOf } from "@/components/teams/Presence";
import { applyReaction } from "./reactions";

const SELF = 1;
const people: TeamsPerson[] = [
  { id: 1, name: "Ben", hasAvatar: false },
  { id: 2, name: "Ali Kaya", hasAvatar: true, avatarVersion: 3 },
];

function line(id: number): TeamsMessage {
  return { id, groupId: 4, kind: "text", body: `satır ${id}`, deleted: false, mine: false, canDelete: false, reactions: [], mentions: [], mentionsAll: false, attachments: [], createdAt: "" };
}

function ev(id: number, userId: number, emoji: string, added: boolean): TeamsEvent {
  return { type: "reaction", groupId: 4, id, userId, name: people.find((p) => p.id === userId)?.name ?? "Biri", emoji, added };
}

describe("applyReaction", () => {
  it("adds, counts and removes a reaction on the line on screen", () => {
    let items = [line(10), line(11)];
    items = applyReaction(items, ev(11, 2, "👍", true), SELF, people);
    expect(items[1].reactions).toEqual([{ emoji: "👍", count: 1, mine: false, names: ["Ali Kaya"], people: [{ id: 2, name: "Ali Kaya", hasAvatar: true, avatarVersion: 3 }] }]);
    items = applyReaction(items, ev(11, 1, "👍", true), SELF, people);
    expect(items[1].reactions[0]).toMatchObject({ count: 2, mine: true, names: ["Ali Kaya", "Ben"] });
    items = applyReaction(items, ev(11, 2, "👍", false), SELF, people);
    expect(items[1].reactions[0]).toMatchObject({ count: 1, mine: true, names: ["Ben"] });
    items = applyReaction(items, ev(11, 1, "👍", false), SELF, people);
    expect(items[1].reactions).toEqual([]);
    expect(items[0].reactions).toEqual([]);
  });

  it("ignores the same event twice and lines that are not loaded", () => {
    const once = applyReaction([line(10)], ev(10, 2, "🔥", true), SELF, people);
    expect(applyReaction(once, ev(10, 2, "🔥", true), SELF, people)).toBe(once);
    const items = [line(10)];
    expect(applyReaction(items, ev(99, 2, "🔥", true), SELF, people)).toBe(items);
  });

  it("keeps the other emojis of the line", () => {
    let items = applyReaction([line(10)], ev(10, 2, "🔥", true), SELF, people);
    items = applyReaction(items, ev(10, 2, "✅", true), SELF, people);
    items = applyReaction(items, ev(10, 2, "🔥", false), SELF, people);
    expect(items[0].reactions.map((r) => r.emoji)).toEqual(["✅"]);
  });
});

describe("statusOf", () => {
  it("does not wait on someone who may not read the line", () => {
    const seats = {
      1: { deliveredId: 50, readId: 50, name: "Ben" },
      2: { deliveredId: 50, readId: 50, name: "Ali" },
      3: { deliveredId: 49, readId: 49, name: "Yeni", historyFrom: 60 },
    };
    expect(statusOf(50, SELF, seats)).toEqual({ status: "read", readBy: ["Ali"] });
    // A line written after they joined waits for them like anyone else.
    expect(statusOf(70, SELF, { ...seats, 2: { deliveredId: 70, readId: 70, name: "Ali" } })).toEqual({ status: "sent", readBy: ["Ali"] });
  });
});
