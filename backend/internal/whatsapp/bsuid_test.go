package whatsapp

import (
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/store"
)

func TestIsBSUID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"TR.1502576394655843", true},
		{"US.1234567890", true},
		{"905321234567", false},
		{"", false},
		{"5551234567", false},
	}
	for _, tt := range tests {
		if got := store.IsBSUID(tt.input); got != tt.want {
			t.Errorf("IsBSUID(%q) = %v; want %v", tt.input, got, tt.want)
		}
	}
}

func TestHookMessageSender(t *testing.T) {
	// 1. Standard from phone
	m1 := hookMessage{From: "905321234567"}
	if m1.Sender() != "905321234567" {
		t.Fatalf("expected 905321234567, got %s", m1.Sender())
	}

	// 2. BSUID from_user_id
	m2 := hookMessage{FromUserID: "TR.1502576394655843"}
	if m2.Sender() != "TR.1502576394655843" {
		t.Fatalf("expected TR.1502576394655843, got %s", m2.Sender())
	}
}

func TestExtractBSUIDFromPayload(t *testing.T) {
	payload := `{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "1000",
			"changes": [{
				"field": "messages",
				"value": {
					"messaging_product": "whatsapp",
					"metadata": {
						"display_phone_number": "908500000000",
						"phone_number_id": "111"
					},
					"contacts": [{
						"profile": {
							"name": "Kıvanç Danış",
							"username": "kivanc"
						},
						"user_id": "TR.1502576394655843"
					}],
					"messages": [{
						"from_user_id": "TR.1502576394655843",
						"id": "wamid.HBgL12345",
						"timestamp": "1728000000",
						"text": {"body": "merhaba"},
						"type": "text"
					}]
				}
			}]
		}]
	}`

	// Find by WAMID
	got1 := extractBSUIDFromPayload(payload, "wamid.HBgL12345", "")
	if got1 != "TR.1502576394655843" {
		t.Fatalf("expected TR.1502576394655843, got %s", got1)
	}

	// Find by Profile Name
	got2 := extractBSUIDFromPayload(payload, "", "Kıvanç Danış")
	if got2 != "TR.1502576394655843" {
		t.Fatalf("expected TR.1502576394655843, got %s", got2)
	}

	// Find by Username
	got3 := extractBSUIDFromPayload(payload, "", "kivanc")
	if got3 != "TR.1502576394655843" {
		t.Fatalf("expected TR.1502576394655843, got %s", got3)
	}
}

func TestContactViewDisplay(t *testing.T) {
	// Standard contact with name
	c1 := &models.WAContact{Name: "Ahmet Yılmaz", WAID: "905321234567"}
	if v := contactView(c1); v.Display != "Ahmet Yılmaz" {
		t.Fatalf("expected Ahmet Yılmaz, got %s", v.Display)
	}

	// Standard contact without name but with phone
	c2 := &models.WAContact{WAID: "905321234567"}
	if v := contactView(c2); v.Display != "+905321234567" {
		t.Fatalf("expected +905321234567, got %s", v.Display)
	}

	// Contact with BSUID and profile name
	c3 := &models.WAContact{ProfileName: "Kıvanç Danış", WAID: "TR.1502576394655843"}
	if v := contactView(c3); v.Display != "Kıvanç Danış" {
		t.Fatalf("expected Kıvanç Danış, got %s", v.Display)
	}

	// Contact with BSUID and NO name or profile name
	c4 := &models.WAContact{WAID: "TR.1502576394655843"}
	if v := contactView(c4); v.Display != "Numara Gizli" {
		t.Fatalf("expected Numara Gizli, got %s", v.Display)
	}

	// Contact with empty WAID
	c5 := &models.WAContact{}
	if v := contactView(c5); v.Display != "Numara Gizli" {
		t.Fatalf("expected Numara Gizli, got %s", v.Display)
	}
}
