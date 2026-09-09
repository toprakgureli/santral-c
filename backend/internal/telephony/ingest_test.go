package telephony

import (
	"context"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/ami"
)

type fakeRepo struct {
	created []*models.Call
	events  []*models.CallEvent
	quality []*models.CallQuality
	updates map[uint][]map[string]any
	nextID  uint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{updates: make(map[uint][]map[string]any)}
}

func (f *fakeRepo) CreateCall(_ context.Context, c *models.Call) error {
	f.nextID++
	c.ID = f.nextID
	f.created = append(f.created, c)
	return nil
}
func (f *fakeRepo) UpdateCall(_ context.Context, id uint, fields map[string]any) error {
	f.updates[id] = append(f.updates[id], fields)
	return nil
}
func (f *fakeRepo) AddEvent(_ context.Context, e *models.CallEvent) error {
	f.events = append(f.events, e)
	return nil
}
func (f *fakeRepo) AddQuality(_ context.Context, q *models.CallQuality) error {
	f.quality = append(f.quality, q)
	return nil
}
func (f *fakeRepo) UserIDByExtension(context.Context, string) (*uint, error) { return nil, nil }
func (f *fakeRepo) ContactIDByNumber(context.Context, string) (*uint, error) { return nil, nil }

func (f *fakeRepo) lastUpdate(id uint) map[string]any {
	u := f.updates[id]
	if len(u) == 0 {
		return nil
	}
	merged := map[string]any{}
	for _, m := range u {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}

func TestIngestInternalAnsweredCall(t *testing.T) {
	repo := newFakeRepo()
	ing := NewIngester(repo)
	ctx := context.Background()

	seq := []ami.Event{
		{"Event": "Newchannel", "Linkedid": "L1", "Uniqueid": "L1", "CallerIDNum": "1001", "Exten": "1002", "Channel": "PJSIP/1001-00000001"},
		{"Event": "Newchannel", "Linkedid": "L1", "Uniqueid": "U2", "Channel": "PJSIP/1002-00000002"},
		{"Event": "DialBegin", "Linkedid": "L1", "DestChannel": "PJSIP/1002-00000002", "DialString": "PJSIP/1002"},
		{"Event": "Newstate", "Linkedid": "L1", "ChannelStateDesc": "Up", "Channel": "PJSIP/1002-00000002"},
		{"Event": "DialEnd", "Linkedid": "L1", "DialStatus": "ANSWER"},
		{"Event": "Hangup", "Linkedid": "L1", "Uniqueid": "U2", "Cause": "16", "Cause-Txt": "Normal Clearing"},
		{"Event": "Hangup", "Linkedid": "L1", "Uniqueid": "L1", "Cause": "16", "Cause-Txt": "Normal Clearing"},
	}
	for _, ev := range seq {
		ing.Handle(ctx, ev)
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 call created, got %d", len(repo.created))
	}
	call := repo.created[0]
	if call.Direction != "internal" || call.FromNumber != "1001" || call.ToNumber != "1002" {
		t.Fatalf("unexpected call: dir=%s from=%s to=%s", call.Direction, call.FromNumber, call.ToNumber)
	}

	final := repo.lastUpdate(call.ID)
	if final["disposition"] != "answered" {
		t.Fatalf("disposition = %v, want answered", final["disposition"])
	}
	if _, ok := final["ended_at"]; !ok {
		t.Fatalf("expected ended_at to be set")
	}
	if final["hangup_cause_code"] != 16 {
		t.Fatalf("hangup_cause_code = %v, want 16", final["hangup_cause_code"])
	}

	types := map[string]bool{}
	for _, e := range repo.events {
		types[e.Type] = true
	}
	for _, want := range []string{"ringing", "answered", "hangup"} {
		if !types[want] {
			t.Fatalf("missing %q event; got %v", want, types)
		}
	}

	if _, live := ing.calls["L1"]; live {
		t.Fatalf("call state should be cleared after finalize")
	}
}

func TestIngestUnansweredCall(t *testing.T) {
	repo := newFakeRepo()
	ing := NewIngester(repo)
	ctx := context.Background()

	seq := []ami.Event{
		{"Event": "Newchannel", "Linkedid": "L2", "Uniqueid": "L2", "CallerIDNum": "1001", "Exten": "1002", "Channel": "PJSIP/1001-1"},
		{"Event": "DialBegin", "Linkedid": "L2", "DestChannel": "PJSIP/1002-2"},
		{"Event": "DialEnd", "Linkedid": "L2", "DialStatus": "BUSY"},
		{"Event": "Hangup", "Linkedid": "L2", "Uniqueid": "L2", "Cause": "17", "Cause-Txt": "User busy"},
	}
	for _, ev := range seq {
		ing.Handle(ctx, ev)
	}

	final := repo.lastUpdate(repo.created[0].ID)
	if final["disposition"] != "busy" {
		t.Fatalf("disposition = %v, want busy", final["disposition"])
	}
}
