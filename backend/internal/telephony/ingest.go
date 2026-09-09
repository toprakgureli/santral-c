package telephony

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/ami"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

var internalExtension = regexp.MustCompile(`^[1-9][0-9]{2,4}$`)

// liveCall is the in-flight state of a call keyed by Asterisk linkedid.
type liveCall struct {
	id         uint
	startedAt  time.Time
	answeredAt *time.Time
	answered   bool
	channels   map[string]struct{}
	seq        int
	dialStatus string
	causeCode  *int
	causeText  string
}

// Ingester consumes AMI events and turns them into call records.
type Ingester struct {
	repo IIngestRepo

	mu    sync.Mutex
	calls map[string]*liveCall
}

// NewIngester builds an AMI event ingester.
func NewIngester(repo IIngestRepo) *Ingester {
	return &Ingester{repo: repo, calls: make(map[string]*liveCall)}
}

// Handle dispatches one AMI event. It is safe to call from the AMI read loop.
func (i *Ingester) Handle(ctx context.Context, ev ami.Event) {
	switch ev.Name() {
	case "Newchannel":
		i.onNewchannel(ctx, ev)
	case "DialBegin":
		i.onDialBegin(ctx, ev)
	case "DialEnd":
		i.onDialEnd(ev)
	case "Newstate", "BridgeEnter":
		i.onAnswered(ctx, ev)
	case "Hangup":
		i.onHangup(ctx, ev)
	case "RTCPReceived", "RTCPSent":
		i.onRTCP(ctx, ev)
	}
}

func (i *Ingester) onNewchannel(ctx context.Context, ev ami.Event) {
	linkedid := ev.Get("Linkedid")
	uid := ev.Get("Uniqueid")
	if linkedid == "" || uid == "" {
		return
	}

	i.mu.Lock()
	lc, ok := i.calls[linkedid]
	if !ok {
		lc = &liveCall{startedAt: time.Now(), channels: make(map[string]struct{})}
		i.calls[linkedid] = lc
	}
	lc.channels[uid] = struct{}{}
	isRoot := !ok
	i.mu.Unlock()

	if !isRoot {
		return
	}

	from := ev.Get("CallerIDNum")
	to := ev.Get("Exten")
	direction := classifyDirection(from, to)

	call := &models.Call{
		Linkedid:    linkedid,
		Direction:   string(direction),
		Disposition: string(enums.DispositionInProgress),
		FromNumber:  from,
		ToNumber:    to,
		StartedAt:   lc.startedAt,
	}
	i.assignParties(ctx, call, direction, from, to)

	if err := i.repo.CreateCall(ctx, call); err != nil {
		slog.Warn("call could not be created from AMI", "linkedid", linkedid, "error", err)
		return
	}
	i.mu.Lock()
	lc.id = call.ID
	i.mu.Unlock()
}

func (i *Ingester) onDialBegin(ctx context.Context, ev ami.Event) {
	lc := i.get(ev.Get("Linkedid"))
	if lc == nil || lc.id == 0 {
		return
	}
	i.event(ctx, lc, enums.CallEventRinging, ev.Get("DestChannel"), map[string]any{
		"dialString": ev.Get("DialString"),
	})
}

func (i *Ingester) onDialEnd(ev ami.Event) {
	lc := i.get(ev.Get("Linkedid"))
	if lc == nil {
		return
	}
	i.mu.Lock()
	lc.dialStatus = ev.Get("DialStatus")
	i.mu.Unlock()
}

