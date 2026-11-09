// Live call transcription combining:
// 1) Browser native SpeechRecognition (tr-TR) for instant microphone capture
// 2) WebRTC remoteStream 16kHz PCM VAD + background Web Worker Whisper (tr-TR)
//    so the remote party (customer) is always transcribed even on headsets
// 3) Windowed acoustic energy attribution and real-time diagnostic logs.

import { useEffect, useState } from "react";
import { api } from "@/api/client";
import type { CallTranscript, TranscriptLine, TranscriptSpeaker } from "@/api/types";
import type { RemotePcmTap, WaveSide } from "./audioGraph";

const ENABLED_KEY = "santral.transcript.enabled";
const CACHE_KEY = "santral.transcript.cache";
const MAX_LINES = 500;
const MAX_LOGS = 80;
const MAX_CACHED_CALLS = 25;
const INTERIM_THROTTLE_MS = 120;

const SAMPLE_RATE = 16000;
const PRE_ROLL_SAMPLES = Math.floor(SAMPLE_RATE * 0.32); // 320ms pre-roll
const MIN_UTTERANCE_SAMPLES = Math.floor(SAMPLE_RATE * 0.25); // at least 250ms of speech (captures "Alo", "Evet", "Merhaba")
const PARTIAL_INTERVAL_SAMPLES = Math.floor(SAMPLE_RATE * 2.0); // partial preview every 2s
const MAX_UTTERANCE_SAMPLES = SAMPLE_RATE * 10; // flush at 10s continuous speech
const SILENCE_FLUSH_MS = 450; // 450ms pause commits the utterance
const REMOTE_VAD_THRESHOLD = 0.009;

export type SpeakerMode = "auto" | "agent" | "customer";
export type RemoteSttStatus = "idle" | "loading" | "ready" | "transcribing" | "error";

export interface TranscriptLogEntry {
  id: string;
  time: string;
  level: "info" | "ok" | "warn" | "error";
  text: string;
}

export interface LiveTranscriptState {
  enabled: boolean;
  supported: boolean;
  listening: boolean;
  callId: string | null;
  peer: string | null;
  direction: "inbound" | "outbound";
  startedAtMs: number;
  speakerMode: SpeakerMode;
  simulating: boolean;
  lines: TranscriptLine[];
  interimAgent: string;
  interimCustomer: string;
  error: string | null;
  localRms: number;
  remoteRms: number;
  hasLocalTrack: boolean;
  hasRemoteTrack: boolean;
  remoteSttStatus: RemoteSttStatus;
  remoteSttProgress: number;
  logs: TranscriptLogEntry[];
}

interface SpeechRecognitionAlternativeLike {
  transcript: string;
  confidence: number;
}

interface SpeechRecognitionResultLike {
  isFinal: boolean;
  length: number;
  [index: number]: SpeechRecognitionAlternativeLike;
}

interface SpeechRecognitionEventLike {
  resultIndex: number;
  results: {
    length: number;
    [index: number]: SpeechRecognitionResultLike;
  };
}

interface SpeechRecognitionErrorEventLike {
  error: string;
}

interface SpeechRecognitionLike {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  maxAlternatives: number;
  onresult: ((ev: SpeechRecognitionEventLike) => void) | null;
  onerror: ((ev: SpeechRecognitionErrorEventLike) => void) | null;
  onend: (() => void) | null;
  start: (track?: MediaStreamTrack) => void;
  stop: () => void;
  abort: () => void;
}

type SpeechRecognitionCtor = new () => SpeechRecognitionLike;

function getSpeechRecognitionCtor(): SpeechRecognitionCtor | null {
  if (typeof window === "undefined") return null;
  const w = window as unknown as {
    SpeechRecognition?: SpeechRecognitionCtor;
    webkitSpeechRecognition?: SpeechRecognitionCtor;
  };
  return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null;
}

function nowClock(): string {
  const d = new Date();
  return [d.getHours(), d.getMinutes(), d.getSeconds()]
    .map((n) => String(n).padStart(2, "0"))
    .join(":");
}

function loadEnabled(): boolean {
  try {
    const raw = window.localStorage.getItem(ENABLED_KEY);
    return raw === null ? true : raw === "1";
  } catch {
    return true;
  }
}

function saveEnabled(v: boolean) {
  try {
    window.localStorage.setItem(ENABLED_KEY, v ? "1" : "0");
  } catch {
    // ignore storage errors
  }
}

