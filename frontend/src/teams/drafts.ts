// Unsent drafts, one per room, kept in memory for as long as the panel is
// open: the text, who it tags and the line it answers. Switching rooms and
// coming back finds the draft where it was left; sending clears it. Nothing
// is written to the browser, so a draft never outlives the session.

import type { TeamsMessage } from "@/api/types";

export interface Draft {
  text: string;
  // Names picked from the @ list, by person id.
  tagged: Record<number, string>;
  reply: TeamsMessage | null;
}

const EMPTY: Draft = { text: "", tagged: {}, reply: null };

const drafts = new Map<string, Draft>();

// The key carries the person too, so another sign-in on the same tab never
// sees someone else's draft.
export function draftKey(selfId: number, groupId: number): string {
  return `${selfId}:${groupId}`;
}

export function readDraft(key: string): Draft {
  return drafts.get(key) ?? EMPTY;
}

export function saveDraft(key: string, patch: Partial<Draft>): void {
  const next = { ...readDraft(key), ...patch };
  if (!next.text && !next.reply && Object.keys(next.tagged).length === 0) {
    drafts.delete(key);
    return;
  }
  drafts.set(key, next);
}

export function clearDraft(key: string): void {
  drafts.delete(key);
}

// forgetDrafts drops every draft, for sign-out.
export function forgetDrafts(): void {
  drafts.clear();
}
