package telephony

import "math"

// estimateMOS approximates the Mean Opinion Score from packet loss, jitter and
// round-trip time using a simplified ITU E-model. The result is clamped to the
// 1.0-5.0 MOS range.
func estimateMOS(lossPct, jitterMs, rttMs float64) float64 {
	// One-way effective latency, approximated from RTT and jitter buffering.
	effectiveLatency := rttMs/2 + 2*jitterMs + 10

	var r float64
	if effectiveLatency < 160 {
		r = 93.2 - effectiveLatency/40
	} else {
		r = 93.2 - (effectiveLatency-120)/10
	}

	// Each percent of loss costs ~2.5 R points.
	r -= lossPct * 2.5

	if r < 0 {
		r = 0
	}
	if r > 100 {
		r = 100
	}

	mos := 1 + 0.035*r + r*(r-60)*(100-r)*7e-6
	if mos < 1 {
		mos = 1
	}
	if mos > 5 {
		mos = 5
	}
	return math.Round(mos*100) / 100
}
