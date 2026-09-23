// Chat attachments: limits, kinds, previews and the direct-to-Drive upload.
//
// Bytes never touch our server on the way up. The server opens a
// resumable session at Google with its own token and returns the session
// URL; the browser PUTs the file there in chunks, with progress, and can
// resume after a dropped connection. When Google confirms the file, the
// browser tells the server the Drive id plus a small preview, and the
// server verifies the file before marking the attachment ready.

import { api } from "@/api/client";
import type { TeamsAttachment } from "@/api/types";
import { stripMarkup } from "@/lib/markup";

export type Kind = "image" | "video" | "file";

export const IMAGE_MAX = 200 * 1024 * 1024;
export const VIDEO_MAX = 1024 * 1024 * 1024;
export const FILE_MAX = 3 * 1024 * 1024 * 1024;
export const MAX_PER_MESSAGE = 10;
export const CANCEL_MESSAGE = "Yükleme iptal edildi.";

const THUMB_SIDE = 480;

export function kindOf(mime: string): Kind {
  if (mime.startsWith("image/")) return "image";
  if (mime.startsWith("video/")) return "video";
  return "file";
}

export function limitFor(kind: Kind): number {
  return kind === "image" ? IMAGE_MAX : kind === "video" ? VIDEO_MAX : FILE_MAX;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function formatDuration(ms: number): string {
  const s = Math.round(ms / 1000);
  const m = Math.floor(s / 60);
  return `${m}:${String(s % 60).padStart(2, "0")}`;
}

export function extensionOf(name: string, mime: string): string {
  const dot = name.lastIndexOf(".");
  if (dot > 0 && dot < name.length - 1) return name.slice(dot + 1).toUpperCase();
  return (mime.split("/")[1] ?? "DOSYA").toUpperCase();
}

// checkFile says why a file cannot be sent, or null.
export function checkFile(file: File, count: number): string | null {
  if (count >= MAX_PER_MESSAGE) return `Bir mesaja en fazla ${MAX_PER_MESSAGE} dosya eklenebilir.`;
  if (file.size === 0) return `${file.name}: dosya boş görünüyor.`;
  const kind = kindOf(file.type);
  const limit = limitFor(kind);
  if (file.size > limit) {
    const label = kind === "image" ? "Görsel" : kind === "video" ? "Video" : "Dosya";
    return `${label} en fazla ${formatSize(limit)} olabilir (${file.name}: ${formatSize(file.size)}).`;
  }
  return null;
}

export interface PendingAttachment {
  key: string;
  file: File;
  name: string;
  mime: string;
  size: number;
  kind: Kind;
  previewUrl: string;
  status: "hazırlanıyor" | "yükleniyor" | "hazır" | "hata";
  progress: number;
  error?: string;
  attachmentId?: number;
  view?: TeamsAttachment;
}

let counter = 0;
export function newKey(): string {
  counter += 1;
  return `up-${counter}-${Date.now()}`;
}

// ---------------------------------------------------------------- previews

export interface Preview {
  thumb?: string;
  width?: number;
  height?: number;
  durationMs?: number;
}

function toWebp(canvas: HTMLCanvasElement): Promise<string | undefined> {
  return new Promise((resolve) => {
    canvas.toBlob(
      (blob) => {
        if (!blob) return resolve(undefined);
        const r = new FileReader();
        r.onload = () => resolve(typeof r.result === "string" ? r.result : undefined);
        r.onerror = () => resolve(undefined);
        r.readAsDataURL(blob);
      },
      "image/webp",
      0.8,
    );
  });
}

function fit(w: number, h: number): { width: number; height: number } {
  const scale = Math.min(1, THUMB_SIDE / Math.max(w, h, 1));
  return { width: Math.max(1, Math.round(w * scale)), height: Math.max(1, Math.round(h * scale)) };
}

async function imagePreview(file: File): Promise<Preview> {
  try {
    const bmp = await createImageBitmap(file, { imageOrientation: "from-image" } as ImageBitmapOptions);
    const { width, height } = fit(bmp.width, bmp.height);
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext("2d");
    if (!ctx) return { width: bmp.width, height: bmp.height };
    ctx.drawImage(bmp, 0, 0, width, height);
    const thumb = await toWebp(canvas);
    const out = { thumb, width: bmp.width, height: bmp.height };
    bmp.close();
    return out;
  } catch {
    return {};
  }
}

function videoPreview(file: File): Promise<Preview> {
  return new Promise((resolve) => {
    const video = document.createElement("video");
    const url = URL.createObjectURL(file);
    let done = false;
    const finish = (p: Preview) => {
      if (done) return;
      done = true;
      URL.revokeObjectURL(url);
      resolve(p);
    };
    const timer = window.setTimeout(() => finish({}), 10000);
    video.preload = "metadata";
    video.muted = true;
    video.playsInline = true;
    video.addEventListener("loadedmetadata", () => {
      const dur = Number.isFinite(video.duration) ? video.duration : 0;
      video.currentTime = Math.min(1, dur / 2 || 0.1);
    });
    video.addEventListener("seeked", () => {
      void (async () => {
        try {
          const { width, height } = fit(video.videoWidth, video.videoHeight);
          const canvas = document.createElement("canvas");
          canvas.width = width;
          canvas.height = height;
          const ctx = canvas.getContext("2d");
          if (ctx) ctx.drawImage(video, 0, 0, width, height);
          const thumb = ctx ? await toWebp(canvas) : undefined;
          window.clearTimeout(timer);
          finish({ thumb, width: video.videoWidth, height: video.videoHeight, durationMs: Number.isFinite(video.duration) ? Math.round(video.duration * 1000) : undefined });
        } catch {
          window.clearTimeout(timer);
          finish({});
        }
      })();
    });
    video.addEventListener("error", () => {
      window.clearTimeout(timer);
      finish({});
    });
    video.src = url;
  });
}

// makePreview builds the small card the room shows before the bytes load.
export async function makePreview(file: File, kind: Kind): Promise<Preview> {
  if (kind === "image") return imagePreview(file);
  if (kind === "video") return videoPreview(file);
  return {};
}

// ---------------------------------------------------------------- upload

interface PutResult {
  status: number;
  received: number | null;
  body: string;
}

function parseRange(header: string | null): number | null {
  if (!header) return null;
  const m = /bytes=0-(\d+)/.exec(header);
  return m ? Number(m[1]) + 1 : null;
}

function put(url: string, chunk: Blob | null, range: string, onProgress: ((loaded: number) => void) | null, signal: AbortSignal): Promise<PutResult> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const abort = () => xhr.abort();
    signal.addEventListener("abort", abort, { once: true });
    xhr.open("PUT", url);
    xhr.setRequestHeader("Content-Range", range);
    if (onProgress) xhr.upload.addEventListener("progress", (e) => onProgress(e.loaded));
    xhr.addEventListener("load", () => {
      signal.removeEventListener("abort", abort);
      resolve({ status: xhr.status, received: parseRange(xhr.getResponseHeader("Range")), body: xhr.responseText });
    });
    xhr.addEventListener("error", () => {
      signal.removeEventListener("abort", abort);
      reject(new Error("network"));
    });
    xhr.addEventListener("abort", () => {
      signal.removeEventListener("abort", abort);
      reject(new Error(CANCEL_MESSAGE));
    });
    xhr.send(chunk);
  });
}

