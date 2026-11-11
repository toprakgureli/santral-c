package followup

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPeerContextWhatsAppSerialization(t *testing.T) {
	now := time.Now()
	ctx := PeerContext{
		InboundBefore: 1,
		WhatsApp: &WAContactContext{
			Found:       true,
			Name:        "Ahmet Yılmaz",
			ProfileName: "ahmet_y",
			ActiveTicket: &WAActiveTicket{
				ID:            42,
				Number:        1001,
				Status:        "open",
				Category:      "Sipariş",
				Priority:      "high",
				AwaitingSince: &now,
				CreatedAt:     now,
			},
		},
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed PeerContext
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed.WhatsApp == nil || !parsed.WhatsApp.Found || parsed.WhatsApp.Name != "Ahmet Yılmaz" {
		t.Fatalf("whatsapp contact mismatch: %+v", parsed.WhatsApp)
	}
	if parsed.WhatsApp.ActiveTicket == nil || parsed.WhatsApp.ActiveTicket.Number != 1001 || parsed.WhatsApp.ActiveTicket.Status != "open" {
		t.Fatalf("active ticket mismatch: %+v", parsed.WhatsApp.ActiveTicket)
	}
}
