// CallAudio routes a call's sound through the browser's audio graph so the
// panel can draw both voices and turn the other side up past what the
// speaker element allows. The hidden audio element keeps receiving the
// remote stream (the browser only decodes a remote track that is attached
// to one) but is muted while the graph plays it through a gain node.
// Without an audio graph the element plays as before, without the boost.

export type WaveSide = "remote" | "local";

const GAIN_KEY = "santral.remote-gain";
export const GAIN_MIN = 0;
export const GAIN_MAX = 4;

export function loadGain(): number {
  try {
    const v = Number(localStorage.getItem(GAIN_KEY));
    return v > 0 && v <= GAIN_MAX ? v : 1;
  } catch {
    return 1;
  }
}

export function saveGain(v: number) {
  try {
    localStorage.setItem(GAIN_KEY, String(v));
  } catch {
    // storage unavailable; the value still applies for this session
  }
}

export type RemotePcmTap = (pcm16k: Float32Array, rms: number) => void;

function downsampleTo16k(input: Float32Array, sampleRate: number): { pcm16k: Float32Array; rms: number } {
  if (sampleRate <= 0 || input.length === 0) {
    return { pcm16k: new Float32Array(0), rms: 0 };
  }
  if (sampleRate === 16000) {
    let sumSq = 0;
    const copy = new Float32Array(input.length);
    for (let i = 0; i < input.length; i++) {
      const v = input[i];
      copy[i] = v;
      sumSq += v * v;
    }
    return { pcm16k: copy, rms: Math.sqrt(sumSq / input.length) };
  }
  const ratio = sampleRate / 16000;
  const outLen = Math.max(1, Math.floor(input.length / ratio));
  const out = new Float32Array(outLen);
  let sumSq = 0;
  for (let i = 0; i < outLen; i++) {
    const start = Math.floor(i * ratio);
    const end = Math.min(input.length, Math.floor((i + 1) * ratio));
    let acc = 0;
    let count = 0;
    for (let j = start; j < end; j++) {
      acc += input[j];
      count++;
    }
    const v = count > 0 ? acc / count : input[start] ?? 0;
    out[i] = v;
    sumSq += v * v;
  }
  return { pcm16k: out, rms: Math.sqrt(sumSq / outLen) };
}

export class CallAudio {
  private ctx: AudioContext | null = null;
  private gain: GainNode | null = null;
  private remote: AnalyserNode | null = null;
  private local: AnalyserNode | null = null;
  private remoteStreamRef: MediaStream | null = null;
  private localStreamRef: MediaStream | null = null;
  private remotePcmTap: RemotePcmTap | null = null;
  private processor: ScriptProcessorNode | null = null;
  private muteSink: GainNode | null = null;
  private sources: MediaStreamAudioSourceNode[] = [];
  private buf = new Float32Array(256);
  private bins = new Uint8Array(128);

