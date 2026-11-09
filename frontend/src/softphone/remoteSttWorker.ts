// remoteSttWorker runs Turkish automatic speech recognition on the remote
// WebRTC audio stream (16kHz mono Float32 PCM) inside a background Web Worker.
// Includes a telephony DSP pre-processor (DC removal, consonant pre-emphasis,
// voiced-frame gate, and Peak AGC normalization to 0.88) plus strict URL and
// subtitle hallucination filtering for 8kHz G.711 SIP audio.

import { env, pipeline } from "@huggingface/transformers";

env.allowLocalModels = false;
env.useBrowserCache = true;

type AsrCallable = (
  audio: Float32Array,
  options?: Record<string, unknown>,
) => Promise<{ text?: string } | { text?: string }[]>;

let transcriber: AsrCallable | null = null;
let currentModelId = "";
let loadingPromise: Promise<AsrCallable | null> | null = null;
let busy = false;
let pendingJob: { id: number; audio: Float32Array; modelId?: string; partial?: boolean } | null = null;

// Common Whisper hallucinations on narrowband telephone noise or silence.
const HALLUCINATION_PATTERNS = [
  /www\./i,
  /https?:\/\//i,
  /\.(com|net|org|edu|gov|io|tv|tr|ru|de|uk|cc|xyz|info|biz)\b/i,
  /@/,
  /altyaz[ıi]/i,
  /m\.?\s*k\.?/i,
  /abone\s+ol/i,
  /be[ğg]enmeyi\s+unutma/i,
  /izledi[ğg]iniz\s+i[çc]in/i,
  /dinledi[ğg]iniz\s+i[çc]in/i,
  /t[üu]rk[çc]e\s+altyaz/i,
  /[çc]eviri\s*:/i,
  /copyright/i,
  /all\s+rights\s+reserved/i,
  /bir sonraki videoda/i,
  /daha fazla video/i,
  /görüşmek üzere/i,
  /^\[.*\]$/,
  /^\(.*\)$/,
  /^♪+/,
  /^[.\s,!?…\-_]+$/,
];

function cleanTranscript(raw: string): string {
  const t = raw
    .replace(/\[.*?\]/g, " ")
    .replace(/\(.*?\)/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!t || t.length < 2) return "";
  for (const pat of HALLUCINATION_PATTERNS) {
    if (pat.test(t)) return "";
  }
  // Reject outputs where the same word repeats 3+ times in a row.
  const words = t.toLowerCase().split(/\s+/);
  if (words.length >= 3) {
    let streak = 1;
    for (let i = 1; i < words.length; i++) {
      if (words[i] === words[i - 1]) {
        streak++;
        if (streak >= 3) return "";
      } else {
        streak = 1;
      }
    }
  }
  return t;
}

// prepareTelephonyPcm normalizes 8kHz-upsampled SIP/WebRTC audio for Whisper:
// 1. Removes DC offset
// 2. Applies a light pre-emphasis filter to sharpen muffled telephony consonants
// 3. Checks that enough frames contain real voice energy (rejects hum/clicks)
// 4. Normalizes peak amplitude to 0.88 (AGC) so quiet callers are never misread as silence
// 5. Adds 160ms of clean silence padding at both ends so initial syllables ("Merhaba") are not clipped
function prepareTelephonyPcm(raw: Float32Array): Float32Array | null {
  const n = raw.length;
  if (n < 4000) return null; // shorter than 250ms

  // 1. DC offset mean
  let sum = 0;
  for (let i = 0; i < n; i++) sum += raw[i];
  const mean = sum / n;

  // 2. Pre-emphasis (boosts 1-3.4kHz consonant clarity on G.711 audio) + Peak measurement
  const filtered = new Float32Array(n);
  let peak = 0;
  let prev = raw[0] - mean;
  for (let i = 0; i < n; i++) {
    const cur = raw[i] - mean;
    // Blend 65% original + 35% pre-emphasized so voice stays natural while consonants sharpen
    const hp = cur - 0.85 * prev;
    prev = cur;
    const val = 0.65 * cur + 0.35 * hp;
    filtered[i] = val;
    const abs = Math.abs(val);
    if (abs > peak) peak = abs;
  }

  // If peak is tiny, there is no real speech in this buffer.
  if (peak < 0.009) return null;

  // 3. Automatic Gain Control (normalize peak to 0.88)
  const gain = Math.min(25, 0.88 / peak);
  const frameSize = 320; // 20ms at 16kHz
  let voicedFrames = 0;
  let totalFrames = 0;
  let firstVoicedSample = 0;
  let lastVoicedSample = n - 1;
  let foundFirst = false;

  for (let f = 0; f + frameSize <= n; f += frameSize) {
    let fSum = 0;
    for (let j = 0; j < frameSize; j++) {
      const idx = f + j;
      const v = Math.max(-1, Math.min(1, filtered[idx] * gain));
      filtered[idx] = v;
      fSum += v * v;
    }
    const fRms = Math.sqrt(fSum / frameSize);
    totalFrames++;
    if (fRms >= 0.028) {
      voicedFrames++;
      if (!foundFirst) {
        foundFirst = true;
        firstVoicedSample = Math.max(0, f - frameSize * 3);
      }
      lastVoicedSample = Math.min(n - 1, f + frameSize * 4);
    }
  }

  // Require at least 4 voiced frames (~80ms) to reject single pops/clicks.
  if (voicedFrames < 4 || totalFrames === 0) return null;

  // 4. Trim dead silence around the utterance and pad with 160ms (2560 samples) of clean zero padding
  const pad = 2560;
  const sliceLen = Math.max(1, lastVoicedSample - firstVoicedSample + 1);
  const out = new Float32Array(sliceLen + pad * 2);
  out.set(filtered.subarray(firstVoicedSample, lastVoicedSample + 1), pad);
  return out;
}