func (i *Ingester) onAnswered(ctx context.Context, ev ami.Event) {
	if ev.Name() == "Newstate" && !strings.EqualFold(ev.Get("ChannelStateDesc"), "Up") {
		return
	}
	lc := i.get(ev.Get("Linkedid"))
	if lc == nil || lc.id == 0 {
		return
	}
	i.mu.Lock()
	if lc.answered {
		i.mu.Unlock()
		return
	}
	now := time.Now()
	lc.answered = true
	lc.answeredAt = &now
	id := lc.id
	i.mu.Unlock()

	if err := i.repo.UpdateCall(ctx, id, map[string]any{
		"answered_at": now,
		"disposition": string(enums.DispositionAnswered),
	}); err != nil {
		slog.Warn("call answer could not be recorded", "error", err)
	}
	i.event(ctx, lc, enums.CallEventAnswered, ev.Get("Channel"), nil)
}

func (i *Ingester) onHangup(ctx context.Context, ev ami.Event) {
	linkedid := ev.Get("Linkedid")
	uid := ev.Get("Uniqueid")
	lc := i.get(linkedid)
	if lc == nil {
		return
	}

	i.mu.Lock()
	if code, err := strconv.Atoi(ev.Get("Cause")); err == nil {
		lc.causeCode = &code
		lc.causeText = ev.Get("Cause-Txt")
	}
	delete(lc.channels, uid)
	remaining := len(lc.channels)
	i.mu.Unlock()

	if remaining > 0 {
		return
	}
	i.finalize(ctx, linkedid, lc)
}

func (i *Ingester) finalize(ctx context.Context, linkedid string, lc *liveCall) {
	i.mu.Lock()
	delete(i.calls, linkedid)
	endedAt := time.Now()
	var ring, talk int
	if lc.answered && lc.answeredAt != nil {
		ring = int(lc.answeredAt.Sub(lc.startedAt).Seconds())
		talk = int(endedAt.Sub(*lc.answeredAt).Seconds())
	} else {
		ring = int(endedAt.Sub(lc.startedAt).Seconds())
	}
	disposition := finalDisposition(lc.answered, lc.dialStatus)
	causeCode := lc.causeCode
	causeText := lc.causeText
	id := lc.id
	i.mu.Unlock()

	if id == 0 {
		return
	}

	fields := map[string]any{
		"ended_at":     endedAt,
		"disposition":  string(disposition),
		"ring_seconds": ring,
		"talk_seconds": talk,
	}
	if causeCode != nil {
		fields["hangup_cause_code"] = *causeCode
		fields["hangup_cause_text"] = causeText
	}
	if err := i.repo.UpdateCall(ctx, id, fields); err != nil {
		slog.Warn("call could not be finalized", "linkedid", linkedid, "error", err)
	}
	i.event(ctx, lc, enums.CallEventHangup, "", map[string]any{
		"causeCode":  causeCode,
		"causeText":  causeText,
		"dialStatus": lc.dialStatus,
	})
}

func (i *Ingester) onRTCP(ctx context.Context, ev ami.Event) {
	lc := i.get(ev.Get("Linkedid"))
	if lc == nil || lc.id == 0 {
		return
	}
	q := &models.CallQuality{
		CallID:  lc.id,
		Leg:     legForChannel(ev.Get("Channel")),
		Channel: ev.Get("Channel"),
		At:      time.Now(),
	}
	jitter := firstFloat(ev, "IAJitter", "Jitter")
	if jitter != nil {
		ms := *jitter * 1000
		q.JitterMs = &ms
	}
	rtt := firstFloat(ev, "RTT")
	if rtt != nil {
		ms := *rtt * 1000
		q.RTTMs = &ms
	}
	if loss := fractionLostPct(ev.Get("FractionLost")); loss != nil {
		q.LossPct = loss
	}
	if q.JitterMs != nil || q.RTTMs != nil || q.LossPct != nil {
		mos := estimateMOS(valueOr(q.LossPct), valueOr(q.JitterMs), valueOr(q.RTTMs))
		q.MOS = &mos
	}
	if err := i.repo.AddQuality(ctx, q); err != nil {
		slog.Warn("quality sample could not be recorded", "error", err)
	}
}

