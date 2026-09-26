// waText renders WhatsApp's own text styling: *kalın*, _italik_, ~üstü
// çizili~, ```sabit genişlik``` and links. Nothing is treated as HTML.

import type { ReactNode } from "react";

const TOKEN = /(```[\s\S]+?```|\*[^*\n]+\*|_[^_\n]+_|~[^~\n]+~|https?:\/\/[^\s<]+)/g;

export function waText(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let key = 0;
  for (const m of text.matchAll(TOKEN)) {
    const i = m.index ?? 0;
    if (i > last) out.push(text.slice(last, i));
    const t = m[0];
    if (t.startsWith("```")) out.push(<code key={key++} className="rounded-md bg-foreground/10 px-1 py-0.5 font-mono text-[0.85em]">{t.slice(3, -3)}</code>);
    else if (t.startsWith("*")) out.push(<strong key={key++}>{t.slice(1, -1)}</strong>);
    else if (t.startsWith("_")) out.push(<em key={key++}>{t.slice(1, -1)}</em>);
    else if (t.startsWith("~")) out.push(<s key={key++}>{t.slice(1, -1)}</s>);
    else out.push(<a key={key++} href={t} target="_blank" rel="noopener noreferrer" className="break-all text-sky-600 underline underline-offset-2 dark:text-sky-400">{t}</a>);
    last = i + t.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}