// uploadToDrive pushes the file into the session in chunks and returns the
// Drive file id. A dropped chunk is retried from where Google says it
// stopped, a few times, before giving up.
export async function uploadToDrive(uploadUrl: string, chunkBytes: number, file: File, onProgress: (ratio: number) => void, signal: AbortSignal): Promise<string> {
  const total = file.size;
  let offset = 0;
  let failures = 0;
  const chunk = Math.max(256 * 1024, Math.floor(chunkBytes / (256 * 1024)) * 256 * 1024);
  while (offset < total) {
    if (signal.aborted) throw new Error(CANCEL_MESSAGE);
    const end = Math.min(offset + chunk, total);
    const piece = file.slice(offset, end);
    let r: PutResult;
    try {
      r = await put(uploadUrl, piece, `bytes ${offset}-${end - 1}/${total}`, (loaded) => onProgress(Math.min(1, (offset + loaded) / total)), signal);
    } catch (e) {
      if (e instanceof Error && e.message === CANCEL_MESSAGE) throw e;
      failures++;
      if (failures > 6) throw new Error("Bağlantı koptu, yükleme tamamlanamadı.");
      await new Promise((res) => setTimeout(res, 1000 * failures));
      // Ask Google how much it has and continue from there.
      try {
        const st = await put(uploadUrl, null, `bytes */${total}`, null, signal);
        if (st.status === 308) offset = st.received ?? offset;
        else if (st.status === 200 || st.status === 201) return JSON.parse(st.body).id as string;
      } catch {
        // keep the same offset
      }
      continue;
    }
    if (r.status === 308) {
      offset = r.received ?? end;
      failures = 0;
      onProgress(offset / total);
      continue;
    }
    if (r.status === 200 || r.status === 201) {
      onProgress(1);
      const id = JSON.parse(r.body).id as string | undefined;
      if (!id) throw new Error("Drive dosya kimliği alınamadı.");
      return id;
    }
    if (r.status === 404 || r.status === 410) throw new Error("Yükleme oturumu sona erdi, yeniden ekle.");
    if (r.status >= 500) {
      failures++;
      if (failures > 6) throw new Error(`Google yanıtı ${r.status}, yükleme tamamlanamadı.`);
      await new Promise((res) => setTimeout(res, 1000 * failures));
      continue;
    }
    throw new Error(`Yükleme reddedildi (${r.status}).`);
  }
  throw new Error("Yükleme tamamlanamadı.");
}

// uploadFile runs the whole pipeline for one file.
export async function uploadFile(
  groupId: number,
  file: File,
  kind: Kind,
  onProgress: (ratio: number) => void,
  onSession: (attachmentId: number) => void,
  signal: AbortSignal,
): Promise<TeamsAttachment> {
  const previewP = makePreview(file, kind);
  const session = await api.teamsBeginUpload(groupId, { name: file.name, mime: file.type || "application/octet-stream", size: file.size });
  onSession(session.attachmentId);
  const driveId = await uploadToDrive(session.uploadUrl, session.chunkBytes, file, onProgress, signal);
  const preview = await previewP;
  return api.teamsFinishUpload(session.attachmentId, { driveId, width: preview.width, height: preview.height, durationMs: preview.durationMs, thumb: preview.thumb });
}

export function attachmentUrl(id: number, download = false): string {
  return `/api/v1/teams/attachments/${id}${download ? "?download=1" : ""}`;
}

export function thumbUrl(a: TeamsAttachment): string | null {
  if (a.hasThumb) return `/api/v1/teams/attachments/${a.id}/thumb`;
  if (a.kind === "image") return attachmentUrl(a.id);
  return null;
}

// previewLabel is what the room list shows for a line with files.
export function previewLabel(body: string, attachments: TeamsAttachment[] | undefined): string {
  if (body) return stripMarkup(body);
  if (!attachments?.length) return "";
  const a = attachments[0];
  const more = attachments.length > 1 ? ` +${attachments.length - 1}` : "";
  if (a.kind === "image") return `📷 Fotoğraf${more}`;
  if (a.kind === "video") return `🎬 Video${more}`;
  return `📎 ${a.name}${more}`;
}
