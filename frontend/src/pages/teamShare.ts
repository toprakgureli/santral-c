// teamShare draws the team's figures as one picture to share: a ranking
// by real calls with bars, each person's talk time, occupancy and how many
// long conversations they had, then a short written summary per person.
// The picture has its own dark palette so it reads the same wherever it
// is pasted.

import type { TeamRow } from "@/api/types";
import { APP_NAME } from "@/lib/brand";
import { avatarUrl } from "@/lib/avatar";

const W = 1200;
const PAD = 56;
const ROW = 64;

const C = {
  bg0: "#0b1020",
  bg1: "#161a3a",
  card: "rgba(255,255,255,0.05)",
  line: "rgba(255,255,255,0.10)",
  text: "#f4f5fb",
  muted: "#9aa1c2",
  dim: "#6b7194",
  accent: "#8b7cf6",
  green: "#34d399",
  amber: "#fbbf24",
  red: "#f87171",
  blue: "#60a5fa",
};

function short(seconds: number) {
  const m = Math.round(seconds / 60);
  if (m < 60) return `${m} dk`;
  return `${Math.floor(m / 60)} sa ${String(m % 60).padStart(2, "0")} dk`;
}

function clock(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function pct(part: number, whole: number) {
  return whole > 0 ? `%${Math.round((part / whole) * 100)}` : "—";
}

function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toLocaleUpperCase("tr") ?? "")
    .join("");
}

function loadImage(url: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => resolve(null);
    img.src = url;
  });
}

