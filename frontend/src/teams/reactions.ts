// applyReaction folds a live reaction event into the lines on screen, so a
// room does not reload its last page every time someone taps an emoji.

import type { TeamsEvent, TeamsMessage, TeamsPerson, TeamsReaction } from "@/api/types";

export function applyReaction(items: TeamsMessage[], e: TeamsEvent, selfId: number, people: TeamsPerson[]): TeamsMessage[] {
  if (e.type !== "reaction" || !e.id || !e.userId || !e.emoji) return items;
  const idx = items.findIndex((m) => m.id === e.id);
  if (idx === -1) return items;
  const m = items[idx];
  const next = toggle(m.reactions, e.emoji, e.userId, e.added ?? false, e.name ?? "", selfId, people);
  if (next === m.reactions) return items;
  const out = items.slice();
  out[idx] = { ...m, reactions: next };
  return out;
}

function toggle(list: TeamsReaction[], emoji: string, userId: number, added: boolean, name: string, selfId: number, people: TeamsPerson[]): TeamsReaction[] {
  const at = list.findIndex((r) => r.emoji === emoji);
  const cur = at === -1 ? null : list[at];
  const has = !!cur?.people?.some((p) => p.id === userId);
  if (added) {
    // The same event twice (two tabs, a reconnect) changes nothing.
    if (has || (userId === selfId && cur?.mine)) return list;
    const known = people.find((p) => p.id === userId);
    const person: TeamsPerson = known ? { id: known.id, name: known.name, hasAvatar: known.hasAvatar, avatarVersion: known.avatarVersion } : { id: userId, name, hasAvatar: false };
    const r: TeamsReaction = cur
      ? { ...cur, count: cur.count + 1, mine: cur.mine || userId === selfId, names: [...cur.names, person.name], people: [...(cur.people ?? []), person] }
      : { emoji, count: 1, mine: userId === selfId, names: [person.name], people: [person] };
    return at === -1 ? [...list, r] : list.map((x, i) => (i === at ? r : x));
  }
  if (!cur || (!has && !(userId === selfId && cur.mine))) return list;
  // names and people run side by side; without a card, drop the name.
  const pos = (cur.people ?? []).findIndex((p) => p.id === userId);
  const drop = pos === -1 ? cur.names.indexOf(name) : pos;
  const rest = (cur.people ?? []).filter((_, i) => i !== pos);
  const names = cur.names.filter((_, i) => i !== drop);
  const count = cur.count - 1;
  if (count <= 0) return list.filter((_, i) => i !== at);
  return list.map((x, i) => (i === at ? { ...cur, count, mine: userId === selfId ? false : cur.mine, names, people: rest } : x));
}
