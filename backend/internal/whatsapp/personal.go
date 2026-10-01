package whatsapp

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// A person's own WhatsApp preferences: whether new messages make a sound
// or a desktop notice, a mute for everything for a while, and mutes and
// pins on single conversations. Nobody else is affected by them.

// forever stands for "until I turn it back on".
var forever = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// ConvPref is one conversation's mute and pin for the person.
type ConvPref struct {
	ID         uint       `json:"id"`
	MutedUntil *time.Time `json:"mutedUntil,omitempty"`
	PinnedAt   *time.Time `json:"pinnedAt,omitempty"`
}

// Prefs is everything the inbox needs to know about the person's choices.
type Prefs struct {
	Sound         bool       `json:"sound"`
	Desktop       bool       `json:"desktop"`
	MutedUntil    *time.Time `json:"mutedUntil,omitempty"`
	Conversations []ConvPref `json:"conversations"`
}

// PrefsInput changes the general preferences. Mute is "", "1h", "8h",
// "1d", "1w", "always" or "off"; empty leaves it as it is.
type PrefsInput struct {
	Sound   *bool  `json:"sound"`
	Desktop *bool  `json:"desktop"`
	Mute    string `json:"mute"`
}

// ConvPrefInput changes one conversation. Mute as above; Pin nil leaves it.
type ConvPrefInput struct {
	Mute string `json:"mute"`
	Pin  *bool  `json:"pin"`
}

func muteUntil(word string) (*time.Time, bool, error) {
	now := time.Now()
	var d time.Duration
	switch word {
	case "":
		return nil, false, nil
	case "off":
		return nil, true, nil
	case "always":
		t := forever
		return &t, true, nil
	case "1h":
		d = time.Hour
	case "8h":
		d = 8 * time.Hour
	case "1d":
		d = 24 * time.Hour
	case "1w":
		d = 7 * 24 * time.Hour
	default:
		return nil, false, errs.Invalid("Sessize alma süresi tanınmadı.", nil)
	}
	t := now.Add(d)
	return &t, true, nil
}

// MyPrefs returns the person's preferences.
func (s *Service) MyPrefs(ctx context.Context, actorID uint) (*Prefs, error) {
	if _, err := s.viewerOf(ctx, actorID); err != nil {
		return nil, err
	}
	out := &Prefs{Sound: true, Desktop: true, Conversations: []ConvPref{}}
	if row, found := s.repo.UserPrefs(ctx, actorID); found {
		out.Sound, out.Desktop = row.Sound, row.Desktop
		if row.MutedUntil != nil && row.MutedUntil.After(time.Now()) {
			out.MutedUntil = row.MutedUntil
		}
	}
	convs, err := s.repo.ConversationPrefs(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, c := range convs {
		p := ConvPref{ID: c.ConversationID, PinnedAt: c.PinnedAt}
		if c.MutedUntil != nil && c.MutedUntil.After(time.Now()) {
			p.MutedUntil = c.MutedUntil
		}
		out.Conversations = append(out.Conversations, p)
	}
	return out, nil
}

// SavePrefs changes the general preferences.
func (s *Service) SavePrefs(ctx context.Context, actorID uint, in PrefsInput) (*Prefs, error) {
	cur, err := s.MyPrefs(ctx, actorID)
	if err != nil {
		return nil, err
	}
	sound, desktop, until := cur.Sound, cur.Desktop, cur.MutedUntil
	if in.Sound != nil {
		sound = *in.Sound
	}
	if in.Desktop != nil {
		desktop = *in.Desktop
	}
	if t, set, err := muteUntil(in.Mute); err != nil {
		return nil, err
	} else if set {
		until = t
	}
	if err := s.repo.SaveUserPrefs(ctx, actorID, sound, desktop, until); err != nil {
		return nil, errs.Internal(err)
	}
	return s.MyPrefs(ctx, actorID)
}

// SaveConvPref mutes or pins one conversation for the person.
func (s *Service) SaveConvPref(ctx context.Context, actorID, conversationID uint, in ConvPrefInput) (*Prefs, error) {
	if _, _, _, err := s.reachable(ctx, actorID, conversationID); err != nil {
		return nil, err
	}
	until, setMute, err := muteUntil(in.Mute)
	if err != nil {
		return nil, err
	}
	if !setMute && in.Pin == nil {
		return s.MyPrefs(ctx, actorID)
	}
	if err := s.repo.AddConversationPref(ctx, actorID, conversationID); err != nil {
		return nil, errs.Internal(err)
	}
	if setMute {
		warnDB(ctx, s.repo.MuteConversation(ctx, actorID, conversationID, until))
	}
	if in.Pin != nil {
		var pinned *time.Time
		if *in.Pin {
			now := time.Now()
			pinned = &now
		}
		warnDB(ctx, s.repo.PinConversation(ctx, actorID, conversationID, pinned))
	}
	warnDB(ctx, s.repo.DropIdleConversationPrefs(ctx, actorID))
	return s.MyPrefs(ctx, actorID)
}
