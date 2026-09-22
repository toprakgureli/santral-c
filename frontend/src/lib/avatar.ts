// Profile photo processing: crop, resize and encode to webp in the browser.
// The server only accepts a small webp data URI, so the conversion happens
// here. The preview and the saved output go through the same drawAvatar
// call with a different size, so what the agent sees is what is stored.

export const AVATAR_SIZE = 256;
export const AVATAR_MAX_BYTES = 5 * 1024 * 1024;
export const AVATAR_QUALITY = 0.82;
export const AVATAR_ZOOM_MIN = 1;
export const AVATAR_ZOOM_MAX = 4;
export const AVATAR_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"];

// Either source draws the same way; ImageBitmap carries the phone's EXIF
// orientation already applied, an <img> does not.
export type AvatarSource = ImageBitmap | HTMLImageElement;

function sizeOf(src: AvatarSource): { width: number; height: number } {
  return src instanceof HTMLImageElement ? { width: src.naturalWidth, height: src.naturalHeight } : { width: src.width, height: src.height };
}

// x and y are fractions of the square, not pixels, so the 288px preview and
// the 256px output describe the same crop.
export interface AvatarTransform {
  zoom: number;
  x: number;
  y: number;
  rotation: number;
}

export const DEFAULT_TRANSFORM: AvatarTransform = { zoom: 1, x: 0, y: 0, rotation: 0 };

function loadImage(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const image = new Image();
    image.addEventListener("load", () => { URL.revokeObjectURL(url); resolve(image); }, { once: true });
    image.addEventListener("error", () => { URL.revokeObjectURL(url); reject(new Error("Görsel okunamadı.")); }, { once: true });
    image.src = url;
  });
}

// loadAvatarFile checks type and size, then decodes. createImageBitmap with
// imageOrientation applies the EXIF rotation phone photos carry, so a
// portrait shot does not come in sideways; older browsers fall back to <img>.
export async function loadAvatarFile(file: File): Promise<AvatarSource> {
  if (!AVATAR_TYPES.includes(file.type)) throw new Error("Yalnızca PNG, JPEG, WEBP veya GIF yükleyebilirsin.");
  if (file.size > AVATAR_MAX_BYTES) throw new Error("Görsel 5 MB sınırını aşıyor.");
  if (typeof createImageBitmap === "function") {
    try {
      return await createImageBitmap(file, { imageOrientation: "from-image" });
    } catch {
      // fall through to the plain image path
    }
  }
  return loadImage(file);
}

function rotatedSize(src: AvatarSource, rotation: number) {
  const { width, height } = sizeOf(src);
  const swapped = Math.abs(rotation) % 180 !== 0;
  return { width: swapped ? height : width, height: swapped ? width : height };
}

// The square must always be covered: panning cannot expose an empty corner.
export function clampTransform(t: AvatarTransform, src: AvatarSource): AvatarTransform {
  const { width, height } = rotatedSize(src, t.rotation);
  const shortest = Math.min(width, height);
  if (!shortest) return { ...t, x: 0, y: 0 };
  const zoom = Math.min(AVATAR_ZOOM_MAX, Math.max(AVATAR_ZOOM_MIN, t.zoom));
  const limitX = Math.max(0, ((width / shortest) * zoom - 1) / 2);
  const limitY = Math.max(0, ((height / shortest) * zoom - 1) / 2);
  return {
    zoom,
    rotation: ((t.rotation % 360) + 360) % 360,
    x: Math.min(limitX, Math.max(-limitX, t.x)),
    y: Math.min(limitY, Math.max(-limitY, t.y)),
  };
}

export function drawAvatar(canvas: HTMLCanvasElement, src: AvatarSource, size: number, t: AvatarTransform): void {
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Görsel işlenemedi.");
  canvas.width = size;
  canvas.height = size;
  const { width, height } = sizeOf(src);
  const rotated = rotatedSize(src, t.rotation);
  const shortest = Math.min(rotated.width, rotated.height) || 1;
  const scale = (size / shortest) * t.zoom;
  ctx.clearRect(0, 0, size, size);
  ctx.save();
  ctx.translate(size / 2 + t.x * size, size / 2 + t.y * size);
  ctx.rotate((t.rotation * Math.PI) / 180);
  ctx.scale(scale, scale);
  ctx.drawImage(src, -width / 2, -height / 2);
  ctx.restore();
}

// The server accepts up to 90 KB; the browser aims well under that so the
// limit is never hit. Like a phone app, it shrinks the photo itself: first
// by lowering webp quality, then by reducing the side, until it fits.
export const AVATAR_TARGET_BYTES = 60 * 1024;
const QUALITIES = [AVATAR_QUALITY, 0.72, 0.62, 0.52, 0.42, 0.32];
const SIDES = [AVATAR_SIZE, 224, 192, 160, 128];

function dataUrlBytes(dataUrl: string): number {
  const b64 = dataUrl.slice(dataUrl.indexOf(",") + 1);
  const padding = b64.endsWith("==") ? 2 : b64.endsWith("=") ? 1 : 0;
  return Math.floor((b64.length * 3) / 4) - padding;
}

export function encodeAvatar(src: AvatarSource, t: AvatarTransform): string {
  const canvas = document.createElement("canvas");
  const clamped = clampTransform(t, src);
  let smallest: string | null = null;
  for (const side of SIDES) {
    drawAvatar(canvas, src, side, clamped);
    for (const quality of QUALITIES) {
      const encoded = canvas.toDataURL("image/webp", quality);
      if (!encoded.startsWith("data:image/webp")) throw new Error("Tarayıcın webp desteklemiyor, başka bir tarayıcı dene.");
      if (dataUrlBytes(encoded) <= AVATAR_TARGET_BYTES) return encoded;
      if (!smallest || encoded.length < smallest.length) smallest = encoded;
    }
  }
  // 128px at the lowest quality is a few KB; this line is only reached on
  // a pathological encoder, and the server still has its own ceiling.
  return smallest as string;
}

// Photos are not carried in list responses (a 200-row list would be
// megabytes); they are fetched by id and cached by the browser. `version`
// changes when the photo does, so a fresh one is not stuck behind the cache.
export function avatarUrl(userId?: number | null, version?: number | null): string | undefined {
  if (!userId) return undefined;
  return `/api/v1/users/${userId}/avatar${version ? `?v=${version}` : ""}`;
}
