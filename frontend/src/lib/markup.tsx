// Light markup for chat lines. Nothing here is HTML: the text
// is tokenised and rendered as React nodes, so it is safe by construction.
//
//   **kalın**   *italik* or _italik_   __altı çizili__   ~~üstü çizili~~
//   `kod`   ```kod bloğu```   ||spoiler||   > alıntı   - liste   https://link
//
// Mention labels ("@Ad Soyad", "@herkes") are passed in and highlighted.

import { useState, type ReactNode } from "react";

export interface MarkupOptions {
  mentions?: string[];
}

export const MARKUP_HINT = /(\*\*|__|~~|\|\||`|^>\s|^[-*]\s)/m;

function escapeRe(v: string) {
  return v.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function Spoiler({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <span
      role="button"
      tabIndex={0}
      data-tip={open ? undefined : "Göstermek için tıkla"}
      onClick={() => setOpen(true)}
      onKeyDown={(e) => e.key === "Enter" && setOpen(true)}
      className={open ? "rounded bg-muted/60 px-0.5" : "cursor-pointer rounded bg-foreground/80 px-0.5 text-transparent select-none [&_*]:text-transparent"}
    >
      {children}
    </span>
  );
}

// Inline pass: earliest match wins; at the same position the alternatives
// are tried in the order listed, so ** beats * and __ beats _.
function inline(text: string, mentions: string[], key = 0): ReactNode[] {
  const parts: string[] = [
    "`([^`\\n]+)`",
    "\\*\\*(.+?)\\*\\*",
    "__(.+?)__",
    "~~(.+?)~~",
    "\\|\\|(.+?)\\|\\|",
    "\\*([^*\\n]+?)\\*",
    "(?<![\\p{L}\\p{N}])_([^_\\n]+?)_(?![\\p{L}\\p{N}])",
    "(https?:\\/\\/[^\\s<>()]+[^\\s<>().,!?;:'\"])",
  ];
  if (mentions.length) parts.push(`(${mentions.map(escapeRe).join("|")})`);
  const re = new RegExp(parts.join("|"), "gu");
  const out: ReactNode[] = [];
  let last = 0;
  let k = key;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const [code, bold, underline, strike, spoiler, italic, italic2, url, mention] = m.slice(1);
    if (code !== undefined) out.push(<code key={k++} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.8em]">{code}</code>);
    else if (bold !== undefined) out.push(<strong key={k++} className="font-semibold">{inline(bold, mentions, k * 100)}</strong>);
    else if (underline !== undefined) out.push(<u key={k++} className="underline underline-offset-2">{inline(underline, mentions, k * 100)}</u>);
    else if (strike !== undefined) out.push(<s key={k++} className="line-through opacity-80">{inline(strike, mentions, k * 100)}</s>);
    else if (spoiler !== undefined) out.push(<Spoiler key={k++}>{inline(spoiler, mentions, k * 100)}</Spoiler>);
    else if (italic !== undefined || italic2 !== undefined) out.push(<em key={k++}>{inline(italic ?? italic2 ?? "", mentions, k * 100)}</em>);
    else if (url !== undefined) out.push(<a key={k++} href={url} target="_blank" rel="noopener noreferrer" className="text-primary underline underline-offset-2 break-all hover:opacity-80">{url}</a>);
    else if (mention !== undefined) out.push(<span key={k++} className="rounded bg-violet-500/20 px-1 font-medium text-violet-500">{mention}</span>);
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

// renderMarkup turns a line's text into nodes: code fences, quotes and
// lists first, then the inline styles inside each piece.
export function renderMarkup(text: string, opts: MarkupOptions = {}): ReactNode {
  const mentions = opts.mentions ?? [];
  const lines = text.split("\n");
  const blocks: ReactNode[] = [];
  let i = 0;
  let k = 0;
  let para: string[] = [];
  const flush = () => {
    if (!para.length) return;
    const nodes: ReactNode[] = [];
    para.forEach((l, idx) => {
      if (idx) nodes.push("\n");
      nodes.push(...inline(l, mentions, k * 1000 + idx * 10));
    });
    blocks.push(<span key={k++}>{nodes}</span>);
    para = [];
  };
  while (i < lines.length) {
    const line = lines[i];
    if (line.startsWith("```")) {
      flush();
      const lang = line.slice(3).trim();
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i].startsWith("```")) body.push(lines[i++]);
      i++;
      // A one-line fence such as ```kod``` keeps its text.
      const single = body.length === 0 && line.length > 3 && line.endsWith("```") ? line.slice(3, -3) : null;
      blocks.push(
        <pre key={k++} className="my-1 overflow-x-auto rounded-lg border border-border/60 bg-muted/60 px-3 py-2 font-mono text-[0.8em] leading-relaxed whitespace-pre-wrap">
          {single ?? body.join("\n")}
          {lang && !single && <span className="float-right ml-3 text-[0.7em] text-muted-foreground">{lang}</span>}
        </pre>,
      );
      continue;
    }
    if (/^>\s?/.test(line)) {
      flush();
      const q: string[] = [];
      while (i < lines.length && /^>\s?/.test(lines[i])) q.push(lines[i++].replace(/^>\s?/, ""));
      blocks.push(
        <blockquote key={k++} className="my-1 border-l-2 border-muted-foreground/40 pl-2.5 text-muted-foreground">
          {q.map((l, idx) => (
            <span key={idx}>
              {idx > 0 && "\n"}
              {inline(l, mentions, k * 1000 + idx * 10)}
            </span>
          ))}
        </blockquote>,
      );
      continue;
    }
    if (/^[-*]\s+/.test(line)) {
      flush();
      const items: string[] = [];
      while (i < lines.length && /^[-*]\s+/.test(lines[i])) items.push(lines[i++].replace(/^[-*]\s+/, ""));
      blocks.push(
        <ul key={k++} className="my-1 list-disc space-y-0.5 pl-5">
          {items.map((l, idx) => <li key={idx}>{inline(l, mentions, k * 1000 + idx * 10)}</li>)}
        </ul>,
      );
      continue;
    }
    para.push(line);
    i++;
  }
  flush();
  return <>{blocks}</>;
}

