package outside

import (
	"encoding/json"
	"testing"
)

func TestFillURL(t *testing.T) {
	vars := map[string]string{"no": "12/../admin?x=1", "ad": "Ali & Veli"}
	tests := []struct {
		name, raw, want string
		wantErr         bool
	}{
		{"path and query escaped", "https://api.example.com/orders/{no}?name={ad}", "https://api.example.com/orders/12%2F..%2Fadmin%3Fx=1?name=Ali+%26+Veli", false},
		{"variable in host", "https://{no}.example.com/x", "", true},
		{"variable as whole host", "http://{no}/x", "", true},
		{"variable in scheme", "{no}://example.com", "", true},
		{"unknown variable kept", "https://api.example.com/{missing}", "https://api.example.com/{missing}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FillURL(tt.raw, vars)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FillURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("FillURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFillJSONKeepsBodyValid(t *testing.T) {
	body := FillJSON(`{"note":"{not}","phone":"{tel}"}`, map[string]string{"not": `a","admin":true,"x":"`, "tel": "555\n123"})
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("body is not valid JSON: %v (%s)", err, body)
	}
	if _, injected := out["admin"]; injected {
		t.Fatalf("a customer answer added a field: %s", body)
	}
	if out["note"] != `a","admin":true,"x":"` {
		t.Fatalf("note = %q", out["note"])
	}
}

func TestFillHeaderDropsLineBreaks(t *testing.T) {
	if got := FillHeader("Bearer {t}", map[string]string{"t": "abc\r\nX-Evil: 1"}); got != "Bearer abc  X-Evil: 1" {
		t.Fatalf("FillHeader() = %q", got)
	}
}
