package verimor

import (
	"testing"
	"time"
)

func TestExtIsParty(t *testing.T) {
	cases := []struct {
		caller, dest, ext string
		want              bool
	}{
		{"1014 (902125550100)", "05551234567", "1014", true},  // outbound: ext is caller prefix
		{"1015 (902125550100)", "05551234568", "1014", false}, // another ext's outbound
		{"05551234569", "902125550102", "1014", false},        // inbound to a bare DID (not this ext)
		{"05551234570", "902125550100 (1008)", "1008", true},  // inbound answered by 1008
		{"05551234570", "902125550100 (1008)", "1014", false}, // inbound to 1008, not 1014
		{"1021 (902125550101)", "1014", "1014", true},         // internal call to 1014
		{"1008 (902125550103)", "05551234571", "1014", false},
	}
	for _, c := range cases {
		got := extIsParty(CDR{CallerIDNumber: c.caller, DestinationNumber: c.dest}, c.ext)
		if got != c.want {
			t.Errorf("extIsParty(caller=%q dest=%q, %q) = %v, want %v", c.caller, c.dest, c.ext, got, c.want)
		}
	}
}

func TestParseParty(t *testing.T) {
	cases := []struct {
		field    string
		ext, num string
	}{
		{"1014 (902125550100)", "1014", "902125550100"},
		{"902125550101 (1014)", "1014", "902125550101"},
		{"05551234567", "", "05551234567"},
		{"1014", "1014", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		got := parseParty(c.field)
		if got.Ext != c.ext || got.Num != c.num {
			t.Errorf("parseParty(%q) = {%q %q}, want {%q %q}", c.field, got.Ext, got.Num, c.ext, c.num)
		}
	}
}

func TestPhoneQuery(t *testing.T) {
	cases := map[string]string{
		"5551234567":       "5551234567",
		"05551234567":      "5551234567",
		"905551234567":     "5551234567",
		"+90 555 123 4567": "5551234567",
		"530423":           "530423",
		"0212 555 01 02":   "2125550102",
		"":                 "",
	}
	for in, want := range cases {
		if got := phoneQuery(in); got != want {
			t.Errorf("phoneQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCDRRowDerivesSearchColumns(t *testing.T) {
	c := CDR{CallUUID: "u1", Direction: "Gelen", CallerIDNumber: "05551234567", DestinationNumber: "902125550101 (1014)", StartStamp: "2026-09-12 20:55:41 +0300", RecordingPresent: true}
	row, ok := cdrRow(c, time.Now())
	if !ok {
		t.Fatal("row should be built")
	}
	if row.Direction != "inbound" || row.CallerNum != "05551234567" || row.DestNum != "902125550101" || row.DestExt != "1014" || row.CallerExt != "" {
		t.Fatalf("unexpected row: %+v", row)
	}
	want := time.Date(2026, 9, 12, 17, 55, 41, 0, time.UTC)
	if !row.StartAt.Equal(want) {
		t.Fatalf("start_at = %s, want %s", row.StartAt, want)
	}
	if back := mapCDR(rowCDR(row)); back.Direction != "inbound" || !back.Recording || back.StartedAt != c.StartStamp {
		t.Fatalf("round trip lost data: %+v", back)
	}
	if _, ok := cdrRow(CDR{CallUUID: "u2", StartStamp: "garbage"}, time.Now()); ok {
		t.Fatal("a row with an unreadable stamp must be skipped")
	}
}

func TestDayBounds(t *testing.T) {
	from, to, err := dayBounds("2026-09-12")
	if err != nil {
		t.Fatal(err)
	}
	if !from.Equal(time.Date(2026, 9, 11, 21, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 9, 12, 21, 0, 0, 0, time.UTC)) {
		t.Fatalf("bounds = %s .. %s", from, to)
	}
}
