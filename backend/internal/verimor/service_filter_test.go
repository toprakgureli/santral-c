package verimor

import (
	"context"
	"strings"
	"testing"
	"time"
)

func sampleScan() []CDR {
	return []CDR{
		{CallUUID: "a", Direction: "outbound", CallerIDNumber: "1014 (902129510292)", DestinationNumber: "05304230113", Duration: "0:02", AnswerStamp: "x"},
		{CallUUID: "b", Direction: "outbound", CallerIDNumber: "1015 (902129510292)", DestinationNumber: "05367441605", Duration: "0:18", AnswerStamp: "x"},
		{CallUUID: "c", Direction: "inbound", CallerIDNumber: "05325219502", DestinationNumber: "902129092554", Duration: "1:59", AnswerStamp: "x"},
		{CallUUID: "d", Direction: "outbound", CallerIDNumber: "1008 (902128526465)", DestinationNumber: "05309752651", Duration: "3:03", AnswerStamp: "x"},
		{CallUUID: "e", Direction: "outbound", CallerIDNumber: "1014 (902129510292)", DestinationNumber: "05333635081", Duration: "1:30", AnswerStamp: "x"},
		{CallUUID: "f", Direction: "internal", CallerIDNumber: "1021 (902127060510)", DestinationNumber: "1014", Duration: "0:30", AnswerStamp: "x"},
	}
}

func TestExtIsParty(t *testing.T) {
	cases := []struct {
		caller, dest, ext string
		want              bool
	}{
		{"1014 (902129510292)", "05304230113", "1014", true},
		{"1015 (902129510292)", "05367441605", "1014", false},
		{"05325219502", "902129092554", "1014", false},
		{"1021 (902127060510)", "1014", "1014", true}, // internal call to 1014
		{"1008 (902128526465)", "05309752651", "1014", false},
	}
	for _, c := range cases {
		got := extIsParty(CDR{CallerIDNumber: c.caller, DestinationNumber: c.dest}, c.ext)
		if got != c.want {
			t.Errorf("extIsParty(caller=%q dest=%q, %q) = %v, want %v", c.caller, c.dest, c.ext, got, c.want)
		}
	}
}

func TestExtScopedCallsFiltersToExtension(t *testing.T) {
	s := &Service{scan: sampleScan(), scanAt: time.Now()}
	res, err := s.extScopedCalls(context.Background(), "1014", "", Filter{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("extScopedCalls: %v", err)
	}
	if res.Total != 3 { // uuids a, e, f involve 1014
		t.Fatalf("expected 3 matches for 1014, got %d (%d items)", res.Total, len(res.Items))
	}
	for _, it := range res.Items {
		if !strings.HasPrefix(it.FromNumber, "1014") && it.ToNumber != "1014" && !strings.HasPrefix(it.ToNumber, "1014") {
			t.Errorf("returned a call that is not 1014's: from=%q to=%q", it.FromNumber, it.ToNumber)
		}
	}
}

func TestExtScopedCallsDirectionFilter(t *testing.T) {
	s := &Service{scan: sampleScan(), scanAt: time.Now()}
	// outbound only -> a, e (not f which is internal, not c which is inbound)
	res, err := s.extScopedCalls(context.Background(), "1014", "", Filter{Page: 1, Limit: 20, Direction: "outbound"})
	if err != nil {
		t.Fatalf("extScopedCalls: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("expected 2 outbound matches, got %d", res.Total)
	}
	for _, it := range res.Items {
		if it.Direction != "outbound" {
			t.Errorf("direction filter leaked %q", it.Direction)
		}
	}
}

func TestExtScopedCallsPaging(t *testing.T) {
	// 25 outbound calls for 1014 -> 2 pages at limit 20.
	scan := make([]CDR, 0, 25)
	for i := 0; i < 25; i++ {
		scan = append(scan, CDR{Direction: "outbound", CallerIDNumber: "1014 (902129510292)", DestinationNumber: "0530000000", AnswerStamp: "x"})
	}
	s := &Service{scan: scan, scanAt: time.Now()}
	res, err := s.extScopedCalls(context.Background(), "1014", "", Filter{Page: 2, Limit: 20})
	if err != nil {
		t.Fatalf("extScopedCalls: %v", err)
	}
	if res.Total != 25 || res.TotalPages != 2 || res.Page != 2 || len(res.Items) != 5 {
		t.Fatalf("paging wrong: total=%d pages=%d page=%d items=%d (want 25/2/2/5)", res.Total, res.TotalPages, res.Page, len(res.Items))
	}
}
