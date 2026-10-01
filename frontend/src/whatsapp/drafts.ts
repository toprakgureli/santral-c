// Unsent drafts, one per conversation: what the agent was writing, whether
// it was a message or an internal note, and the message it answers. They
// live in memory only, for this tab, so switching to another chat and back
// gives the text back, while customer text never lands in browser storage.
// Drafts are kept per signed-in person; the next person on the same tab
// never sees them.

import { userKey } from "@/lib/userStorage";
import type { WAMessage } from "@/whatsapp/types";

export interface WADraft {
  text: string;
  mode: "message" | "note";
  replyTo: WAMessage | null;
}

const drafts = new Map<string, WADraft>();

const keyOf = (conversationId: number) => userKey(`wa.draft.${conversationId}`);

// getDraft returns the conversation's draft, or undefined.
export function getDraft(conversationId: number): WADraft | undefined {
  return drafts.get(keyOf(conversationId));
}

// saveDraft keeps a conversation's draft; an empty one is dropped.
export function saveDraft(conversationId: number, d: WADraft) {
  if (!d.text.trim() && !d.replyTo) {
    drafts.delete(keyOf(conversationId));
    return;
  }
  drafts.set(keyOf(conversationId), d);
}

// clearDraft drops a conversation's draft, once it is sent.
export function clearDraft(conversationId: number) {
  drafts.delete(keyOf(conversationId));
}

// clearDrafts drops every draft.
export function clearDrafts() {
  drafts.clear();
}
