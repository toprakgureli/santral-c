package teams

import (
	"encoding/json"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

func TestParseHistory(t *testing.T) {
	cases := []struct {
		in    string
		want  History
		ok    bool
		lines int
	}{
		{"", HistoryNone, true, 0},
		{"none", HistoryNone, true, 0},
		{"50", History50, true, 50},
		{"100", History100, true, 100},
		{"all", HistoryAll, true, -1},
		{"20", "", false, 0},
		{"ALL", "", false, 0},
	}
	for _, c := range cases {
		got, ok := ParseHistory(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseHistory(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
		if ok && got.lines() != c.lines {
			t.Errorf("%q shows %d lines, want %d", c.in, got.lines(), c.lines)
		}
	}
}

func TestSeesHistoryStart(t *testing.T) {
	if !sees(nil, 1) {
		t.Error("someone looking in without a seat reads everything")
	}
	all := &models.ChatMember{}
	if !sees(all, 1) {
		t.Error("a seat with no history start reads everything")
	}
	late := &models.ChatMember{HistoryFrom: 100}
	if sees(late, 99) || !sees(late, 100) || !sees(late, 101) {
		t.Error("a seat starting at 100 reads 100 and later only")
	}
}

// A line quoting an older one: the seat that may read both gets the quote,
// the newcomer gets it without text, and a seat that cannot read the line
// itself hears nothing.
func TestSendLineHidesQuotesPerSeat(t *testing.T) {
	hub := NewHub()
	s := &Service{hub: hub}
	old, _ := hub.Subscribe(1)
	newcomer, _ := hub.Subscribe(2)
	later, _ := hub.Subscribe(3)
	seats := []models.ChatMember{{UserID: 1}, {UserID: 2, HistoryFrom: 50}, {UserID: 3, HistoryFrom: 200}}
	msg := &MessageView{ID: 120, Body: "yanıt", ReplyTo: &ReplyView{ID: 10, Body: "eski sır", Sender: "Ayşe"}}
	s.sendLine(seats, 120, Event{Type: "message", GroupID: 7, Message: msg})

	read := func(ch chan []byte) *Event {
		select {
		case raw := <-ch:
			var ev Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatal(err)
			}
			return &ev
		default:
			return nil
		}
	}
	if ev := read(old); ev == nil || ev.Message.ReplyTo.Body != "eski sır" || ev.Message.ReplyTo.Hidden {
		t.Errorf("the long-time member should see the quote: %+v", ev)
	}
	if ev := read(newcomer); ev == nil || ev.Message.ReplyTo.Body != "" || ev.Message.ReplyTo.Sender != "" || !ev.Message.ReplyTo.Hidden {
		t.Errorf("the newcomer should get the quote without text: %+v", ev)
	}
	if ev := read(later); ev != nil {
		t.Errorf("a seat that may not read the line heard about it: %+v", ev)
	}
	if msg.ReplyTo.Hidden || msg.ReplyTo.Body == "" {
		t.Error("the caller's view was changed")
	}
}

// Someone who joined later does not hold up a line's read mark.
func TestStatusSkipsSeatsThatCannotReadTheLine(t *testing.T) {
	s := &Service{}
	sender := uint(1)
	msg := &models.ChatMessage{ID: 40, SenderID: &sender, Kind: "text"}
	seats := []models.ChatMember{
		{UserID: 1, LastReadID: 40},
		{UserID: 2, LastReadID: 40, LastDeliveredID: 40},
		{UserID: 3, HistoryFrom: 60, LastReadID: 59, LastDeliveredID: 59},
	}
	people := map[uint]Person{2: {ID: 2, Name: "Ali"}, 3: {ID: 3, Name: "Veli"}}
	status, readBy := s.status(1, msg, seats, people)
	if status != "read" || len(readBy) != 1 || readBy[0] != "Ali" {
		t.Errorf("status = %s %v, want read by Ali only", status, readBy)
	}
}