function readCachedTranscripts(): CallTranscript[] {
  try {
    const raw = window.sessionStorage.getItem(CACHE_KEY);
    return raw ? (JSON.parse(raw) as CallTranscript[]) : [];
  } catch {
    return [];
  }
}

function writeCachedTranscript(item: CallTranscript) {
  if (!item.callId || item.lines.length === 0) return;
  try {
    const list = readCachedTranscripts().filter((c) => c.callId !== item.callId);
    list.unshift(item);
    window.sessionStorage.setItem(CACHE_KEY, JSON.stringify(list.slice(0, MAX_CACHED_CALLS)));
  } catch {
    // ignore storage errors
  }
}

export function findCachedTranscript(params: {
  callId?: string | null;
  peer?: string | null;
}): CallTranscript | null {
  const list = readCachedTranscripts();
  if (params.callId) {
    const exact = list.find((c) => c.callId === params.callId);
    if (exact && exact.lines.length > 0) return exact;
  }
  if (params.peer) {
    const cleanPeer = params.peer.replace(/\D/g, "").slice(-10);
    if (cleanPeer.length >= 7) {
      const byPeer = list.find((c) => c.peer.replace(/\D/g, "").slice(-10) === cleanPeer);
      if (byPeer && byPeer.lines.length > 0) return byPeer;
    }
  }
  return null;
}

export async function fetchCallTranscript(params: {
  callId?: string | null;
  peer?: string | null;
  startedAt?: string | null;
}): Promise<CallTranscript> {
  const cached = findCachedTranscript(params);
  try {
    const res = await api.callTranscript({
      callId: params.callId ?? undefined,
      peer: params.peer ?? undefined,
      startedAt: params.startedAt ?? undefined,
    });
    if (res && Array.isArray(res.lines) && res.lines.length > 0) {
      writeCachedTranscript(res);
      return res;
    }
  } catch {
    // fall back to sessionStorage cache
  }
  if (cached) return cached;
  return {
    callId: params.callId ?? "",
    peer: params.peer ?? "",
    direction: "inbound",
    lines: [],
  };
}

function rmsEnergy(samples: Float32Array | null): number {
  if (!samples || samples.length === 0) return 0;
  let sum = 0;
  for (let i = 0; i < samples.length; i++) {
    const v = samples[i];
    sum += v * v;
  }
  return Math.sqrt(sum / samples.length);
}

function concatFloat32(chunks: Float32Array[], totalLength: number): Float32Array {
  const out = new Float32Array(totalLength);
  let offset = 0;
  for (const c of chunks) {
    out.set(c, offset);
    offset += c.length;
  }
  return out;
}

const SAMPLE_DIALOGUE: { speaker: TranscriptSpeaker; text: string }[] = [
  { speaker: "customer", text: "Merhaba, dün başvurduğum internet nakil işlemi hakkında bilgi almak istiyorum." },
  { speaker: "agent", text: "Merhaba, memnuniyetle yardımcı olayım. Abone numaranızı veya telefon numaranızı doğrulayabilir miyiz?" },
  { speaker: "customer", text: "Tabii, 555 123 45 67 numarası üzerinden kayıtlıyım, adresim Kadıköy Moda Mahallesi." },
  { speaker: "agent", text: "Kontrol ediyorum, nakil talebiniz saha ekibine iletilmiş ve yarın saat 14:00 ile 16:00 arasında randevunuz görünüyor." },
  { speaker: "customer", text: "Harika, peki modem değişikliği de yapılacak mı yoksa mevcut modemi kullanmaya devam mı edeceğim?" },
  { speaker: "agent", text: "Yeni adresinizde fiber altyapı olduğu için ekip gelirken yeni Wi-Fi 6 fiber modemi ücretsiz olarak getirecek." },
  { speaker: "customer", text: "Çok teşekkür ederim, yarın ekibi bekliyor olacağım. İyi çalışmalar." },
  { speaker: "agent", text: "Rica ederim, iyi günler dilerim." },
];

