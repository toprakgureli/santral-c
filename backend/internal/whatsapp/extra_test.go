package whatsapp

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

func TestMobileWAID(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"0530 123 45 67":    {"905301234567", true},
		"+90 530 123 4567":  {"905301234567", true},
		"0212 555 44 33":    {"", false}, // landline
		"1001":              {"", false}, // extension
		"+49 151 2345 6789": {"4915123456789", true},
	}
	for in, c := range cases {
		got, ok := mobileWAID(in)
		if ok != c.ok || got != c.want {
			t.Errorf("mobileWAID(%q) = %q, %v; want %q, %v", in, got, ok, c.want, c.ok)
		}
	}
}

func TestBlockedIP(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.10", "172.16.5.4", "169.254.169.254", "100.64.0.1", "::1", "fd00::1", "0.0.0.0"} {
		if !blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s should be blocked", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700::1111"} {
		if blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}
}

func TestCallSurveyAnswer(t *testing.T) {
	m := &hookMessage{Type: "button", Button: &hookButton{Payload: "csv:42:1", Text: "İyi"}}
	id, idx, ok := callSurveyAnswer(m)
	if !ok || id != 42 || idx != 1 {
		t.Fatalf("got %d %d %v", id, idx, ok)
	}
	for _, p := range []string{"csv:x:1", "rate-1-5", "csv:1", ""} {
		if _, _, ok := callSurveyAnswer(&hookMessage{Type: "button", Button: &hookButton{Payload: p}}); ok {
			t.Errorf("payload %q should not match", p)
		}
	}
	if _, _, ok := callSurveyAnswer(&hookMessage{Type: "text"}); ok {
		t.Error("text should not match")
	}
}

func TestBuildTemplateQuickAndFile(t *testing.T) {
	comps := `[{"type":"HEADER","format":"IMAGE"},{"type":"BODY","text":"Merhaba {{1}}"},{"type":"BUTTONS","buttons":[{"type":"QUICK_REPLY","text":"İyi"},{"type":"QUICK_REPLY","text":"Kötü"}]}]`
	tpl := &models.WATemplate{Name: "anket", Language: "tr", Components: comps}
	obj, _, err := buildTemplate(tpl, TemplateParams{Body: []string{"Ayşe"}, headerMediaID: "M1", quickPayloads: []string{"csv:1:0", "csv:1:1"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(obj)
	s := string(raw)
	for _, want := range []string{`"id":"M1"`, `"payload":"csv:1:0"`, `"payload":"csv:1:1"`, `"sub_type":"quick_reply"`} {
		if !strings.Contains(s, want) {
			t.Errorf("template object misses %s: %s", want, s)
		}
	}
	if _, _, err := buildTemplate(tpl, TemplateParams{Body: []string{"Ayşe"}}); err == nil {
		t.Error("a media header with no file should fail")
	}
}
