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

export function encodeAvatar(src: AvatarSource, t: AvatarTransform): string {
  const canvas = document.createElement("canvas");
  drawAvatar(canvas, src, AVATAR_SIZE, clampTransform(t, src));
  const encoded = canvas.toDataURL("image/webp", AVATAR_QUALITY);
  if (!encoded.startsWith("data:image/webp")) throw new Error("Tarayıcın webp desteklemiyor, başka bir tarayıcı dene.");
  return encoded;
}

// Photos are not carried in list responses (a 200-row list would be
// megabytes); they are fetched by id and cached by the browser. `version`
// changes when the photo does, so a fresh one is not stuck behind the cache.
export function avatarUrl(userId?: number | null, version?: number | null): string | undefined {
  if (!userId) return undefined;
  return `/api/v1/users/${userId}/avatar${version ? `?v=${version}` : ""}`;
}