class LiveTranscriptEngine {
  private state: LiveTranscriptState = {
    enabled: loadEnabled(),
    supported: Boolean(getSpeechRecognitionCtor()),
    listening: false,
    callId: null,
    peer: null,
    direction: "inbound",
    startedAtMs: 0,
    speakerMode: "auto",
    simulating: false,
    lines: [],
    interimAgent: "",
    interimCustomer: "",
    error: null,
    localRms: 0,
    remoteRms: 0,
    hasLocalTrack: false,
    hasRemoteTrack: false,
    remoteSttStatus: "idle",
    remoteSttProgress: 0,
    logs: [],
  };

  private listeners = new Set<() => void>();
  private agentRec: SpeechRecognitionLike | null = null;
  private activeCall = false;
  private muted = false;
  private waveFn: ((side: WaveSide) => Float32Array | null) | null = null;
  private streamsFn: (() => { local: MediaStream | null; remote: MediaStream | null }) | null = null;
  private setRemotePcmTapFn: ((tap: RemotePcmTap | null) => void) | null = null;
  private energyTimer = 0;
  private windowRemotePeak = 0;
  private windowLocalPeak = 0;
  private simTimer = 0;
  private simStep = 0;
  private interimTimer = 0;
  private pendingAgentInterim = "";
  private pendingCustomerInterim = "";
  private seq = 0;
  private logSeq = 0;

  // Remote WebRTC PCM VAD + WebWorker Whisper state
  private remoteWorker: Worker | null = null;
  private remoteJobSeq = 0;
  private preRollChunks: Float32Array[] = [];
  private preRollLen = 0;
  private remoteSpeechChunks: Float32Array[] = [];
  private remoteSpeechLen = 0;
  private remoteSpeaking = false;
  private remoteLastVoiceMs = 0;
  private remoteLastPartialLen = 0;

  constructor() {
    if (typeof window !== "undefined") {
      window.setTimeout(() => {
        if (this.state.enabled) {
          this.ensureRemoteWorker();
        }
      }, 1000);
    }
  }

  getSnapshot = (): LiveTranscriptState => this.state;

  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  private emit(patch: Partial<LiveTranscriptState>) {
    this.state = { ...this.state, ...patch };
    this.listeners.forEach((fn) => fn());
  }

  log = (level: TranscriptLogEntry["level"], text: string) => {
    const entry: TranscriptLogEntry = {
      id: `lg-${Date.now()}-${++this.logSeq}`,
      time: nowClock(),
      level,
      text,
    };
    if (typeof console !== "undefined") {
      const fn = level === "error" ? console.error : level === "warn" ? console.warn : console.info;
      fn.call(console, `[CanlıTranskript ${entry.time}]`, text);
    }
    const nextLogs =
      this.state.logs.length >= MAX_LOGS
        ? [...this.state.logs.slice(1), entry]
        : [...this.state.logs, entry];
    this.emit({ logs: nextLogs });
  };

  clearLogs = () => {
    this.emit({ logs: [] });
  };

  setEnabled = (enabled: boolean) => {
    saveEnabled(enabled);
    this.emit({ enabled, error: null });
    this.log("info", enabled ? "Canlı transkript etkinleştirildi." : "Canlı transkript duraklatıldı.");
    if (!enabled) {
      this.stopRecognizers();
    } else if (this.activeCall) {
      this.startRecognizers();
    }
  };

  setSpeakerMode = (speakerMode: SpeakerMode) => {
    this.emit({ speakerMode });
    this.log("info", `Konuşmacı modu değiştirildi: ${speakerMode}`);
  };

  syncCall(params: {
    inCall: boolean;
    callId: string | null;
    peer: string | null;
    direction?: "inbound" | "outbound";
    answeredAt: number | null;
    muted: boolean;
    wave: (side: WaveSide) => Float32Array | null;
    streams?: () => { local: MediaStream | null; remote: MediaStream | null };
    setRemotePcmTap?: (tap: RemotePcmTap | null) => void;
  }) {
    this.muted = params.muted;
    this.waveFn = params.wave;
    if (params.streams) this.streamsFn = params.streams;
    if (params.setRemotePcmTap) this.setRemotePcmTapFn = params.setRemotePcmTap;

    if (params.inCall) {
      const cid = params.callId || `call-${params.answeredAt || Date.now()}`;
      const isNewCall = !this.activeCall || this.state.callId !== cid;
      this.activeCall = true;
      if (isNewCall) {
        this.seq = 0;
        this.windowRemotePeak = 0;
        this.windowLocalPeak = 0;
        this.resetRemoteVad();
        this.emit({
          callId: cid,
          peer: params.peer ?? "",
          direction: params.direction ?? "inbound",
          startedAtMs: params.answeredAt ?? Date.now(),
          lines: [],
          interimAgent: "",
          interimCustomer: "",
          error: null,
        });
        this.log("ok", `Çağrı başladı (${params.peer || cid}). Ses kanalları bağlanıyor...`);
      } else if (params.peer && params.peer !== this.state.peer) {
        this.emit({ peer: params.peer });
      }
      if (this.state.enabled && !this.agentRec) {
        this.startRecognizers();
      }
      this.bindRemoteAudioTap();
      this.startEnergyMonitor();
    } else if (this.activeCall) {
      this.finishCall();
    }
  }

