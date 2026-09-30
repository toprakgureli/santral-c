// Package sheet writes CSV files that open safely in spreadsheet programs.
package sheet

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// bom makes spreadsheet programs read the file as UTF-8, so Turkish letters
// show right.
var bom = []byte{0xEF, 0xBB, 0xBF}

// Cell makes a value safe to open in a spreadsheet. Text that starts with
// =, +, -, @, a tab or a carriage return is read as a formula, which a
// customer could use to run something on the reader's computer; such text
// gets a leading apostrophe, which spreadsheet programs hide.
func Cell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// Writer is a semicolon separated CSV file with a UTF-8 mark, whose cells
// are passed through Cell.
type Writer struct {
	buf bytes.Buffer
	w   *csv.Writer
}

// NewWriter starts an empty file.
func NewWriter() *Writer {
	s := &Writer{}
	s.buf.Write(bom)
	s.w = csv.NewWriter(&s.buf)
	s.w.Comma = ';'
	return s
}

// Row writes one line.
func (s *Writer) Row(cells ...string) error {
	safe := make([]string, len(cells))
	for i, c := range cells {
		safe[i] = Cell(c)
	}
	if err := s.w.Write(safe); err != nil {
		return fmt.Errorf("csv row could not be written: %w", err)
	}
	return nil
}

// Bytes finishes the file and returns it.
func (s *Writer) Bytes() ([]byte, error) {
	s.w.Flush()
	if err := s.w.Error(); err != nil {
		return nil, fmt.Errorf("csv could not be written: %w", err)
	}
	return s.buf.Bytes(), nil
}
