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

func TestSurveyAnswersHelpers(t *testing.T) {
	answers := []RatingAnswer{{Question: "Hız", Score: 5}, {Question: "İlgi", Score: 4}, {Question: "Çözüm", Score: 2}}
	if got := overallScore(answers); got != 4 {
		t.Errorf("overall = %d, want 4", got)
	}
	if got := lowestScore(4, answers); got != 2 {
		t.Errorf("lowest = %d, want 2", got)
	}
	if got := answersText(answers); got != " (Hız 5, İlgi 4, Çözüm 2)" {
		t.Errorf("text = %q", got)
	}
	if got := joinTexts([]RatingText{{Question: "Öneriniz", Text: "Daha hızlı olun"}, {Question: "Not", Text: "Teşekkürler"}}); got != "Öneriniz: Daha hızlı olun\nNot: Teşekkürler" {
		t.Errorf("texts = %q", got)
	}
	if got := questionTitle("  ", 2); got != "Soru 2" {
		t.Errorf("title = %q", got)
	}
}

func TestParseTallyThreeQuestions(t *testing.T) {
	body := []byte(`{"eventType":"FORM_RESPONSE","data":{"fields":[
		{"key":"q1","label":"ticket","type":"HIDDEN_FIELDS","value":"42"},
		{"key":"q2","label":"token","type":"HIDDEN_FIELDS","value":"abc"},
		{"key":"q3","label":"Hızımızdan memnun musunuz?","type":"RATING","value":5},
		{"key":"q4","label":"İlgimizden memnun musunuz?","type":"LINEAR_SCALE","value":3},
		{"key":"q5","label":"Sorununuz çözüldü mü?","type":"MULTIPLE_CHOICE","value":["o2"],"options":[{"id":"o1","text":"5 - Çok iyi"},{"id":"o2","text":"Kötü"}]},
		{"key":"q6","label":"Eklemek istedikleriniz","type":"TEXTAREA","value":"Daha hızlı olun"}
	]}}`)
	f, err := parseTally(body)
	if err != nil {
		t.Fatal(err)
	}
	if f.ticket != "42" || f.token != "abc" {
		t.Errorf("hidden fields: %+v", f)
	}
	want := []RatingAnswer{{"Hızımızdan memnun musunuz?", 5}, {"İlgimizden memnun musunuz?", 3}, {"Sorununuz çözüldü mü?", 2}}
	if len(f.answers) != len(want) {
		t.Fatalf("answers = %+v", f.answers)
	}
	for i := range want {
		if f.answers[i] != want[i] {
			t.Errorf("answer %d = %+v, want %+v", i, f.answers[i], want[i])
		}
	}
	if joinTexts(f.texts) != "Daha hızlı olun" {
		t.Errorf("texts = %+v", f.texts)
	}
	if scoreFromText("10") != 0 || scoreFromText("4 - İyi") != 4 || scoreFromText("Çok iyi") != 5 {
		t.Error("scoreFromText")
	}
}
