package performance

import "testing"

func TestMaskNumber(t *testing.T) {
	tests := map[string]string{
		"05551234567":    "•••••••••67",
		"+90 555 123 45": "+•• ••• ••• 45",
		"1014":           "1014",
		"":               "",
	}
	for in, want := range tests {
		if got := MaskNumber(in); got != want {
			t.Errorf("MaskNumber(%q) = %q, want %q", in, got, want)
		}
	}
}
