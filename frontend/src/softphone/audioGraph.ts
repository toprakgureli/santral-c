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

export class CallAudio {
  private ctx: AudioContext | null = null;
  private gain: GainNode | null = null;
  private remote: AnalyserNode | null = null;
  private local: AnalyserNode | null = null;
  private sources: MediaStreamAudioSourceNode[] = [];
  private buf = new Float32Array(256);

  attach(pc: RTCPeerConnection, el: HTMLAudioElement | null, gainValue: number) {
    this.detach();
    const remoteStream = new MediaStream();
    pc.getReceivers().forEach((r) => r.track && remoteStream.addTrack(r.track));
    const localStream = new MediaStream();
    pc.getSenders().forEach((s) => s.track && s.track.kind === "audio" && localStream.addTrack(s.track));

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

  detach() {
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
    if (this.ctx) {
      void this.ctx.close().catch(() => undefined);
      this.ctx = null;
    }
  }
}
