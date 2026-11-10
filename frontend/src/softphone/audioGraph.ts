// CallAudio routes a call's sound through the browser's audio graph so the
// panel can draw both voices and turn the other side up past what the
// speaker element allows. The hidden audio element keeps receiving the
// remote stream (the browser only decodes a remote track that is attached
// to one) while the graph plays it through a gain node.
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

export class CallAudio {
  private ctx: AudioContext | null = null;
  private gain: GainNode | null = null;
  private remote: AnalyserNode | null = null;
  private local: AnalyserNode | null = null;
  private remoteStreamRef: MediaStream | null = null;
  private localStreamRef: MediaStream | null = null;
  private sources: MediaStreamAudioSourceNode[] = [];
  private pcRef: RTCPeerConnection | null = null;
  private elRef: HTMLAudioElement | null = null;
  private currentGain = 1;
  private onTrackBound: ((ev: RTCTrackEvent) => void) | null = null;
  private buf = new Float32Array(256);
  private bins = new Uint8Array(128);

  attach(pc: RTCPeerConnection, el: HTMLAudioElement | null, gainValue: number) {
    this.detach();
    this.pcRef = pc;
    this.elRef = el;
    this.currentGain = gainValue;

    const remoteStream = new MediaStream();
    const localStream = new MediaStream();
    this.remoteStreamRef = remoteStream;
    this.localStreamRef = localStream;

    this.onTrackBound = (ev: RTCTrackEvent) => {
      if (ev.track && ev.track.kind === "audio") {
        if (!remoteStream.getTracks().includes(ev.track)) {
          remoteStream.addTrack(ev.track);
        }
        if (el && el.srcObject !== remoteStream) {
          el.srcObject = remoteStream;
        }
        this.setupAudioGraph();
      }
    };

    try {
      pc.addEventListener("track", this.onTrackBound);
    } catch {
      // test environment mock might lack addEventListener
    }

    try {
      pc.getReceivers().forEach((r) => {
        if (r.track && r.track.kind === "audio" && !remoteStream.getTracks().includes(r.track)) {
          remoteStream.addTrack(r.track);
        }
      });
      pc.getTransceivers().forEach((t) => {
        if (t.receiver?.track && t.receiver.track.kind === "audio" && !remoteStream.getTracks().includes(t.receiver.track)) {
          remoteStream.addTrack(t.receiver.track);
        }
      });
      pc.getSenders().forEach((s) => {
        if (s.track && s.track.kind === "audio" && !localStream.getTracks().includes(s.track)) {
          localStream.addTrack(s.track);
        }
      });
    } catch {
      // ignore
    }

    if (el) {
      el.srcObject = remoteStream;
      el.muted = false;
      void el.play().catch(() => undefined);
    }

    this.setupAudioGraph();
  }

  private setupAudioGraph() {
    if (!this.remoteStreamRef || this.remoteStreamRef.getAudioTracks().length === 0) {
      return;
    }
    if (this.ctx && this.ctx.state !== "closed") {
      return;
    }

    const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!Ctor) return;

    try {
      const ctx = this.ctx ?? new Ctor();
      this.ctx = ctx;

      const gain = this.gain ?? ctx.createGain();
      gain.gain.value = this.currentGain;
      this.gain = gain;

      const remote = this.remote ?? ctx.createAnalyser();
      remote.fftSize = 256;
      remote.smoothingTimeConstant = 0.6;
      this.remote = remote;

      const src = ctx.createMediaStreamSource(this.remoteStreamRef);
      src.connect(gain).connect(ctx.destination);
      src.connect(remote);
      this.sources.push(src);

      if (this.localStreamRef && this.localStreamRef.getAudioTracks().length > 0 && !this.local) {
        const local = ctx.createAnalyser();
        local.fftSize = 256;
        local.smoothingTimeConstant = 0.6;
        this.local = local;
        const lsrc = ctx.createMediaStreamSource(this.localStreamRef);
        lsrc.connect(local);
        this.sources.push(lsrc);
      }

      void ctx.resume().then(() => {
        if (this.elRef && this.ctx === ctx && ctx.state === "running") {
          this.elRef.volume = 0;
          this.elRef.muted = false;
        }
      });
    } catch {
      // fallback to basic element playback
    }
  }

  get boosted() {
    return this.ctx?.state === "running";
  }

  setGain(v: number) {
    if (this.gain && this.ctx) this.gain.gain.setTargetAtTime(v, this.ctx.currentTime, 0.02);
  }

  wave(side: WaveSide): Float32Array | null {
    const a = side === "remote" ? this.remote : this.local;
    if (!a) return null;
    if (this.buf.length !== a.fftSize) this.buf = new Float32Array(a.fftSize);
    a.getFloatTimeDomainData(this.buf);
    return this.buf;
  }

  spectrum(side: WaveSide): Uint8Array | null {
    const a = side === "remote" ? this.remote : this.local;
    if (!a) return null;
    if (this.bins.length !== a.frequencyBinCount) this.bins = new Uint8Array(a.frequencyBinCount);
    a.getByteFrequencyData(this.bins);
    return this.bins;
  }

  streams(): { local: MediaStream | null; remote: MediaStream | null } {
    return { local: this.localStreamRef, remote: this.remoteStreamRef };
  }

  detach() {
    if (this.pcRef && this.onTrackBound) {
      try {
        this.pcRef.removeEventListener("track", this.onTrackBound);
      } catch {
        // ignore
      }
      this.onTrackBound = null;
      this.pcRef = null;
    }
    this.elRef = null;
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
