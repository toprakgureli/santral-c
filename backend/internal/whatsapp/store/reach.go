package store

import "strings"

// Reach is which tickets one person sees, as the panel's permissions
// decide it. Queries narrow their rows to it before their LIMIT, so a
// person with a narrow view still gets a full page of what they may see.
type Reach struct {
	// None is set when the person sees no ticket at all.
	None bool
	// All is set when the person sees every ticket.
	All bool
	// Channels are the devices the person is placed on.
	Channels []uint
	// UserID is the person; they see the tickets they own or take part in.
	UserID uint
	// Teams are the teams whose tickets the person sees.
	Teams []uint
	// Pool is set when the person sees open tickets nobody owns.
	Pool bool
	// Waiting is set when the person sees the tickets on the waiting list.
	Waiting bool
}

// tickets is the query of the ids of the tickets the person sees.
func (r Reach) tickets() (string, []any) {
	if r.None || (!r.All && len(r.Channels) == 0) {
		return "SELECT id FROM wa_tickets WHERE false", nil
	}
	var where []string
	var args []any
	if !r.All {
		where = append(where, "t.channel_id IN ?")
		args = append(args, r.Channels)
		reach := []string{"t.owner_id = ?", "EXISTS (SELECT 1 FROM wa_ticket_participants p WHERE p.ticket_id = t.id AND p.user_id = ?)"}
		args = append(args, r.UserID, r.UserID)
		if len(r.Teams) > 0 {
			reach = append(reach, "t.team_id IN ?")
			args = append(args, r.Teams)
		}
		if r.Pool {
			reach = append(reach, "(t.status NOT IN ('resolved','bot') AND t.owner_id IS NULL)")
		}
		if r.Waiting {
			reach = append(reach, "(t.waiting_listed_at IS NOT NULL AND t.status <> 'resolved')")
		}
		where = append(where, "("+strings.Join(reach, " OR ")+")")
	}
	q := "SELECT t.id FROM wa_tickets t"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	return q, args
}