  pushLine = (speaker: TranscriptSpeaker, text: string) => {
    const clean = text.trim();
    if (!clean) return;
    const started = this.state.startedAtMs || Date.now();
    const atSec = Math.max(0, Math.round((Date.now() - started) / 1000));
    const prev = this.state.lines;
    const last = prev[prev.length - 1];
    let nextLines: TranscriptLine[];

    // Deduplicate if both mic and remote worker caught the exact same phrase within 3s.
    if (last && Math.abs(atSec - last.atSec) <= 3 && last.text.toLowerCase() === clean.toLowerCase()) {
      return;
    }

    // Merge consecutive short fragments from the same speaker within 4 seconds
    // so natural pauses do not split one sentence into many tiny rows.
    if (last && last.speaker === speaker && atSec - last.atSec <= 4 && last.text.length < 180) {
      const merged: TranscriptLine = {
        ...last,
        text: `${last.text} ${clean}`,
      };
      nextLines = [...prev.slice(0, -1), merged];
    } else {
      const line: TranscriptLine = {
        id: `tr-${Date.now()}-${++this.seq}`,
        speaker,
        text: clean,
        atSec,
      };
      nextLines = prev.length >= MAX_LINES ? [...prev.slice(1), line] : [...prev, line];
    }

    this.log("ok", `${speaker === "customer" ? "Müşteri" : "Temsilci"}: "${clean}"`);

    this.emit({
      lines: nextLines,
      interimAgent: speaker === "agent" ? "" : this.state.interimAgent,
      interimCustomer: speaker === "customer" ? "" : this.state.interimCustomer,
    });

    if (this.state.callId) {
      writeCachedTranscript({
        callId: this.state.callId,
        peer: this.state.peer ?? "",
        direction: this.state.direction,
        startedAt: new Date(started).toISOString(),
        endedAt: new Date().toISOString(),
        lines: nextLines,
      });
    }
  };

  clearLines = () => {
    this.emit({ lines: [], interimAgent: "", interimCustomer: "", error: null });
  };

  toggleSimulation = () => {
    if (this.state.simulating) {
      this.stopSimulation();
      return;
    }
    if (!this.state.callId) {
      this.emit({
        callId: `sim-${Date.now()}`,
        peer: this.state.peer || "05551234567",
        startedAtMs: this.state.startedAtMs || Date.now(),
      });
    }
    this.emit({ simulating: true, error: null });
    this.log("info", "Örnek canlı konuşma simülasyonu başlatıldı.");
    const runNext = () => {
      if (!this.state.simulating) return;
      const item = SAMPLE_DIALOGUE[this.simStep % SAMPLE_DIALOGUE.length];
      this.simStep++;
      const words = item.text.split(" ");
      let idx = 0;
      const typeWord = () => {
        if (!this.state.simulating) return;
        idx++;
        const partial = words.slice(0, idx).join(" ");
        if (idx < words.length) {
          if (item.speaker === "agent") {
            this.emit({ interimAgent: partial });
          } else {
            this.emit({ interimCustomer: partial });
          }
          this.simTimer = window.setTimeout(typeWord, 140);
        } else {
          this.pushLine(item.speaker, item.text);
          this.simTimer = window.setTimeout(runNext, 1100);
        }
      };
      typeWord();
    };
    runNext();
  };

  private stopSimulation() {
    if (this.simTimer) {
      window.clearTimeout(this.simTimer);
      this.simTimer = 0;
    }
    if (this.state.simulating) {
      this.emit({ simulating: false, interimAgent: "", interimCustomer: "" });
    }
  }

