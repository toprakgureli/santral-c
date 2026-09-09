package telephony

import "testing"

func TestEstimateMOS(t *testing.T) {
	perfect := estimateMOS(0, 0, 0)
	if perfect < 4.3 || perfect > 4.5 {
		t.Fatalf("clean call MOS = %.2f, want ~4.4", perfect)
	}

	poor := estimateMOS(10, 60, 400)
	if poor >= perfect {
		t.Fatalf("degraded MOS %.2f should be below clean MOS %.2f", poor, perfect)
	}

	terrible := estimateMOS(50, 200, 1000)
	if terrible < 1 || terrible > 2 {
		t.Fatalf("terrible MOS = %.2f, want between 1 and 2", terrible)
	}
}

func TestEstimateMOSClamped(t *testing.T) {
	got := estimateMOS(100, 500, 2000)
	if got < 1 {
		t.Fatalf("MOS %.2f below floor", got)
	}
}
