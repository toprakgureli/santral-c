package whatsapp

import "testing"

func TestExistingPathRefusesServerPathsInAnySpelling(t *testing.T) {
	for _, bad := range []string{
		"https://ornek.com/API/v1/users",
		"https://ornek.com/Api",
		"https://ornek.com/HEALTHZ",
		"https://ornek.com/metrics",
		"https://ornek.com/Metrics/x",
	} {
		if _, err := existingPath(bad); err == nil {
			t.Errorf("existingPath(%q) should fail", bad)
		}
	}
	// Paths that only start with the same letters are a webhook's own.
	for _, ok := range []string{"https://ornek.com/apiwebhook", "https://ornek.com/healthzone/hook"} {
		if _, err := existingPath(ok); err != nil {
			t.Errorf("existingPath(%q): %v", ok, err)
		}
	}
}

func TestHookKeyIgnoresCapitalsAndSlashes(t *testing.T) {
	cases := map[string]string{
		"/Webhook/WhatsApp/": "/webhook/whatsapp",
		"webhook/whatsapp":   "/webhook/whatsapp",
		"/WEBHOOK":           "/webhook",
	}
	for in, want := range cases {
		if got := hookKey(in); got != want {
			t.Errorf("hookKey(%q) = %q, want %q", in, got, want)
		}
	}
}
