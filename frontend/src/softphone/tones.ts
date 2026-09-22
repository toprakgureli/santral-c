// Call-progress tones generated with the Web Audio API, so no audio assets are
// needed. Frequencies follow the Turkish 425 Hz call-progress standard.

type Segment = { on: boolean; ms: number };

let ctx: AudioContext | null = null;
let gain: GainNode | null = null;
let oscillators: OscillatorNode[] = [];
let timer: number | null = null;

function audio(): AudioContext {
  if (!ctx) {
    const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    ctx = new Ctor();
  }
  if (ctx.state === "suspended") void ctx.resume();
  return ctx;
}

function clear() {
  if (timer !== null) {
    clearTimeout(timer);
    timer = null;
  }
  if (gain) {
    try {
      gain.gain.cancelScheduledValues(0);
      gain.gain.value = 0;
    } catch {
      // ignore
    }
  }
  for (const o of oscillators) {
    try {
      o.stop();
      o.disconnect();
    } catch {
      // ignore
    }
  }
  oscillators = [];
  if (gain) {
    gain.disconnect();
    gain = null;
  }
}

function playCadence(freqs: number[], volume: number, cadence: Segment[]) {
  clear();
  const c = audio();
  gain = c.createGain();
  gain.gain.value = 0;
  gain.connect(c.destination);
  oscillators = freqs.map((f) => {
    const o = c.createOscillator();
    o.type = "sine";
    o.frequency.value = f;
    o.connect(gain!);
    o.start();
    return o;
  });

  let i = 0;
  const step = () => {
    const seg = cadence[i % cadence.length];
    if (gain) gain.gain.setTargetAtTime(seg.on ? volume : 0, c.currentTime, 0.005);
    i++;
    timer = window.setTimeout(step, seg.ms);
  };
  step();
}

// beepBurst plays `count` short beeps once (not looped).
function beepBurst(count: number, freq: number, onMs: number, gapMs: number) {
  clear();
  const c = audio();
  const g = c.createGain();
  g.gain.value = 0;
  g.connect(c.destination);
  const o = c.createOscillator();
  o.type = "sine";
  o.frequency.value = freq;
  o.connect(g);
  o.start();
  let t = c.currentTime;
  for (let i = 0; i < count; i++) {
    g.gain.setTargetAtTime(0.16, t, 0.004);
    g.gain.setTargetAtTime(0, t + onMs / 1000, 0.01);
    t += (onMs + gapMs) / 1000;
  }
  window.setTimeout(
    () => {
      try {
        o.stop();
        o.disconnect();
        g.disconnect();
      } catch {
        // ignore
      }
    },
    count * (onMs + gapMs) + 100,
  );
}

// A short two-note chime for a new message. It uses its own gain node so it
// never interrupts a ringing or in-call tone.
function chime() {
  try {
    const c = audio();
    const g = c.createGain();
    g.gain.value = 0;
    g.connect(c.destination);
    const notes: [number, number][] = [[660, 0], [880, 0.12]];
    const oscs: OscillatorNode[] = [];
    for (const [freq, at] of notes) {
      const o = c.createOscillator();
      o.type = "sine";
      o.frequency.value = freq;
      o.connect(g);
      o.start(c.currentTime + at);
      o.stop(c.currentTime + at + 0.18);
      oscs.push(o);
    }
    const t = c.currentTime;
    g.gain.setTargetAtTime(0.12, t, 0.005);
    g.gain.setTargetAtTime(0, t + 0.27, 0.03);
    window.setTimeout(() => {
      try {
        oscs.forEach((o) => o.disconnect());
        g.disconnect();
      } catch {
        // ignore
      }
    }, 500);
  } catch {
    // audio unavailable
  }
}

export const tones = {
  // New chat message.
  notify() {
    chime();
  },

  // Outbound ringback: 425 Hz, 2s on / 4s off (Turkish standard).
  ringback() {
    playCadence([425], 0.14, [
      { on: true, ms: 2000 },
      { on: false, ms: 4000 },
    ]);
  },

  // Incoming ring: a two-burst pattern with a pause.
  incoming() {
    playCadence([480, 440], 0.18, [
      { on: true, ms: 400 },
      { on: false, ms: 200 },
      { on: true, ms: 400 },
      { on: false, ms: 2000 },
    ]);
  },

  // Busy tone: 425 Hz, 0.5s on / 0.5s off (played briefly then stopped).
  busy() {
    playCadence([425], 0.14, [
      { on: true, ms: 500 },
      { on: false, ms: 500 },
    ]);
  },

  // Congestion / unreachable: faster 425 Hz cadence.
  congestion() {
    playCadence([425], 0.14, [
      { on: true, ms: 250 },
      { on: false, ms: 250 },
    ]);
  },

  stop() {
    clear();
  },

  // Resume the audio context; must be called from a user gesture so later
  // sounds (like the incoming ring) are allowed to play.
  unlock() {
    audio();
  },

  // Three quick beeps to mark the end of a call.
  endBeep() {
    beepBurst(3, 480, 90, 55);
  },

  // Local DTMF feedback tone for a keypad press.
  dtmf(key: string) {
    const map: Record<string, [number, number]> = {
      "1": [697, 1209], "2": [697, 1336], "3": [697, 1477],
      "4": [770, 1209], "5": [770, 1336], "6": [770, 1477],
      "7": [852, 1209], "8": [852, 1336], "9": [852, 1477],
      "*": [941, 1209], "0": [941, 1336], "#": [941, 1477],
    };
    const pair = map[key];
    if (!pair) return;
    const c = audio();
    const g = c.createGain();
    g.gain.value = 0;
    g.connect(c.destination);
    const oscs = pair.map((f) => {
      const o = c.createOscillator();
      o.type = "sine";
      o.frequency.value = f;
      o.connect(g);
      o.start();
      return o;
    });
    g.gain.setTargetAtTime(0.12, c.currentTime, 0.004);
    g.gain.setTargetAtTime(0, c.currentTime + 0.11, 0.01);
    window.setTimeout(() => {
      for (const o of oscs) {
        try {
          o.stop();
          o.disconnect();
        } catch {
          // ignore
        }
      }
      g.disconnect();
    }, 200);
  },
};
