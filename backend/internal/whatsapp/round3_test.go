package whatsapp

import (
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/device"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/flow"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// The return time is counted from the chat being resolved to the
// customer's message, not to when its follow-up work happens to run.
func TestWithinReturn(t *testing.T) {
	resolved := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		minutes int
		at      time.Time
		want    bool
	}{
		{30, resolved.Add(10 * time.Minute), true},
		{30, resolved.Add(30 * time.Minute), true},
		{30, resolved.Add(31 * time.Minute), false},
		// WhatsApp's time has whole seconds; a message just around the close
		// still counts.
		{30, resolved.Add(-500 * time.Millisecond), true},
		{0, resolved.Add(time.Minute), false},
	} {
		if got := withinReturn(c.minutes, &resolved, c.at); got != c.want {
			t.Errorf("withinReturn(%d, +%v) = %v, want %v", c.minutes, c.at.Sub(resolved), got, c.want)
		}
	}
	if withinReturn(30, nil, resolved) {
		t.Error("a chat never resolved cannot be returned to")
	}
}

func TestReturnsToAgentUsesTheMessageTime(t *testing.T) {
	resolved := time.Now().Add(-3 * time.Hour)
	written := resolved.Add(10 * time.Minute)
	owner := uint(7)
	res := &inboundResult{reopened: true, resolvedAt: &resolved, ticket: &models.WATicket{OwnerID: &owner},
		msg: &models.WAMessage{WATimestamp: &written, CreatedAt: time.Now()}}
	set := device.Settings{ReturnMinutes: 30}
	s := &Service{}
	if !s.returnsToAgent(set, res) {
		t.Fatal("a message written 10 minutes after the close must go back to its agent, however late its work runs")
	}
	res.ticket.OwnerID = nil
	if s.returnsToAgent(set, res) {
		t.Fatal("a chat whose agent was taken off must be handed out afresh")
	}
	res.ticket.OwnerID, res.reopened = &owner, false
	if s.returnsToAgent(set, res) {
		t.Fatal("only a reopened chat returns to its agent")
	}
}

// A publish-only person tests a flow without reaching outside systems.
func TestSimulatorWithoutCaller(t *testing.T) {
	g := &flow.Graph{
		Nodes: []flow.Node{
			{ID: "s", Type: "start"},
			{ID: "a", Type: "api", Data: flow.Data{Integration: 3}},
			{ID: "ok", Type: "message", Data: flow.Data{Text: "devam"}},
			{ID: "no", Type: "message", Data: flow.Data{Text: "hata"}},
		},
		Edges: []flow.Edge{{From: "s", To: "a"}, {From: "a", Port: "ok", To: "ok"}, {From: "a", Port: "fail", To: "no"}},
	}
	io := flow.NewSimIO(true, time.Now(), nil)
	flow.Step(g, &flow.State{Vars: map[string]string{}}, nil, io)
	if len(io.Out) < 2 || io.Out[0].Kind != "api" || io.Out[1].Text != "devam" {
		t.Fatalf("outputs = %+v, want the step shown as not called and the flow going on", io.Out)
	}
}

func TestViewerDevices(t *testing.T) {
	all := &viewer{user: &models.User{Roles: []models.Role{{Permissions: []models.Permission{{Key: string(enums.WAView)}, {Key: string(enums.WAViewAll)}}}}}}
	if d := all.devices(); !d.All {
		t.Errorf("someone who sees every device got %+v", d)
	}
	some := &viewer{user: &models.User{Roles: []models.Role{{Permissions: []models.Permission{{Key: string(enums.WAView)}}}}}, channels: map[uint]bool{4: true}}
	if d := some.devices(); d.All || len(d.IDs) != 1 || d.IDs[0] != 4 {
		t.Errorf("someone placed on device 4 got %+v", d)
	}
	none := &viewer{user: &models.User{}}
	if d := none.devices(); d.All || len(d.IDs) != 0 {
		t.Errorf("someone without WhatsApp got %+v", d)
	}
}