func (i *Ingester) assignParties(ctx context.Context, call *models.Call, direction enums.Direction, from, to string) {
	switch direction {
	case enums.DirectionOutbound:
		call.FromUserID = i.userID(ctx, from)
		call.ContactID = i.contactID(ctx, to)
	case enums.DirectionInbound:
		call.ToUserID = i.userID(ctx, to)
		call.ContactID = i.contactID(ctx, from)
	case enums.DirectionInternal:
		call.FromUserID = i.userID(ctx, from)
		call.ToUserID = i.userID(ctx, to)
	}
}

func (i *Ingester) userID(ctx context.Context, ext string) *uint {
	if !internalExtension.MatchString(ext) {
		return nil
	}
	id, err := i.repo.UserIDByExtension(ctx, ext)
	if err != nil {
		slog.Warn("extension could not be resolved", "ext", ext, "error", err)
		return nil
	}
	return id
}

func (i *Ingester) contactID(ctx context.Context, number string) *uint {
	e164, err := phone.Normalize(number)
	if err != nil {
		return nil
	}
	id, err := i.repo.ContactIDByNumber(ctx, e164)
	if err != nil {
		slog.Warn("contact could not be resolved", "number", e164, "error", err)
		return nil
	}
	return id
}

func (i *Ingester) event(ctx context.Context, lc *liveCall, t enums.CallEventType, channel string, detail map[string]any) {
	i.mu.Lock()
	if lc.id == 0 {
		i.mu.Unlock()
		return
	}
	lc.seq++
	seq := lc.seq
	id := lc.id
	i.mu.Unlock()

	payload := "{}"
	if detail != nil {
		if raw, err := json.Marshal(detail); err == nil {
			payload = string(raw)
		}
	}
	e := &models.CallEvent{
		CallID:  id,
		Seq:     seq,
		Type:    string(t),
		Channel: channel,
		At:      time.Now(),
		Detail:  payload,
	}
	if err := i.repo.AddEvent(ctx, e); err != nil {
		slog.Warn("call event could not be recorded", "type", string(t), "error", err)
	}
}

func (i *Ingester) get(linkedid string) *liveCall {
	if linkedid == "" {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.calls[linkedid]
}

func classifyDirection(from, to string) enums.Direction {
	fromInternal := internalExtension.MatchString(from)
	toInternal := internalExtension.MatchString(to)
	switch {
	case fromInternal && toInternal:
		return enums.DirectionInternal
	case fromInternal && !toInternal:
		return enums.DirectionOutbound
	default:
		return enums.DirectionInbound
	}
}

func finalDisposition(answered bool, dialStatus string) enums.Disposition {
	if answered {
		return enums.DispositionAnswered
	}
	switch strings.ToUpper(dialStatus) {
	case "BUSY":
		return enums.DispositionBusy
	case "CANCEL":
		return enums.DispositionCanceled
	case "CONGESTION", "CHANUNAVAIL":
		return enums.DispositionFailed
	default:
		return enums.DispositionNoAnswer
	}
}

func legForChannel(channel string) string {
	if idx := strings.Index(channel, "/"); idx >= 0 {
		rest := channel[idx+1:]
		if dash := strings.Index(rest, "-"); dash >= 0 {
			rest = rest[:dash]
		}
		if internalExtension.MatchString(rest) {
			return string(enums.QualityLegAgent)
		}
	}
	return string(enums.QualityLegTrunk)
}

func firstFloat(ev ami.Event, keys ...string) *float64 {
	for _, k := range keys {
		if raw := ev.Get(k); raw != "" {
			if f, err := strconv.ParseFloat(raw, 64); err == nil {
				return &f
			}
		}
	}
	return nil
}

func fractionLostPct(raw string) *float64 {
	if raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	// FractionLost is an 8-bit fixed point fraction (0-255).
	pct := f / 256 * 100
	if f <= 1 {
		pct = f * 100 // already a 0-1 fraction
	}
	return &pct
}

func valueOr(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
