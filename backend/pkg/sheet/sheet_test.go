package sheet

import (
	"bytes"
	"testing"
)

func TestCell(t *testing.T) {
	tests := map[string]string{
		"=HYPERLINK(\"http://x\")": "'=HYPERLINK(\"http://x\")",
		"+905551234567":            "'+905551234567",
		"-2":                       "'-2",
		"@SUM(A1)":                 "'@SUM(A1)",
		"\tcmd":                    "'\tcmd",
		"Çok iyi":                  "Çok iyi",
		"":                         "",
	}
	for in, want := range tests {
		if got := Cell(in); got != want {
			t.Errorf("Cell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriter(t *testing.T) {
	w := NewWriter()
	if err := w.Row("Yorum", "=1+1"); err != nil {
		t.Fatal(err)
	}
	out, err := w.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("missing UTF-8 mark")
	}
	if got := string(out[3:]); got != "Yorum;'=1+1\n" {
		t.Fatalf("row = %q", got)
	}
}
