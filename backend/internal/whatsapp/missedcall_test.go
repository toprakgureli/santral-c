package whatsapp

import (
	"encoding/json"
	"testing"
)

func TestDefaultMissedCallRecovery(t *testing.T) {
	d := defaultMissedCallRecovery()
	if d.Enabled {
		t.Error("expected default to be disabled")
	}
	if d.Mode != "auto" {
		t.Errorf("expected default mode 'auto', got %s", d.Mode)
	}
	if d.QuietHours != 4 {
		t.Errorf("expected quietHours 4, got %d", d.QuietHours)
	}
	if d.TemplateLang != "tr" {
		t.Errorf("expected templateLang tr, got %s", d.TemplateLang)
	}
}

func TestMissedCallRecoverySettingsJSON(t *testing.T) {
	set := MissedCallRecoverySettings{
		Enabled:      true,
		Mode:         "manual",
		ChannelID:    1,
		Template:     "kacan_cagri_test",
		TemplateLang: "tr",
		Params:       []string{"{musteri}", "{saat}"},
		QuietHours:   6,
	}
	data, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var unmarshaled MissedCallRecoverySettings
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !unmarshaled.Enabled || unmarshaled.Mode != "manual" || unmarshaled.Template != "kacan_cagri_test" || unmarshaled.QuietHours != 6 {
		t.Errorf("unmarshaled mismatch: %+v", unmarshaled)
	}
	if len(unmarshaled.Params) != 2 || unmarshaled.Params[0] != "{musteri}" {
		t.Errorf("params mismatch: %+v", unmarshaled.Params)
	}
}

func TestMobileWAIDFilteringForMissedCalls(t *testing.T) {
	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{"05321234567", "905321234567", true},
		{"+905321234567", "905321234567", true},
		{"905321234567", "905321234567", true},
		{"02123456789", "", false},   // Turkish landline should be filtered out
		{"+902163456789", "", false}, // Turkish landline should be filtered out
		{"1014", "", false},          // internal extension should be filtered out
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := mobileWAID(c.input)
		if ok != c.ok || got != c.want {
			t.Errorf("mobileWAID(%q) = (%q, %v), want (%q, %v)", c.input, got, ok, c.want, c.ok)
		}
	}
}
