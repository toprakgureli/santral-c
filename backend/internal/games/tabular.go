package games

// Spreadsheet reading for content import: .xlsx (Office Open XML, first
// sheet) and .csv (comma or the Turkish Excel semicolon). The same minimal
// reader the escalation catalogue import uses; kept local so the two
// packages do not depend on each other.

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// readRows returns the cells of an uploaded spreadsheet.
func readRows(filename string, data []byte) ([][]string, error) {
	name := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(name, ".xlsx"), bytes.HasPrefix(data, []byte("PK")):
		return readXLSX(data)
	default:
		return readCSV(data)
	}
}

func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
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
		return nil, nil
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
		idx := 0
		for _, r := range v {
			if r < '0' || r > '9' {
				return ""
			}
			idx = idx*10 + int(r-'0')
		}
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
