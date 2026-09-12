package verimor

import (
	"strings"
	"testing"
)

func sampleScan() []CDR {
	return []CDR{
		{CallUUID: "a", Direction: "outbound", CallerIDNumber: "1014 (902129510292)", DestinationNumber: "05304230113", StartStamp: "2026-09-11 17:44:10 +0300", AnswerStamp: "x"},
		{CallUUID: "b", Direction: "outbound", CallerIDNumber: "1015 (902129510292)", DestinationNumber: "05367441605", StartStamp: "2026-09-11 17:40:00 +0300", AnswerStamp: "x"},
		{CallUUID: "c", Direction: "inbound", CallerIDNumber: "05325219502", DestinationNumber: "902129092554", StartStamp: "2026-09-11 17:39:30 +0300", AnswerStamp: "x"},
		{CallUUID: "d", Direction: "outbound", CallerIDNumber: "1008 (902128526465)", DestinationNumber: "05309752651", StartStamp: "2026-09-10 09:00:00 +0300", AnswerStamp: "x"},
		{CallUUID: "e", Direction: "outbound", CallerIDNumber: "1014 (902129510292)", DestinationNumber: "05333635081", StartStamp: "2026-09-10 08:30:00 +0300", AnswerStamp: "x"},
		{CallUUID: "f", Direction: "internal", CallerIDNumber: "1021 (902127060510)", DestinationNumber: "1014", StartStamp: "2026-09-11 10:00:00 +0300", AnswerStamp: "x"},
	}
}

func TestExtIsParty(t *testing.T) {
	cases := []struct {
		caller, dest, ext string
		want              bool
	}{
		{"1014 (902129510292)", "05304230113", "1014", true},  // outbound: ext is caller prefix
		{"1015 (902129510292)", "05367441605", "1014", false}, // another ext's outbound
		{"05325219502", "902129092554", "1014", false},        // inbound to a bare DID (not this ext)
		{"05357352889", "902129510292 (1008)", "1008", true},  // inbound answered by 1008
		{"05357352889", "902129510292 (1008)", "1014", false}, // inbound to 1008, not 1014
		{"1021 (902127060510)", "1014", "1014", true},         // internal call to 1014
		{"1008 (902128526465)", "05309752651", "1014", false},
	}
	for _, c := range cases {
		got := extIsParty(CDR{CallerIDNumber: c.caller, DestinationNumber: c.dest}, c.ext)
		if got != c.want {
			t.Errorf("extIsParty(caller=%q dest=%q, %q) = %v, want %v", c.caller, c.dest, c.ext, got, c.want)
		}
	}
}

func TestWindowCallsFiltersToExtension(t *testing.T) {
	s := &Service{scan: sampleScan()}
	res := s.windowCalls("1014", "", "", "", Filter{Page: 1, Limit: 20})
	if res.Total != 3 { // a, e, f involve 1014
		t.Fatalf("expected 3 matches for 1014, got %d", res.Total)
	}
	for _, it := range res.Items {
		if !strings.HasPrefix(it.FromNumber, "1014") && it.ToNumber != "1014" && !strings.HasPrefix(it.ToNumber, "1014") {
			t.Errorf("returned a call that is not 1014's: from=%q to=%q", it.FromNumber, it.ToNumber)
		}
	}
}

func TestWindowCallsDirectionFilter(t *testing.T) {
	s := &Service{scan: sampleScan()}
	// outbound only for 1014 -> a, e (not f internal, not c inbound)
	res := s.windowCalls("1014", "", "", "", Filter{Page: 1, Limit: 20, Direction: "outbound"})
	if res.Total != 2 {
		t.Fatalf("expected 2 outbound matches, got %d", res.Total)
	}
	for _, it := range res.Items {
		if it.Direction != "outbound" {
			t.Errorf("direction filter leaked %q", it.Direction)
		}
	}
}

func TestWindowCallsDateRange(t *testing.T) {
	s := &Service{scan: sampleScan()}
	// 1014's calls only on 2026-09-11 -> a and f (e is on 09-10)
	res := s.windowCalls("1014", "", "2026-09-11", "2026-09-11", Filter{Page: 1, Limit: 20})
	if res.Total != 2 {
		t.Fatalf("expected 2 matches for 1014 on 2026-09-11, got %d", res.Total)
	}
	// All calls (no ext) on 2026-09-10 -> d, e
	all := s.windowCalls("", "", "2026-09-10", "2026-09-10", Filter{Page: 1, Limit: 20})
	if all.Total != 2 {
		t.Fatalf("expected 2 calls on 2026-09-10, got %d", all.Total)
	}
}

func TestWindowCallsPaging(t *testing.T) {
	scan := make([]CDR, 0, 25)
	for i := 0; i < 25; i++ {
		scan = append(scan, CDR{CallUUID: string(rune('a' + i)), Direction: "outbound", CallerIDNumber: "1014 (902129510292)", DestinationNumber: "0530000000", StartStamp: "2026-09-11 10:00:00 +0300", AnswerStamp: "x"})
	}
	s := &Service{scan: scan}
	res := s.windowCalls("1014", "", "", "", Filter{Page: 2, Limit: 20})
	if res.Total != 25 || res.TotalPages != 2 || res.Page != 2 || len(res.Items) != 5 {
		t.Fatalf("paging wrong: total=%d pages=%d page=%d items=%d (want 25/2/2/5)", res.Total, res.TotalPages, res.Page, len(res.Items))
	}
}
