// In-browser video trimming and compression.
//
// No library: the video is drawn onto a canvas while being cropped,
// MediaRecorder records a new video from the canvas, the audio is taken
// from the original. The work runs in real time: a clip cut down to 30
// seconds takes 30 seconds. The clock comes from a worker so a background
// tab does not slow the recording to one frame per second.

export type VideoCrop = { x: number; y: number; width: number; height: number };
export type VideoEdit = { start: number; end: number; crop: VideoCrop };

export const FULL_CROP: VideoCrop = { x: 0, y: 0, width: 1, height: 1 };
const EDIT_EPSILON = 0.05;

export function isEdited(edit: VideoEdit, duration: number): boolean {
  const cropped = edit.crop.width < 1 || edit.crop.height < 1;
  const trimmed = edit.start > EDIT_EPSILON || edit.end < duration - EDIT_EPSILON;
  return cropped || trimmed;
}

export const MAX_OUTPUT_WIDTH = 1280;

const MIME_CANDIDATES = ["video/webm;codecs=vp9,opus", "video/webm;codecs=vp8,opus", "video/webm"];

export function pickMime(): string | null {
  if (typeof MediaRecorder === "undefined") return null;
  return MIME_CANDIDATES.find((t) => MediaRecorder.isTypeSupported(t)) ?? null;
}

export function canEditVideo(): boolean {
  const canvas = document.createElement("canvas");
  return pickMime() !== null && typeof canvas.captureStream === "function" && typeof (document.createElement("video") as HTMLVideoElement & { captureStream?: unknown }).captureStream === "function";
}

// MediaRecorder's webm has no duration in the header; seeking far ahead
// makes the browser scan the file and find the real one.
export function fixDuration(video: HTMLVideoElement): Promise<void> {
  return new Promise((resolve) => {
    if (isFinite(video.duration) && video.duration > 0) {
      resolve();
      return;
    }
    const finish = () => {
      video.removeEventListener("durationchange", changed);
      clearTimeout(timer);
      video.currentTime = 0;
      resolve();
    };
    const changed = () => {
      if (isFinite(video.duration) && video.duration > 0) finish();
    };
    const timer = setTimeout(finish, 3000);
    video.addEventListener("durationchange", changed);
    video.currentTime = 1e101;
  });
}

export function loadVideoFile(file: File): Promise<HTMLVideoElement> {
  return new Promise((resolve, reject) => {
    const video = document.createElement("video");
    const url = URL.createObjectURL(file);
    video.preload = "metadata";
    video.muted = true;
    video.playsInline = true;
    video.addEventListener(
      "loadedmetadata",
      () => {
        void fixDuration(video).then(() => {
          if (!video.duration || !isFinite(video.duration)) {
            reject(new Error("Video süresi okunamadı."));
            return;
          }
          resolve(video);
        });
      },
      { once: true },
    );
    video.addEventListener(
      "error",
      () => {
        URL.revokeObjectURL(url);
        reject(new Error("Video okunamadı."));
      },
      { once: true },
    );
    video.src = url;
  });
}

// Bit rate that fits the budget, with 15% left for audio and container.
export function targetBitrate(seconds: number, maxBytes: number): number {
  const safe = maxBytes * 0.85;
  const bps = Math.floor((safe * 8) / Math.max(seconds, 1));
  return Math.max(300_000, Math.min(bps, 8_000_000));
}

export function outputSize(video: { videoWidth: number; videoHeight: number }, crop: VideoCrop): { width: number; height: number } {
  const w = Math.round(video.videoWidth * crop.width);
  const h = Math.round(video.videoHeight * crop.height);
  const ratio = Math.min(1, MAX_OUTPUT_WIDTH / Math.max(w, 1));
  const even = (n: number) => Math.max(2, Math.round(n / 2) * 2);
  return { width: even(w * ratio), height: even(h * ratio) };
}

export function estimateBytes(seconds: number, bitrate: number): number {
  return Math.round((bitrate * seconds) / 8);
}

const LOGO_URL = "/icon-192.png";

function loadLogo(): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image();
    img.addEventListener("load", () => resolve(img), { once: true });
    img.addEventListener("error", () => resolve(null), { once: true });
    img.src = LOGO_URL;
  });
}