// wrap breaks a sentence into lines that fit the width.
function wrap(ctx: CanvasRenderingContext2D, text: string, width: number): string[] {
  const words = text.split(" ");
  const lines: string[] = [];
  let line = "";
  for (const w of words) {
    const probe = line ? `${line} ${w}` : w;
    if (ctx.measureText(probe).width > width && line) {
      lines.push(line);
      line = w;
    } else {
      line = probe;
    }
  }
  if (line) lines.push(line);
  return lines;
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

// summaryOf is one person's figures as a sentence.
export function summaryOf(r: TeamRow): string {
  const c = r.calls;
  const first = r.name.split(" ")[0];
  const parts: string[] = [];
  parts.push(`${first} ${c.long} gerçek çağrı almış (${c.inboundReal} gelen, ${c.outboundReal} giden), toplam ${short(c.talkSeconds)} görüşmüş`);
  if (r.shift.seconds > 0) parts.push(`yoğunluğu ${pct(c.talkSeconds, r.shift.seconds)} (${short(r.shift.seconds)} mesainin görüşmede geçen payı)`);
  if (c.peers > 0) {
    const bands: string[] = [];
    if (c.over5 > 0) bands.push(`${c.over5} görüşme 5 dk üstü`);
    if (c.over10 > 0) bands.push(`${c.over10} görüşme 10 dk üstü`);
    if (c.over20 > 0) bands.push(`${c.over20} görüşme 20 dk üstü`);
    parts.push(`${c.peers} farklı kişiyle konuşmuş${bands.length ? `: ${bands.join(", ")}` : ""}`);
  }
  if (c.avgTalkSeconds > 0) parts.push(`ortalama görüşme ${clock(c.avgTalkSeconds)}, en uzunu ${clock(c.longestSeconds)}`);
  if (c.avgAnswerSeconds > 0) parts.push(`gelen çağrıyı ortalama ${c.avgAnswerSeconds} saniyede açmış`);
  const unreached = c.unanswered + c.short;
  if (unreached > 0) parts.push(`${unreached} çağrıya ulaşılamamış (${c.unanswered} cevapsız, ${c.short} geçersiz)`);
  if (r.escalations > 0) parts.push(`${r.escalations} eskalasyon kaydetmiş`);
  return parts.join("; ") + ".";
}

export async function renderTeamImage(rows: TeamRow[], rangeLabel: string, scopeLabel: string): Promise<HTMLCanvasElement> {
  const ranked = [...rows]
    .filter((r) => r.calls.long + r.calls.unanswered + r.calls.short > 0 || r.shift.seconds > 0)
    .sort((a, b) => b.calls.long - a.calls.long || b.calls.talkSeconds - a.calls.talkSeconds || a.name.localeCompare(b.name, "tr"))
    .slice(0, 14);
  const totals = ranked.reduce(
    (t, r) => ({ real: t.real + r.calls.long, talk: t.talk + r.calls.talkSeconds, shift: t.shift + r.shift.seconds, unreached: t.unreached + r.calls.unanswered + r.calls.short, escalations: t.escalations + r.escalations }),
    { real: 0, talk: 0, shift: 0, unreached: 0, escalations: 0 },
  );
  const maxReal = Math.max(1, ...ranked.map((r) => r.calls.long));

  // Measure the summary block first so the canvas is exactly as tall as needed.
  const probe = document.createElement("canvas").getContext("2d")!;
  probe.font = "500 17px system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";
  const summaries = ranked.map((r) => wrap(probe, summaryOf(r), W - PAD * 2 - 24));
  const summaryLines = summaries.reduce((n, l) => n + l.length, 0);

  const headH = 190;
  const listY = headH + 64;
  const listH = ranked.length * ROW + 16;
  const sumY = listY + listH + 48;
  const sumH = 44 + summaryLines * 26 + ranked.length * 14 + 16;
  const H = sumY + sumH + 72;

  const scale = 2;
  const canvas = document.createElement("canvas");
  canvas.width = W * scale;
  canvas.height = H * scale;
  const ctx = canvas.getContext("2d")!;
  ctx.scale(scale, scale);

  const font = (weight: number, size: number) => `${weight} ${size}px system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif`;

  // Floor
  const g = ctx.createLinearGradient(0, 0, W, H);
  g.addColorStop(0, C.bg0);
  g.addColorStop(1, C.bg1);
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, W, H);
  const glow = ctx.createRadialGradient(W - 140, 60, 0, W - 140, 60, 420);
  glow.addColorStop(0, "rgba(139,124,246,0.28)");
  glow.addColorStop(1, "rgba(139,124,246,0)");
  ctx.fillStyle = glow;
  ctx.fillRect(0, 0, W, H);

  // Header
  ctx.fillStyle = C.text;
  ctx.font = font(700, 34);
  ctx.fillText("Ekip Performansı", PAD, PAD + 30);
  ctx.fillStyle = C.muted;
  ctx.font = font(500, 17);
  ctx.fillText(`${rangeLabel} · ${scopeLabel} · ${APP_NAME}`, PAD, PAD + 60);

  const chips = [
    { label: "Gerçek çağrı", value: String(totals.real), color: C.green },
    { label: "Görüşme süresi", value: short(totals.talk), color: C.blue },
    { label: "Ulaşma", value: pct(totals.real, totals.real + totals.unreached), color: C.accent },
    { label: "Yoğunluk", value: pct(totals.talk, totals.shift), color: C.amber },
    { label: "Ulaşılamayan", value: String(totals.unreached), color: C.red },
    { label: "Eskalasyon", value: String(totals.escalations), color: C.muted },
  ];
  const chipW = (W - PAD * 2 - 12 * (chips.length - 1)) / chips.length;
  chips.forEach((c, i) => {
    const x = PAD + i * (chipW + 12);
    const y = PAD + 84;
    ctx.fillStyle = C.card;
    roundRect(ctx, x, y, chipW, 66, 14);
    ctx.fill();
    ctx.fillStyle = c.color;
    roundRect(ctx, x, y + 14, 4, 38, 2);
    ctx.fill();
    ctx.fillStyle = C.text;
    ctx.font = font(700, 24);
    ctx.fillText(c.value, x + 18, y + 32);
    ctx.fillStyle = C.muted;
    ctx.font = font(500, 13);
    ctx.fillText(c.label, x + 18, y + 54);
  });

  // Ranking
  ctx.fillStyle = C.muted;
  ctx.font = font(600, 13);
  ctx.fillText("SIRALAMA · GERÇEK ÇAĞRIYA GÖRE", PAD, listY - 18);
  const colName = PAD + 96;
  const colBar = PAD + 360;
  const barW = 250;
  const colTalk = colBar + barW + 70;
  const colOcc = colTalk + 110;
  const colLong = colOcc + 100;
  const colAns = colLong + 130;
  ctx.fillStyle = C.dim;
  ctx.font = font(600, 12);
  ctx.fillText("GÖRÜŞME", colTalk, listY - 18);
  ctx.fillText("YOĞUNLUK", colOcc, listY - 18);
  ctx.fillText("10+ / 20+ DK", colLong, listY - 18);
  ctx.fillText("ORT. CEVAP", colAns, listY - 18);

  const avatars = await Promise.all(ranked.map((r) => loadImage(avatarUrl(r.userId) ?? "")));

  ranked.forEach((r, i) => {
    const y = listY + i * ROW;
    if (i % 2 === 0) {
      ctx.fillStyle = "rgba(255,255,255,0.03)";
      roundRect(ctx, PAD - 12, y, W - PAD * 2 + 24, ROW, 12);
      ctx.fill();
    }
    // rank
    ctx.fillStyle = i < 3 ? [C.amber, "#cbd5e1", "#d97706"][i] : C.dim;
    ctx.font = font(700, 18);
    ctx.fillText(String(i + 1), PAD, y + 39);
    // photo or initials
    const ax = PAD + 40;
    const ay = y + ROW / 2;
    ctx.save();
    ctx.beginPath();
    ctx.arc(ax, ay, 20, 0, Math.PI * 2);
    ctx.closePath();
    ctx.clip();
    const img = avatars[i];
    if (img) {
      ctx.drawImage(img, ax - 20, ay - 20, 40, 40);
    } else {
      ctx.fillStyle = "rgba(139,124,246,0.25)";
      ctx.fillRect(ax - 20, ay - 20, 40, 40);
      ctx.fillStyle = C.text;
      ctx.font = font(700, 15);
      ctx.textAlign = "center";
      ctx.fillText(initials(r.name), ax, ay + 5);
      ctx.textAlign = "left";
    }
    ctx.restore();
    // name and roles
    ctx.fillStyle = C.text;
    ctx.font = font(600, 17);
    ctx.fillText(r.name, colName, y + 30);
    ctx.fillStyle = C.dim;
    ctx.font = font(500, 12);
    ctx.fillText(`${r.calls.inboundReal} gelen · ${r.calls.outboundReal} giden · ${r.shift.seconds > 0 ? short(r.shift.seconds) + " mesai" : "mesai yok"}`, colName, y + 48);
    // bar
    ctx.fillStyle = "rgba(255,255,255,0.08)";
    roundRect(ctx, colBar, y + 26, barW, 12, 6);
    ctx.fill();
    const w = Math.max(6, (r.calls.long / maxReal) * barW);
    ctx.fillStyle = i === 0 ? C.green : C.accent;
    roundRect(ctx, colBar, y + 26, w, 12, 6);
    ctx.fill();
    ctx.fillStyle = C.text;
    ctx.font = font(700, 18);
    ctx.fillText(String(r.calls.long), colBar + barW + 14, y + 39);
    // figures
    ctx.font = font(600, 16);
    ctx.fillStyle = C.text;
    ctx.fillText(r.calls.talkSeconds > 0 ? short(r.calls.talkSeconds) : "—", colTalk, y + 38);
    ctx.fillStyle = r.shift.seconds > 0 && r.calls.talkSeconds / r.shift.seconds >= 0.5 ? C.amber : C.text;
    ctx.fillText(pct(r.calls.talkSeconds, r.shift.seconds), colOcc, y + 38);
    ctx.fillStyle = C.text;
    ctx.fillText(`${r.calls.over10} / ${r.calls.over20}`, colLong, y + 38);
    ctx.fillText(r.calls.avgAnswerSeconds > 0 ? `${r.calls.avgAnswerSeconds} sn` : "—", colAns, y + 38);
  });

  // Summary
  ctx.fillStyle = C.card;
  roundRect(ctx, PAD - 12, sumY, W - PAD * 2 + 24, sumH, 16);
  ctx.fill();
  ctx.fillStyle = C.muted;
  ctx.font = font(600, 13);
  ctx.fillText("ÖZET", PAD + 12, sumY + 30);
  let ly = sumY + 60;
  summaries.forEach((lines, i) => {
    ctx.fillStyle = i === 0 ? C.green : C.accent;
    roundRect(ctx, PAD + 12, ly - 14, 4, lines.length * 26 - 8, 2);
    ctx.fill();
    ctx.fillStyle = C.text;
    ctx.font = font(500, 17);
    for (const l of lines) {
      ctx.fillText(l, PAD + 28, ly);
      ly += 26;
    }
    ly += 14;
  });

  // Footer
  ctx.fillStyle = C.dim;
  ctx.font = font(500, 12);
  ctx.fillText("Gerçek çağrı: 30 saniye ve üstü görüşmeler · Yoğunluk: görüşme süresi / mesai süresi, yalnızca çağrılar · Ort. cevap: gelen çağrının açılmasına kadar geçen süre", PAD, H - 28);
  ctx.textAlign = "right";
  ctx.fillText(new Date().toLocaleString("tr-TR", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" }), W - PAD, H - 28);
  ctx.textAlign = "left";

  return canvas;
}
