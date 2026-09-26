package whatsapp

import (
	"strings"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

func TestTemplatePreviewButtons(t *testing.T) {
	tpl := &models.WATemplate{Name: "ulasilamadi", Language: "tr", Components: `[
		{"type":"BODY","text":"Merhaba {{1}}, size ulaşamadık."},
		{"type":"BUTTONS","buttons":[
			{"type":"PHONE_NUMBER","text":"İletişim","phone_number":"+908501234567"},
			{"type":"URL","text":"Talebim","url":"https://ornek.com/t/{{1}}"},
			{"type":"QUICK_REPLY","text":"Beni arayın"}
		]}]`}
	_, text, err := buildTemplate(tpl, TemplateParams{Body: []string{"Ayşe"}, Buttons: []string{"42"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Merhaba Ayşe, size ulaşamadık.", "[İletişim](tel:+908501234567)", "[Talebim](https://ornek.com/t/42)", "[Beni arayın]"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
	m := &models.WAMessage{Kind: "template", Body: text}
	if got := preview(m); strings.Contains(got, "[") || got != "Merhaba Ayşe, size ulaşamadık." {
		t.Errorf("list preview = %q", got)
	}
}

func TestMenuPreviewIsTheQuestion(t *testing.T) {
	m := &models.WAMessage{Kind: "interactive", Direction: "out", Body: "Lütfen bir konu seçin.\n1. Teknik Destek\n2. Satış Ekibi"}
	if got := preview(m); got != "Lütfen bir konu seçin." {
		t.Errorf("got %q", got)
	}
}
