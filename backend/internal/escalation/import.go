package escalation

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// parseCatalog turns an uploaded spreadsheet into a category -> reasons map.
// Each row is [category, reason]; a leading header row (e.g. "Kategori","Durum")
// is ignored. Both .xlsx and .csv are accepted.
func parseCatalog(filename string, data []byte) (map[string][]string, error) {
	name := strings.ToLower(filename)
	var rows [][]string
	var err error
	switch {
	case strings.HasSuffix(name, ".csv"):
		rows, err = readCSV(data)
	case strings.HasSuffix(name, ".xlsx"):
		rows, err = readXLSX(data)
	default:
		// Fall back on content sniffing: xlsx is a zip (starts with "PK").
		if bytes.HasPrefix(data, []byte("PK")) {
			rows, err = readXLSX(data)
		} else {
			rows, err = readCSV(data)
		}
	}
	if err != nil {
		return nil, err
	}
	return foldRows(rows), nil
}

func foldRows(rows [][]string) map[string][]string {
	catalog := make(map[string][]string)
	seen := make(map[string]map[string]bool)
	for i, row := range rows {
		if len(row) < 2 {
			continue
		}
		category := strings.TrimSpace(row[0])
		reason := strings.TrimSpace(row[1])
		if category == "" || reason == "" {
			continue
		}
		if i == 0 && isHeader(category, reason) {
			continue
		}
		if seen[category] == nil {
			seen[category] = make(map[string]bool)
			catalog[category] = nil
		}
		if seen[category][strings.ToLower(reason)] {
			continue
		}
		seen[category][strings.ToLower(reason)] = true
		catalog[category] = append(catalog[category], reason)
	}
	return catalog
}

func isHeader(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	head := map[string]bool{"kategori": true, "category": true, "durum": true, "reason": true, "sebep": true}
	return head[a] || head[b]
}

func readCSV(data []byte) ([][]string, error) {
	// Excel exports Turkish CSV with a semicolon separator; detect it.
	sep := ','
	if bytes.Count(data, []byte(";")) > bytes.Count(data, []byte(",")) {
		sep = ';'
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r.ReadAll()
}

// --- Minimal .xlsx reader (Office Open XML) ---

type xlsxSST struct {
	Items []struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type xlsxSheet struct {
	Rows []struct {
		Cells []struct {
			Ref  string `xml:"r,attr"`
			Type string `xml:"t,attr"`
			V    string `xml:"v"`
			IS   struct {
				Text string `xml:"t"`
			} `xml:"is"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func readXLSX(data []byte) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("xlsx is not a valid archive: %w", err)
	}

	shared, err := readSharedStrings(zr)
	if err != nil {
		return nil, err
	}

	sheet, err := openFirstSheet(zr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sheet.Close() }()

	raw, err := io.ReadAll(sheet)
	if err != nil {
		return nil, fmt.Errorf("xlsx sheet could not be read: %w", err)
	}
	var parsed xlsxSheet
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("xlsx sheet could not be parsed: %w", err)
	}

	out := make([][]string, 0, len(parsed.Rows))
	for _, row := range parsed.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, c := range row.Cells {
			cells = append(cells, cellValue(c.Type, c.V, c.IS.Text, shared))
		}
		out = append(out, cells)
	}
	return out, nil
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	f, err := openInZip(zr, "xl/sharedStrings.xml")
	if err != nil {
		return nil, nil // a sheet may inline all strings
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("shared strings could not be read: %w", err)
	}
	var sst xlsxSST
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil, fmt.Errorf("shared strings could not be parsed: %w", err)
	}
	out := make([]string, 0, len(sst.Items))
	for _, si := range sst.Items {
		if si.Text != "" || len(si.Runs) == 0 {
			out = append(out, si.Text)
			continue
		}
		var b strings.Builder
		for _, r := range si.Runs {
			b.WriteString(r.Text)
		}
		out = append(out, b.String())
	}
	return out, nil
}

func openFirstSheet(zr *zip.Reader) (io.ReadCloser, error) {
	if f, err := openInZip(zr, "xl/worksheets/sheet1.xml"); err == nil {
		return f, nil
	}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "xl/worksheets/") && strings.HasSuffix(f.Name, ".xml") {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("xlsx contains no worksheet")
}

func openInZip(zr *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range zr.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("%s not found", name)
}

func cellValue(typ, v, inline string, shared []string) string {
	switch typ {
	case "s":
		idx := atoi(v)
		if idx >= 0 && idx < len(shared) {
			return shared[idx]
		}
		return ""
	case "inlineStr":
		return inline
	default:
		return v
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return -1
	}
	return n
}