  attach(pc: RTCPeerConnection, el: HTMLAudioElement | null, gainValue: number) {
    this.detach();
    const remoteStream = new MediaStream();
    pc.getReceivers().forEach((r) => r.track && remoteStream.addTrack(r.track));
    const localStream = new MediaStream();
    pc.getSenders().forEach((s) => s.track && s.track.kind === "audio" && localStream.addTrack(s.track));
    this.remoteStreamRef = remoteStream;
    this.localStreamRef = localStream;

    if (el) {
      el.srcObject = remoteStream;
      el.muted = false;
      void el.play().catch(() => undefined);
    }

    const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!Ctor || remoteStream.getAudioTracks().length === 0) return;
    try {
      const ctx = new Ctor();
      this.ctx = ctx;
      const gain = ctx.createGain();
      gain.gain.value = gainValue;
      this.gain = gain;
      const remote = ctx.createAnalyser();
      remote.fftSize = 256;
      remote.smoothingTimeConstant = 0.6;
      this.remote = remote;
      const src = ctx.createMediaStreamSource(remoteStream);
      src.connect(gain).connect(ctx.destination);
      src.connect(remote);
      this.sources.push(src);

      // Tap remote WebRTC audio into 16kHz mono PCM chunks for real-time
      // customer speech-to-text, routed to a 0-gain sink so playback is untouched.
      if (typeof ctx.createScriptProcessor === "function") {
        try {
          const proc = ctx.createScriptProcessor(4096, 1, 1);
          const muteSink = ctx.createGain();
          muteSink.gain.value = 0;
          proc.onaudioprocess = (ev: AudioProcessingEvent) => {
            if (!this.remotePcmTap) return;
            const ch0 = ev.inputBuffer.getChannelData(0);
            const { pcm16k, rms } = downsampleTo16k(ch0, ev.inputBuffer.sampleRate || ctx.sampleRate || 48000);
            if (pcm16k.length > 0) {
              this.remotePcmTap(pcm16k, rms);
            }
          };
          src.connect(proc);
          proc.connect(muteSink);
          muteSink.connect(ctx.destination);
          this.processor = proc;
          this.muteSink = muteSink;
        } catch {
          // ignore if script processor is unavailable in test env
        }
      }
      if (localStream.getAudioTracks().length > 0) {
        const local = ctx.createAnalyser();
        local.fftSize = 256;
        local.smoothingTimeConstant = 0.6;
        this.local = local;
        const lsrc = ctx.createMediaStreamSource(localStream);
        lsrc.connect(local);
        this.sources.push(lsrc);
      }
      void ctx.resume().then(() => {
        // The graph carries the sound now; the element only keeps the track alive.
        if (el && this.ctx === ctx && ctx.state === "running") el.muted = true;
      });
    } catch {
      this.detach();
    }
  }

  // boosted says whether the gain node is doing the playing.
  get boosted() {
    return this.ctx?.state === "running";
  }

  setGain(v: number) {
    if (this.gain && this.ctx) this.gain.gain.setTargetAtTime(v, this.ctx.currentTime, 0.02);
  }

  // wave returns the latest time-domain samples of one side, -1..1, or null
  // when that side is not wired.
  wave(side: WaveSide): Float32Array | null {
    const a = side === "remote" ? this.remote : this.local;
    if (!a) return null;
    if (this.buf.length !== a.fftSize) this.buf = new Float32Array(a.fftSize);
    a.getFloatTimeDomainData(this.buf);
    return this.buf;
  }

  // spectrum returns the latest frequency bins of one side, 0..255, or null
  // when that side is not wired.
  spectrum(side: WaveSide): Uint8Array | null {
    const a = side === "remote" ? this.remote : this.local;
    if (!a) return null;
    if (this.bins.length !== a.frequencyBinCount) this.bins = new Uint8Array(a.frequencyBinCount);
    a.getByteFrequencyData(this.bins);
    return this.bins;
  }

  // streams returns the active local and remote audio streams for live
  // speech-to-text, without disturbing the WebRTC playback graph.
  streams(): { local: MediaStream | null; remote: MediaStream | null } {
    return { local: this.localStreamRef, remote: this.remoteStreamRef };
  }

  setRemotePcmTap(tap: RemotePcmTap | null) {
    this.remotePcmTap = tap;
  }

  detach() {
    if (this.processor) {
      try {
        this.processor.onaudioprocess = null;
        this.processor.disconnect();
      } catch {
        // ignore
      }
      this.processor = null;
    }
    if (this.muteSink) {
      try {
        this.muteSink.disconnect();
      } catch {
        // ignore
      }
      this.muteSink = null;
    }
    for (const s of this.sources) {
      try {
        s.disconnect();
      } catch {
        // already gone
      }
    }
    this.sources = [];
    this.gain = null;
    this.remote = null;
    this.local = null;
    this.remoteStreamRef = null;
    this.localStreamRef = null;
    if (this.ctx) {
      void this.ctx.close().catch(() => undefined);
      this.ctx = null;
    }
  }
}