  private scheduleInterim(speaker: TranscriptSpeaker, text: string) {
    if (speaker === "agent") this.pendingAgentInterim = text;
    else this.pendingCustomerInterim = text;
    if (this.interimTimer) return;
    this.interimTimer = window.setTimeout(() => {
      this.interimTimer = 0;
      this.emit({
        interimAgent: this.pendingAgentInterim,
        interimCustomer: this.pendingCustomerInterim,
      });
    }, INTERIM_THROTTLE_MS);
  }

  private resolveSpeaker(defaultSpeaker: TranscriptSpeaker): TranscriptSpeaker {
    if (this.state.speakerMode === "agent") return "agent";
    if (this.state.speakerMode === "customer") return "customer";
    if (this.muted && defaultSpeaker === "agent") return "customer";
    // In automatic mode:
    // Browser SpeechRecognition (rec) is the local physical microphone -> "agent".
    // WebRTC PCM Worker (remoteWorker) is the remote SIP audio stream -> "customer".
    return defaultSpeaker;
  }

  private startEnergyMonitor() {
    if (this.energyTimer) return;
    this.energyTimer = window.setInterval(() => {
      const streams = this.streamsFn?.() ?? { local: null, remote: null };
      const hasLocalTrack = Boolean(streams.local && streams.local.getAudioTracks().length > 0);
      const hasRemoteTrack = Boolean(streams.remote && streams.remote.getAudioTracks().length > 0);

      const remoteRms = this.waveFn ? rmsEnergy(this.waveFn("remote")) : 0;
      const localRms = !this.muted && this.waveFn ? rmsEnergy(this.waveFn("local")) : 0;

      if (remoteRms > this.windowRemotePeak) this.windowRemotePeak = remoteRms;
      if (localRms > this.windowLocalPeak) this.windowLocalPeak = localRms;

      this.emit({
        localRms: Number(localRms.toFixed(3)),
        remoteRms: Number(remoteRms.toFixed(3)),
        hasLocalTrack,
        hasRemoteTrack,
      });
    }, 150);
  }

  private stopEnergyMonitor() {
    if (this.energyTimer) {
      window.clearInterval(this.energyTimer);
      this.energyTimer = 0;
    }
    this.emit({ localRms: 0, remoteRms: 0, hasLocalTrack: false, hasRemoteTrack: false });
  }

  private ensureRemoteWorker(): Worker | null {
    if (typeof window === "undefined" || typeof Worker === "undefined") return null;
    if (this.remoteWorker) return this.remoteWorker;
    try {
      const w = new Worker(new URL("./remoteSttWorker.ts", import.meta.url), { type: "module" });
      w.onmessage = (ev: MessageEvent) => {
        const d = ev.data as {
          type?: string;
          status?: RemoteSttStatus;
          progress?: number;
          message?: string;
          level?: TranscriptLogEntry["level"];
          text?: string;
          durationSec?: number;
        };
        if (!d || typeof d.type !== "string") return;

        if (d.type === "status" && d.status) {
          const prevStatus = this.state.remoteSttStatus;
          this.emit({
            remoteSttStatus: d.status,
            remoteSttProgress: typeof d.progress === "number" ? d.progress : this.state.remoteSttProgress,
          });
          if (d.message && (d.status !== prevStatus || d.status === "loading" || d.status === "error")) {
            this.log(d.status === "error" ? "error" : d.status === "ready" ? "ok" : "info", d.message);
          }
          return;
        }

        if (d.type === "log" && d.message) {
          this.log(d.level ?? "info", d.message);
          return;
        }

        if (d.type === "partial") {
          if (d.text && this.activeCall && this.state.enabled) {
            this.scheduleInterim("customer", d.text);
          }
          return;
        }

        if (d.type === "final") {
          this.scheduleInterim("customer", "");
          if (d.text && this.activeCall && this.state.enabled) {
            this.pushLine("customer", d.text);
          }
        }
      };
      w.onerror = (err) => {
        this.log("warn", `Karşı taraf STT Worker uyarısı: ${err.message || "Bilinmeyen hata"}`);
      };
      this.remoteWorker = w;
      w.postMessage({ type: "warmup" });
      return w;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      this.log("warn", `Karşı taraf STT Worker başlatılamadı: ${msg}`);
      return null;
    }
  }

