package whatsapp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/flow"
)

func TestInboundShape(t *testing.T) {
	raw := `{"from":"905551112233","id":"wamid.X","timestamp":"1727000000","type":"interactive",
		"interactive":{"type":"button_reply","button_reply":{"id":"opt:o1","title":"Destek"}}}`
	var m hookMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	kind, body, media, payload := inboundShape(&m)
	if kind != "interactive" || body != "Destek" || media != nil || payload.(map[string]string)["id"] != "opt:o1" {
		t.Fatalf("button reply shape wrong: %s %s %+v", kind, body, payload)
	}
	raw = `{"from":"905551112233","id":"wamid.Y","type":"document","document":{"id":"M1","mime_type":"application/pdf","filename":"fatura.pdf","caption":"ekte"}}`
	m = hookMessage{}
	_ = json.Unmarshal([]byte(raw), &m)
	kind, body, media, _ = inboundShape(&m)
	if kind != "document" || body != "ekte" || media == nil || media.MetaID != "M1" || media.Name != "fatura.pdf" {
		t.Fatalf("document shape wrong: %s %s %+v", kind, body, media)
	}
	m = hookMessage{Type: "order"}
	if kind, _, _, _ := inboundShape(&m); kind != "unsupported" {
		t.Fatalf("unknown types must be kept as unsupported, got %s", kind)
	}
}

func TestBuildTemplate(t *testing.T) {
	tpl := &models.WATemplate{Name: "siparis_hazir", Language: "tr", Components: `[
		{"type":"HEADER","format":"TEXT","text":"Sipariş {{1}}"},
		{"type":"BODY","text":"Merhaba {{1}}, siparişiniz {{2}} hazır."},
		{"type":"FOOTER","text":"İyi günler"},
		{"type":"BUTTONS","buttons":[{"type":"URL","text":"Takip","url":"https://x.com/{{1}}"},{"type":"QUICK_REPLY","text":"Teşekkürler"}]}]`}
	obj, preview, err := buildTemplate(tpl, TemplateParams{Header: []string{"#12"}, Body: []string{"Ayşe", "bugün"}, Buttons: []string{"abc"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview, "Merhaba Ayşe, siparişiniz bugün hazır.") || !strings.Contains(preview, "*Sipariş #12*") {
		t.Fatalf("preview wrong: %s", preview)
	}
	comps := obj["components"].([]map[string]any)
	if len(comps) != 3 {
		t.Fatalf("header, body and the url button need parameters, got %d", len(comps))
	}
	if comps[2]["index"] != "0" || comps[2]["sub_type"] != "url" {
		t.Fatalf("url button parameter wrong: %+v", comps[2])
	}
	if _, _, err := buildTemplate(tpl, TemplateParams{Header: []string{"#1"}, Body: []string{"Ayşe"}}); err == nil {
		t.Fatal("a missing body variable must be refused")
	}
}

func TestMenuMessageShapes(t *testing.T) {
	opts := []flow.Option{{ID: "a", Label: "Bir"}, {ID: "b", Label: "İki"}}
	m := flow.MenuMessage("buttons", "Seç", "", opts)
	in := m["interactive"].(map[string]any)
	if in["type"] != "button" {
		t.Fatalf("two options fit buttons: %+v", in)
	}
	opts = append(opts, flow.Option{ID: "c", Label: "Üç"}, flow.Option{ID: "d", Label: "Dört"})
	m = flow.MenuMessage("buttons", "Seç", "", opts)
	if m["interactive"].(map[string]any)["type"] != "list" {
		t.Fatal("more than three options must become a list")
	}
}

func TestMatchesWord(t *testing.T) {
	if !matchesWord("  dur ", []string{"DUR"}) || matchesWord("durum nedir", []string{"DUR"}) {
		t.Fatal("opt-out words must match the whole message only")
	}
}

func TestSnippetTurkishCase(t *testing.T) {
	body := strings.Repeat("İ", 60) + " sipariş numaram 12345 " + strings.Repeat("ı", 80)
	got := snippet(body, "SİPARİŞ")
	if !strings.Contains(got, "sipariş numaram") {
		t.Errorf("snippet lost the word: %q", got)
	}
	// Letters that change length when lowered never push past the end.
	_ = snippet(strings.Repeat("Ⱥ", 200)+"x", "x")
}

func TestRuneIndexFold(t *testing.T) {
	if i := runeIndexFold([]rune("Merhaba DÜNYA"), []rune("dünya")); i != 8 {
		t.Errorf("index = %d, want 8", i)
	}
	if i := runeIndexFold([]rune("abc"), []rune("x")); i != -1 {
		t.Errorf("index = %d, want -1", i)
	}
}
