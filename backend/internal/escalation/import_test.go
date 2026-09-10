package escalation

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestParseCatalogHeaderSkip(t *testing.T) {
	data := []byte("Kategori,Durum\nTalep,Yeni ozellik\n")
	cat, err := parseCatalog("x.csv", data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat["Kategori"]; ok {
		t.Errorf("header row was not skipped: %v", cat)
	}
	if len(cat["Talep"]) != 1 {
		t.Errorf("Talep reasons = %v, want 1", cat["Talep"])
	}
}

func TestParseCatalogCSV(t *testing.T) {
	data := []byte("Kategori;Durum\nMemnuniyet;Teknik birimle ilgili memnuniyetsizlik\nMemnuniyet;Yazilim ekibiyle ilgili memnuniyetsizlik\nTalep;Yeni ozellik talebi\n")
	cat, err := parseCatalog("durumlar.csv", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat["Memnuniyet"]) != 2 {
		t.Errorf("Memnuniyet reasons = %v, want 2", cat["Memnuniyet"])
	}
	if len(cat["Talep"]) != 1 {
		t.Errorf("Talep reasons = %v, want 1", cat["Talep"])
	}
}

func TestParseCatalogXLSX(t *testing.T) {
	data := buildXLSX(t, [][2]string{
		{"Kategori", "Durum"},
		{"Memnuniyet", "Teknik birim memnuniyetsizligi"},
		{"Memnuniyet", "Yazilim ekibi memnuniyetsizligi"},
	})
	cat, err := parseCatalog("durumlar.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat["Memnuniyet"]) != 2 {
		t.Errorf("Memnuniyet reasons = %v, want 2", cat["Memnuniyet"])
	}
}

// buildXLSX writes a minimal valid xlsx with inline strings.
func buildXLSX(t *testing.T, rows [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`)
	var sb bytes.Buffer
	sb.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, r := range rows {
		sb.WriteString(`<row r="` + itoa(i+1) + `">`)
		sb.WriteString(`<c t="inlineStr"><is><t>` + r[0] + `</t></is></c>`)
		sb.WriteString(`<c t="inlineStr"><is><t>` + r[1] + `</t></is></c>`)
		sb.WriteString(`</row>`)
	}
	sb.WriteString(`</sheetData></worksheet>`)
	write("xl/worksheets/sheet1.xml", sb.String())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