  private resetRemoteVad() {
    this.preRollChunks = [];
    this.preRollLen = 0;
    this.remoteSpeechChunks = [];
    this.remoteSpeechLen = 0;
    this.remoteSpeaking = false;
    this.remoteLastVoiceMs = 0;
    this.remoteLastPartialLen = 0;
  }

  private flushRemoteUtterance() {
    if (this.remoteSpeechLen >= MIN_UTTERANCE_SAMPLES && this.state.enabled) {
      const audio = concatFloat32(this.remoteSpeechChunks, this.remoteSpeechLen);
      const worker = this.ensureRemoteWorker();
      if (worker) {
        const id = ++this.remoteJobSeq;
        this.log("info", `Karşı taraf konuşma bloğu çevriliyor (${(audio.length / SAMPLE_RATE).toFixed(1)} sn)...`);
        worker.postMessage({ type: "transcribe", id, audio, partial: false }, [audio.buffer]);
      }
    } else if (this.pendingCustomerInterim === "...") {
      this.scheduleInterim("customer", "");
    }
    this.remoteSpeechChunks = [];
    this.remoteSpeechLen = 0;
    this.remoteSpeaking = false;
    this.remoteLastPartialLen = 0;
  }

  private handleRemotePcm = (pcm16k: Float32Array, rms: number) => {
    if (!this.activeCall || !this.state.enabled) return;
    const now = Date.now();

    if (rms >= REMOTE_VAD_THRESHOLD) {
      if (!this.remoteSpeaking) {
        this.remoteSpeaking = true;
        this.remoteLastPartialLen = 0;
        if (this.preRollLen > 0) {
          this.remoteSpeechChunks = [...this.preRollChunks];
          this.remoteSpeechLen = this.preRollLen;
          this.preRollChunks = [];
          this.preRollLen = 0;
        }
        if (!this.pendingCustomerInterim) {
          this.scheduleInterim("customer", "...");
        }
        this.log("info", `Karşı taraftan ses algılandı (RMS: ${rms.toFixed(3)})`);
      }
      this.remoteLastVoiceMs = now;
      this.remoteSpeechChunks.push(pcm16k);
      this.remoteSpeechLen += pcm16k.length;

      // Send a live partial preview every ~1.8s of speech so words appear while customer talks.
      if (this.remoteSpeechLen - this.remoteLastPartialLen >= PARTIAL_INTERVAL_SAMPLES) {
        this.remoteLastPartialLen = this.remoteSpeechLen;
        const previewAudio = concatFloat32(this.remoteSpeechChunks, this.remoteSpeechLen);
        const worker = this.ensureRemoteWorker();
        if (worker) {
          const id = ++this.remoteJobSeq;
          worker.postMessage({ type: "transcribe", id, audio: previewAudio, partial: true }, [previewAudio.buffer]);
        }
      }

      if (this.remoteSpeechLen >= MAX_UTTERANCE_SAMPLES) {
        this.flushRemoteUtterance();
      }
    } else if (this.remoteSpeaking) {
      this.remoteSpeechChunks.push(pcm16k);
      this.remoteSpeechLen += pcm16k.length;
      if (now - this.remoteLastVoiceMs >= SILENCE_FLUSH_MS) {
        this.flushRemoteUtterance();
      }
    } else {
      // Maintain rolling 320ms pre-roll buffer during silence.
      this.preRollChunks.push(pcm16k);
      this.preRollLen += pcm16k.length;
      while (this.preRollLen > PRE_ROLL_SAMPLES && this.preRollChunks.length > 1) {
        const dropped = this.preRollChunks.shift()!;
        this.preRollLen -= dropped.length;
      }
    }
  };

  private bindRemoteAudioTap() {
    if (!this.setRemotePcmTapFn) return;
    if (this.state.enabled && this.activeCall) {
      this.ensureRemoteWorker();
      this.setRemotePcmTapFn(this.handleRemotePcm);
    } else {
      this.setRemotePcmTapFn(null);
    }
  }

