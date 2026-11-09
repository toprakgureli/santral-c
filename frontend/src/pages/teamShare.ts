// teamShare draws the team's figures as one picture to share: a ranking
// by real calls with bars, each person's talk time, occupancy and how many
// long conversations they had, then a short written summary per person.
// The picture has its own dark palette so it reads the same wherever it
// is pasted.

import type { TeamRow } from "@/api/types";
import { APP_NAME } from "@/lib/brand";
import { avatarUrl } from "@/lib/avatar";
import { numericDateTime } from "../lib/time";

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
  const hrs = Math.floor(m / 60);
  const rem = m % 60;
  if (rem === 0) return `${hrs} sa`;
  return `${hrs} sa ${rem} dk`;
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
export function summaryOf(r: TeamRow, breakLimitMinutes = 60, dayCount = 1): string {
  const c = r.calls;
  const first = r.name.split(" ")[0];
  const parts: string[] = [];
  parts.push(`${first} ${c.long} gerçek çağrı almış (${c.inboundReal} gelen, ${c.outboundReal} giden), toplam ${short(c.talkSeconds)} görüşmüş`);
  if (r.shift.seconds > 0) parts.push(`yoğunluğu ${pct(c.talkSeconds, r.shift.seconds)} (${short(r.shift.seconds)} mesainin görüşmede geçen payı)`);

  // Break compliance and total break duration
  const limitSec = breakLimitMinutes * 60 * dayCount;
  const bSec = r.breakSeconds || 0;
  if (bSec > 0) {
    if (bSec > limitSec) {
      parts.push(`TOPLAM ${short(bSec)} MOLA KULLANMIŞ (${short(limitSec)} SINIRINI ${short(bSec - limitSec)} AŞMIŞ!)`);
    } else {
      parts.push(`toplam ${short(bSec)} mola kullanmış (${short(limitSec)} mola sınırı aşılmadı)`);
    }
  } else if (r.shift.seconds > 0) {
    parts.push("hiç mola kullanmamış");
  }

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

export async function renderTeamImage(
  rows: TeamRow[],
  rangeLabel: string,
  scopeLabel: string,
  realSeconds: number,
  breakLimitMinutes = 60,
  dayCount = 1,
): Promise<HTMLCanvasElement> {
  const ranked = [...rows]
    .filter((r) => r.calls.long + r.calls.unanswered + r.calls.short > 0 || r.shift.seconds > 0)
    .sort((a, b) => b.calls.long - a.calls.long || b.calls.talkSeconds - a.calls.talkSeconds || a.name.localeCompare(b.name, "tr"))
    .slice(0, 14);
  const totals = ranked.reduce(
    (t, r) => ({
      real: t.real + r.calls.long,
      talk: t.talk + r.calls.talkSeconds,
      shift: t.shift + r.shift.seconds,
      breakSec: t.breakSec + (r.breakSeconds || 0),
      unreached: t.unreached + r.calls.unanswered + r.calls.short,
      escalations: t.escalations + r.escalations,
    }),
    { real: 0, talk: 0, shift: 0, breakSec: 0, unreached: 0, escalations: 0 },
  );
  const maxReal = Math.max(1, ...ranked.map((r) => r.calls.long));
  const limitSec = breakLimitMinutes * 60 * dayCount;
  const exceededCount = ranked.filter((r) => (r.breakSeconds || 0) > limitSec).length;

  // Measure the summary block first so the canvas is exactly as tall as needed.
  const probe = document.createElement("canvas").getContext("2d")!;
  probe.font = "500 17px system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";
  const summaries = ranked.map((r) => wrap(probe, summaryOf(r, breakLimitMinutes, dayCount), W - PAD * 2 - 24));
  const summaryLines = summaries.reduce((n, l) => n + l.length, 0);

  const bannerY = PAD + 84 + 66 + 14;
  const bannerH = 42;
  const listY = bannerY + bannerH + 32;
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
    { label: "Toplam Mola", value: short(totals.breakSec), color: C.amber },
    {
      label: `Mola Sınırı (${short(limitSec)})`,
      value: exceededCount > 0 ? `${exceededCount} kişi aştı` : "Aşım yok",
      color: exceededCount > 0 ? C.red : C.green,
    },
    { label: "Ulaşma", value: pct(totals.real, totals.real + totals.unreached), color: C.accent },
    { label: "Yoğunluk", value: pct(totals.talk, totals.shift), color: C.amber },
    { label: "Ulaşılamayan", value: String(totals.unreached), color: C.red },
  ];
  const chipW = (W - PAD * 2 - 10 * (chips.length - 1)) / chips.length;
  chips.forEach((c, i) => {
    const x = PAD + i * (chipW + 10);
    const y = PAD + 84;
    ctx.fillStyle = C.card;
    roundRect(ctx, x, y, chipW, 66, 14);
    ctx.fill();
    ctx.fillStyle = c.color;
    roundRect(ctx, x, y + 14, 4, 38, 2);
    ctx.fill();
    ctx.fillStyle = C.text;
    ctx.font = font(700, 20);
    ctx.fillText(c.value, x + 12, y + 32);
    ctx.fillStyle = C.muted;
    ctx.font = font(500, 12);
    ctx.fillText(c.label, x + 12, y + 54);
  });

  // Break compliance banner
  if (exceededCount > 0) {
    ctx.fillStyle = "rgba(248, 113, 113, 0.16)";
    roundRect(ctx, PAD - 12, bannerY, W - PAD * 2 + 24, bannerH, 10);
    ctx.fill();
    ctx.strokeStyle = "rgba(248, 113, 113, 0.55)";
    ctx.lineWidth = 1.5;
    ctx.stroke();

    ctx.fillStyle = C.red;
    ctx.font = font(700, 14.5);
    const exceededNames = ranked
      .filter((r) => (r.breakSeconds || 0) > limitSec)
      .map((r) => `${r.name.split(" ")[0]} (+${short((r.breakSeconds || 0) - limitSec)})`)
      .join(", ");
    ctx.fillText(`MOLA AŞIMI UYARISI: ${exceededCount} temsilci mola sınırını (${short(limitSec)}) aştı: ${exceededNames}`, PAD + 10, bannerY + 26);
  } else {
    ctx.fillStyle = "rgba(52, 211, 153, 0.12)";
    roundRect(ctx, PAD - 12, bannerY, W - PAD * 2 + 24, bannerH, 10);
    ctx.fill();
    ctx.strokeStyle = "rgba(52, 211, 153, 0.4)";
    ctx.lineWidth = 1;
    ctx.stroke();

    ctx.fillStyle = C.green;
    ctx.font = font(600, 14);
    ctx.fillText(`MOLA UYUMU: Tüm ekip mola sınırına (${short(limitSec)}) eksiksiz uydu. Mola aşımı yapılmadı.`, PAD + 10, bannerY + 26);
  }

  // Ranking
  ctx.fillStyle = C.muted;
  ctx.font = font(600, 13);
  ctx.fillText("SIRALAMA · GERÇEK ÇAĞRIYA GÖRE", PAD, listY - 18);
  const colName = PAD + 76;
  const colBar = PAD + 310;
  const barW = 160;
  const colTalk = colBar + barW + 55;
  const colBreak = colTalk + 115;
  const colOcc = colBreak + 145;
  const colAns = colOcc + 95;
  const colLong = colAns + 85;

  ctx.fillStyle = C.dim;
  ctx.font = font(600, 12);
  ctx.fillText("GÖRÜŞME", colTalk, listY - 18);
  ctx.fillText("MOLA (AŞIM)", colBreak, listY - 18);
  ctx.fillText("YOĞUNLUK", colOcc, listY - 18);
  ctx.fillText("ORT. CEVAP", colAns, listY - 18);
  ctx.fillText("10+ / 20+", colLong, listY - 18);

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
    const ax = PAD + 38;
    const ay = y + ROW / 2;
    ctx.save();
    ctx.beginPath();
    ctx.arc(ax, ay, 19, 0, Math.PI * 2);
    ctx.closePath();
    ctx.clip();
    const img = avatars[i];
    if (img) {
      ctx.drawImage(img, ax - 19, ay - 19, 38, 38);
    } else {
      ctx.fillStyle = "rgba(139,124,246,0.25)";
      ctx.fillRect(ax - 19, ay - 19, 38, 38);
      ctx.fillStyle = C.text;
      ctx.font = font(700, 14);
      ctx.textAlign = "center";
      ctx.fillText(initials(r.name), ax, ay + 5);
      ctx.textAlign = "left";
    }
    ctx.restore();
    // name and roles
    ctx.fillStyle = C.text;
    ctx.font = font(600, 16);
    ctx.fillText(r.name, colName, y + 29);
    ctx.fillStyle = C.dim;
    ctx.font = font(500, 11.5);
    ctx.fillText(`${r.calls.inboundReal} gelen · ${r.calls.outboundReal} giden · ${r.shift.seconds > 0 ? short(r.shift.seconds) + " mesai" : "mesai yok"}`, colName, y + 47);
    // bar
    ctx.fillStyle = "rgba(255,255,255,0.08)";
    roundRect(ctx, colBar, y + 26, barW, 12, 6);
    ctx.fill();
    const w = Math.max(6, (r.calls.long / maxReal) * barW);
    ctx.fillStyle = i === 0 ? C.green : C.accent;
    roundRect(ctx, colBar, y + 26, w, 12, 6);
    ctx.fill();
    ctx.fillStyle = C.text;
    ctx.font = font(700, 17);
    ctx.fillText(String(r.calls.long), colBar + barW + 12, y + 38);
    // figures
    ctx.font = font(600, 15);
    ctx.fillStyle = C.text;
    ctx.fillText(r.calls.talkSeconds > 0 ? short(r.calls.talkSeconds) : "-", colTalk, y + 37);

    // Mola & Aşım durumu (Net ve Dikkat Çekici)
    const bSec = r.breakSeconds || 0;
    if (bSec > limitSec) {
      const diff = bSec - limitSec;
      ctx.fillStyle = "rgba(248, 113, 113, 0.22)";
      roundRect(ctx, colBreak - 6, y + 17, 136, 28, 6);
      ctx.fill();
      ctx.strokeStyle = "rgba(248, 113, 113, 0.6)";
      ctx.lineWidth = 1.2;
      ctx.stroke();

      ctx.fillStyle = C.red;
      ctx.font = font(700, 13.5);
      ctx.fillText(short(bSec), colBreak, y + 36);
      ctx.font = font(700, 11);
      ctx.fillText(`+${short(diff)} aştı`, colBreak + 58, y + 36);
    } else if (bSec > 0) {
      ctx.fillStyle = C.green;
      ctx.font = font(600, 14.5);
      ctx.fillText(short(bSec), colBreak, y + 37);
      ctx.fillStyle = C.dim;
      ctx.font = font(500, 11);
      ctx.fillText("uygun", colBreak + 58, y + 37);
    } else {
      ctx.fillStyle = C.dim;
      ctx.font = font(500, 14);
      ctx.fillText("0 dk", colBreak, y + 37);
    }

    ctx.font = font(600, 15);
    ctx.fillStyle = r.shift.seconds > 0 && r.calls.talkSeconds / r.shift.seconds >= 0.5 ? C.amber : C.text;
    ctx.fillText(pct(r.calls.talkSeconds, r.shift.seconds), colOcc, y + 37);
    ctx.fillStyle = C.text;
    ctx.fillText(r.calls.avgAnswerSeconds > 0 ? `${r.calls.avgAnswerSeconds} sn` : "-", colAns, y + 37);
    ctx.fillStyle = C.dim;
    ctx.fillText(`${r.calls.over10} / ${r.calls.over20}`, colLong, y + 37);
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
  ctx.fillText(`Gerçek çağrı: ${realSeconds} sn ve üstü · Mola sınırı: ${short(limitSec)} (kişi başı) · Yoğunluk: görüşme / mesai süresi`, PAD, H - 28);
  ctx.textAlign = "right";
  ctx.fillText(numericDateTime(new Date()), W - PAD, H - 28);
  ctx.textAlign = "left";

  return canvas;
}
