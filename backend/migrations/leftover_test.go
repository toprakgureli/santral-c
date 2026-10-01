package migrations

import "testing"

func TestLeftoverNames(t *testing.T) {
	for name, want := range map[string]bool{
		"wa_messages_created_idx_ccnew":  true,
		"wa_messages_created_idx_ccnew1": true,
		"wa_messages_created_idx_ccold":  true,
		"wa_messages_created_idx":        false,
		"accnewsletter_idx":              false,
		"x_ccnew_y":                      false,
	} {
		if got := leftover(name); got != want {
			t.Errorf("leftover(%q) = %v, want %v", name, got, want)
		}
	}
}