// The watermark is burned into the output, not a player layer, so it
// survives a download.
function drawWatermark(ctx: CanvasRenderingContext2D, width: number, height: number, logo: HTMLImageElement | null) {
  const pad = Math.round(Math.min(width, height) * 0.03);
  ctx.save();
  ctx.globalAlpha = 0.55;
  if (logo && logo.naturalWidth) {
    const w = Math.max(16, Math.round(width * 0.035));
    const h = Math.round((w * logo.naturalHeight) / logo.naturalWidth);
    ctx.drawImage(logo, width - w - pad, height - h - pad, w, h);
  }
  const pt = Math.max(10, Math.round(width * 0.018));
  ctx.font = `600 ${pt}px system-ui, sans-serif`;
  ctx.textBaseline = "bottom";
  ctx.strokeStyle = "rgba(0, 0, 0, 0.5)";
  ctx.lineWidth = Math.max(2, pt * 0.16);
  ctx.strokeText("SantralC", pad, height - pad);
  ctx.fillStyle = "rgba(255, 255, 255, 0.9)";
  ctx.fillText("SantralC", pad, height - pad);
  ctx.restore();
}

const TICKER = `let id;onmessage=e=>{if(e.data.ms){id=setInterval(()=>postMessage(0),e.data.ms)}else{clearInterval(id)}}`;

function startTicker(ms: number, tick: () => void): () => void {
  if (typeof Worker === "undefined") {
    const id = setInterval(tick, ms);
    return () => clearInterval(id);
  }
  const url = URL.createObjectURL(new Blob([TICKER], { type: "text/javascript" }));
  const worker = new Worker(url);
  worker.addEventListener("message", () => tick());
  worker.postMessage({ ms });
  return () => {
    worker.terminate();
    URL.revokeObjectURL(url);
  };
}

export const CANCEL_MESSAGE = "İşlem iptal edildi.";

export async function compressVideo(file: File, edit: VideoEdit, maxBytes: number, onProgress?: (ratio: number) => void, signal?: AbortSignal): Promise<File> {
  const mime = pickMime();
  if (!mime) throw new Error("Bu tarayıcı video sıkıştırmayı desteklemiyor.");

  const video = await loadVideoFile(file);
  const seconds = Math.max(0.5, edit.end - edit.start);
  const { width, height } = outputSize(video, edit.crop);

  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Video işlenemedi.");

  const src = {
    x: video.videoWidth * edit.crop.x,
    y: video.videoHeight * edit.crop.y,
    w: video.videoWidth * edit.crop.width,
    h: video.videoHeight * edit.crop.height,
  };

  const logo = await loadLogo();

  let stream = canvas.captureStream(0);
  let frameTrack = stream.getVideoTracks()[0] as CanvasCaptureMediaStreamTrack;
  if (typeof frameTrack?.requestFrame !== "function") {
    for (const t of stream.getTracks()) t.stop();
    stream = canvas.captureStream(30);
    frameTrack = stream.getVideoTracks()[0] as CanvasCaptureMediaStreamTrack;
  }

  const parts: BlobPart[] = [];
  try {
    const source = (video as HTMLVideoElement & { captureStream?: () => MediaStream }).captureStream?.();
    for (const t of source?.getAudioTracks() ?? []) stream.addTrack(t);
  } catch {
    // no audio, keep going with the picture
  }

  const recorder = new MediaRecorder(stream, { mimeType: mime, videoBitsPerSecond: targetBitrate(seconds, maxBytes) });
  recorder.addEventListener("dataavailable", (e) => {
    if (e.data.size) parts.push(e.data);
  });
  const stopped = new Promise<void>((resolve) => recorder.addEventListener("stop", () => resolve(), { once: true }));

  await new Promise<void>((resolve, reject) => {
    video.addEventListener("seeked", () => resolve(), { once: true });
    video.addEventListener("error", () => reject(new Error("Video okunamadı.")), { once: true });
    video.currentTime = edit.start;
  });

  recorder.start(1000);
  await video.play();

  let cancelled = false;
  await new Promise<void>((resolve) => {
    let done = false;
    let stop: (() => void) | undefined;
    const draw = () => {
      if (done) return;
      if (signal?.aborted || video.currentTime >= edit.end || video.ended) {
        done = true;
        cancelled = signal?.aborted ?? false;
        stop?.();
        video.pause();
        recorder.stop();
        resolve();
        return;
      }
      ctx.drawImage(video, src.x, src.y, src.w, src.h, 0, 0, width, height);
      drawWatermark(ctx, width, height, logo);
      frameTrack.requestFrame?.();
      onProgress?.((video.currentTime - edit.start) / seconds);
    };
    stop = startTicker(1000 / 30, draw);
    if (done) stop();
  });

  await stopped;
  for (const t of stream.getTracks()) t.stop();
  URL.revokeObjectURL(video.src);

  if (cancelled) throw new Error(CANCEL_MESSAGE);

  const blob = new Blob(parts, { type: mime.split(";")[0] });
  const name = file.name.replace(/\.[^.]+$/, "") + ".webm";
  return new File([blob], name, { type: blob.type });
}