// stripMarkup flattens a line to plain text for quotes, previews and
// notifications: the markers go, the words stay.
export function stripMarkup(text: string): string {
  return text
    .replace(/```[^\n]*\n?([\s\S]*?)```/g, "$1")
    .replace(/`([^`\n]+)`/g, "$1")
    .replace(/\*\*(.+?)\*\*/g, "$1")
    .replace(/__(.+?)__/g, "$1")
    .replace(/~~(.+?)~~/g, "$1")
    .replace(/\|\|(.+?)\|\|/g, "$1")
    .replace(/(^|[^\p{L}\p{N}*])\*([^*\n]+?)\*(?![\p{L}\p{N}*])/gu, "$1$2")
    .replace(/(?<![\p{L}\p{N}])_([^_\n]+?)_(?![\p{L}\p{N}])/gu, "$1")
    .replace(/^>\s?/gm, "")
    .replace(/^[-*]\s+/gm, "• ")
    .replace(/\n{2,}/g, "\n")
    .trim();
}

// Formatting the composer applies to a selection.
export type Style = "bold" | "italic" | "underline" | "strike" | "code" | "block" | "spoiler" | "quote" | "list";

export const STYLES: { key: Style; label: string; sample: string; shortcut?: string }[] = [
  { key: "bold", label: "Kalın", sample: "**metin**", shortcut: "Ctrl+B" },
  { key: "italic", label: "İtalik", sample: "*metin*", shortcut: "Ctrl+I" },
  { key: "underline", label: "Altı çizili", sample: "__metin__", shortcut: "Ctrl+U" },
  { key: "strike", label: "Üstü çizili", sample: "~~metin~~", shortcut: "Ctrl+Shift+X" },
  { key: "code", label: "Kod", sample: "`metin`", shortcut: "Ctrl+E" },
  { key: "block", label: "Kod bloğu", sample: "```\nmetin\n```", shortcut: "Ctrl+Shift+E" },
  { key: "spoiler", label: "Spoiler", sample: "||metin||" },
  { key: "quote", label: "Alıntı", sample: "> metin" },
  { key: "list", label: "Liste", sample: "- madde" },
];

// applyStyle wraps the selection of `text` and returns the new text plus
// where the caret and selection should land.
export function applyStyle(text: string, start: number, end: number, style: Style): { text: string; start: number; end: number } {
  const sel = text.slice(start, end) || "metin";
  const wrap = (l: string, r = l) => {
    const next = text.slice(0, start) + l + sel + r + text.slice(end);
    return { text: next, start: start + l.length, end: start + l.length + sel.length };
  };
  switch (style) {
    case "bold":
      return wrap("**");
    case "italic":
      return wrap("*");
    case "underline":
      return wrap("__");
    case "strike":
      return wrap("~~");
    case "code":
      return wrap("`");
    case "spoiler":
      return wrap("||");
    case "block": {
      const lead = start > 0 && text[start - 1] !== "\n" ? "\n" : "";
      const tail = end < text.length && text[end] !== "\n" ? "\n" : "";
      const l = `${lead}\`\`\`\n`;
      const r = `\n\`\`\`${tail}`;
      const next = text.slice(0, start) + l + sel + r + text.slice(end);
      return { text: next, start: start + l.length, end: start + l.length + sel.length };
    }
    case "quote":
    case "list": {
      const prefix = style === "quote" ? "> " : "- ";
      const lineStart = text.lastIndexOf("\n", start - 1) + 1;
      const chunk = text.slice(lineStart, end || start);
      const lines = (chunk || "metin").split("\n").map((l) => (l.startsWith(prefix) ? l : prefix + l));
      const replaced = lines.join("\n");
      const next = text.slice(0, lineStart) + replaced + text.slice(end || start);
      return { text: next, start: lineStart, end: lineStart + replaced.length };
    }
  }
}