function post(msg: Record<string, unknown>) {
  self.postMessage(msg);
}

async function ensureModel(requestedModel?: string): Promise<AsrCallable | null> {
  const targetModel = requestedModel || currentModelId || "Xenova/whisper-small";
  if (transcriber && currentModelId === targetModel) return transcriber;
  if (loadingPromise && currentModelId === targetModel) return loadingPromise;

  currentModelId = targetModel;
  transcriber = null;

  loadingPromise = (async () => {
    try {
      const shortName = targetModel.includes("small") ? "Whisper-Small TR (Yüksek Doğruluk)" : "Whisper-Base TR";
      post({
        type: "status",
        status: "loading",
        progress: 0,
        message: `Karşı taraf ses modeli hazırlanıyor (${shortName})...`,
      });

      let lastPct = -1;
      const onProgress = (info: { status?: string; progress?: number }) => {
        if (typeof info?.progress === "number") {
          const pct = Math.round(info.progress);
          if (pct !== lastPct && pct % 10 === 0) {
            lastPct = pct;
            post({
              type: "status",
              status: "loading",
              progress: pct,
              message: `${shortName} indiriliyor (%${pct})...`,
            });
          }
        }
      };

      const asr = await pipeline("automatic-speech-recognition", targetModel, {
        dtype: "q8",
        progress_callback: onProgress,
      });

      transcriber = asr as unknown as AsrCallable;
      post({
        type: "status",
        status: "ready",
        progress: 100,
        message: `${shortName} hazır.`,
      });
      return transcriber;
    } catch (err) {
      // If whisper-small fails to download, fall back automatically to whisper-base.
      if (targetModel !== "Xenova/whisper-base") {
        loadingPromise = null;
        return ensureModel("Xenova/whisper-base");
      }
      const msg = err instanceof Error ? err.message : String(err);
      post({
        type: "status",
        status: "error",
        message: `Karşı taraf ses modeli yüklenemedi: ${msg}`,
      });
      loadingPromise = null;
      return null;
    }
  })();

  return loadingPromise;
}

async function runJob(job: { id: number; audio: Float32Array; modelId?: string; partial?: boolean }) {
  const prepared = prepareTelephonyPcm(job.audio);
  if (!prepared) {
    if (!job.partial) {
      post({
        type: "final",
        id: job.id,
        text: "",
        durationSec: 0,
      });
    }
    return;
  }

  const model = await ensureModel(job.modelId);
  if (!model) return;

  busy = true;
  try {
    const durSec = Number((prepared.length / 16000).toFixed(1));
    post({
      type: "status",
      status: "transcribing",
      message: `Karşı taraf sesi çevriliyor (${durSec} sn)...`,
    });

    const out = await model(prepared, {
      language: "turkish",
      task: "transcribe",
      return_timestamps: false,
      temperature: 0,
      condition_on_previous_text: false,
      no_repeat_ngram_size: 3,
      repetition_penalty: 1.2,
      max_new_tokens: 64,
    });

    const rawText = Array.isArray(out) ? out[0]?.text ?? "" : out?.text ?? "";
    const text = cleanTranscript(rawText);

    if (job.partial) {
      post({
        type: "partial",
        id: job.id,
        text,
        rawText: rawText.trim(),
        durationSec: durSec,
      });
    } else {
      post({
        type: "final",
        id: job.id,
        text,
        rawText: rawText.trim(),
        durationSec: durSec,
      });
    }
    post({
      type: "status",
      status: "ready",
      progress: 100,
      message: "Karşı taraf (WebRTC) ses tanıma motoru hazır.",
    });
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    post({
      type: "log",
      level: "warn",
      message: `Karşı taraf ses çeviri hatası: ${msg}`,
    });
  } finally {
    busy = false;
    if (pendingJob) {
      const next = pendingJob;
      pendingJob = null;
      void runJob(next);
    }
  }
}

self.addEventListener("message", (ev: MessageEvent) => {
  const data = ev.data as {
    type?: string;
    id?: number;
    audio?: Float32Array;
    modelId?: string;
    partial?: boolean;
  };
  if (!data || typeof data.type !== "string") return;

  if (data.type === "warmup") {
    void ensureModel(data.modelId);
    return;
  }

  if (data.type === "transcribe" && data.audio instanceof Float32Array) {
    const isPartial = Boolean(data.partial);
    const job = {
      id: Number(data.id ?? Date.now()),
      audio: data.audio,
      modelId: data.modelId,
      partial: isPartial,
    };
    if (busy) {
      // Do not overwrite an already queued final job with a partial preview.
      if (pendingJob && !pendingJob.partial && isPartial) {
        return;
      }
      pendingJob = job;
      return;
    }
    void runJob(job);
  }
});