  private startRecognizers() {
    const Ctor = getSpeechRecognitionCtor();
    if (!Ctor) {
      this.emit({ supported: false });
      this.log("warn", "Bu tarayıcıda Web Speech API (webkitSpeechRecognition) desteklenmiyor; karşı taraf için WebRTC Worker aktif.");
      this.bindRemoteAudioTap();
      return;
    }
    this.stopRecognizers();
    this.bindRemoteAudioTap();

    try {
      const rec = new Ctor();
      rec.lang = "tr-TR";
      rec.continuous = true;
      rec.interimResults = true;
      rec.maxAlternatives = 1;
      rec.onresult = (ev: SpeechRecognitionEventLike) => {
        let interim = "";
        for (let i = ev.resultIndex; i < ev.results.length; i++) {
          const r = ev.results[i];
          const txt = r[0]?.transcript ?? "";
          if (r.isFinal) {
            const spk = this.resolveSpeaker("agent");
            this.scheduleInterim(spk, "");
            this.pushLine(spk, txt);
          } else {
            interim += txt;
          }
        }
        const spk = this.resolveSpeaker("agent");
        this.scheduleInterim(spk, interim.trim());
      };
      rec.onerror = (ev: SpeechRecognitionErrorEventLike) => {
        if (ev.error === "not-allowed" || ev.error === "service-not-allowed") {
          const msg = "Mikrofon konuşma tanıma izni verilmedi.";
          this.emit({ error: msg, listening: false });
          this.log("error", `${msg} (${ev.error})`);
          this.stopRecognizers();
        } else if (ev.error !== "no-speech" && ev.error !== "aborted") {
          this.log("warn", `Mikrofon STT uyarısı: ${ev.error}`);
        }
      };
      rec.onend = () => {
        if (this.activeCall && this.state.enabled && this.agentRec === rec) {
          try {
            rec.start();
          } catch {
            window.setTimeout(() => {
              if (this.activeCall && this.state.enabled && this.agentRec === rec) {
                try {
                  rec.start();
                } catch {
                  // ignore
                }
              }
            }, 400);
          }
        }
      };

      rec.start();
      this.agentRec = rec;
      this.emit({ listening: true, error: null });
      this.log("ok", "Mikrofon STT (WebSpeech tr-TR) ve WebRTC Karşı Taraf STT başlatıldı.");
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      this.emit({ listening: false });
      this.log("error", `Mikrofon STT başlatılamadı: ${msg}`);
    }
  }

  private stopRecognizers() {
    this.stopSimulation();
    if (this.setRemotePcmTapFn) {
      this.setRemotePcmTapFn(null);
    }
    this.resetRemoteVad();
    if (this.interimTimer) {
      window.clearTimeout(this.interimTimer);
      this.interimTimer = 0;
    }
    this.pendingAgentInterim = "";
    this.pendingCustomerInterim = "";
    const a = this.agentRec;
    this.agentRec = null;
    if (a) {
      try {
        a.onend = null;
        a.stop();
      } catch {
        // ignore
      }
    }
    if (this.state.listening || this.state.interimAgent || this.state.interimCustomer) {
      this.emit({ listening: false, interimAgent: "", interimCustomer: "" });
    }
  }

  private finishCall() {
    this.activeCall = false;
    this.flushRemoteUtterance();
    this.stopEnergyMonitor();
    if (this.pendingAgentInterim.trim()) {
      this.pushLine("agent", this.pendingAgentInterim);
    }
    if (this.pendingCustomerInterim.trim() && this.pendingCustomerInterim.trim() !== "...") {
      this.pushLine("customer", this.pendingCustomerInterim);
    }
    this.stopRecognizers();
    this.log("info", "Çağrı sona erdi, transkript kaydediliyor.");

    const { callId, peer, direction, startedAtMs, lines } = this.state;
    if (callId && lines.length > 0) {
      const startedIso = new Date(startedAtMs || Date.now()).toISOString();
      const endedIso = new Date().toISOString();
      const payload: CallTranscript = {
        callId,
        peer: peer ?? "",
        direction,
        startedAt: startedIso,
        endedAt: endedIso,
        lines,
      };
      writeCachedTranscript(payload);
      void api
        .saveCallTranscript({
          callId,
          direction,
          peer: peer ?? undefined,
          startedAt: startedIso,
          endedAt: endedIso,
          lines,
        })
        .catch(() => {
          // kept in sessionStorage cache even if backend is unreachable
        });
    }
  }
}

export const liveTranscript = new LiveTranscriptEngine();

export function useLiveTranscript(): LiveTranscriptState {
  const [state, setState] = useState<LiveTranscriptState>(liveTranscript.getSnapshot);
  useEffect(() => liveTranscript.subscribe(() => setState(liveTranscript.getSnapshot())), []);
  return state;
}
