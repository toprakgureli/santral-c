package setting

import "testing"

func TestIPTrusted(t *testing.T) {
	entries, err := normalizeIPs([]string{" 85.105.10.20 ", "10.0.0.0/8", "85.105.10.20", "2a02:ff0::/32"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("duplicates should collapse, got %v", entries)
	}
	cases := map[string]bool{
		"85.105.10.20":        true,
		"::ffff:85.105.10.20": true,
		"10.4.5.6":            true,
		"11.0.0.1":            false,
		"85.105.10.21":        false,
		"2a02:ff0:1::9":       true,
		"":                    false,
		"not-an-ip":           false,
	}
	for ip, want := range cases {
		if got := IPTrusted(ip, entries); got != want {
			t.Errorf("IPTrusted(%q) = %v, want %v", ip, got, want)
		}
	}
	if _, err := normalizeIPs([]string{"300.1.1.1"}); err == nil {
		t.Error("an invalid address must be rejected")
	}
	if _, err := normalizeIPs([]string{"10.0.0.0/33"}); err == nil {
		t.Error("an invalid block must be rejected")
	}
}
