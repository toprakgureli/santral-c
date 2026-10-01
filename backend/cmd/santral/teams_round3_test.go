package main

// Room history for people who join later: whoever adds or invites someone
// chooses how much of the earlier conversation they may read (nothing, the
// last 50 or 100 messages, or all of it). These tests run the real routes
// against the database and check every place a member reads lines: pages,
// jumps, search, shared files, quotes, receipts, unread counts and the room
// list preview. Direct messages are left alone.

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// line is a message as a test reads it.
type line struct {
	ID      uint   `json:"id"`
	Kind    string `json:"kind"`
	Body    string `json:"body"`
	ReplyTo *struct {
		ID     uint   `json:"id"`
		Body   string `json:"body"`
		Sender string `json:"sender"`
		Hidden bool   `json:"hidden"`
	} `json:"replyTo"`
}

type linePage struct {
	Items     []line `json:"items"`
	More      bool   `json:"more"`
	MoreNewer bool   `json:"moreNewer"`
}

// allLines pages a room back to its first readable line, oldest first.
func allLines(t *testing.T, b *browser, groupID uint) []line {
	t.Helper()
	var out []line
	before := uint(0)
	for range 200 {
		path := fmt.Sprintf("/api/v1/teams/groups/%d/messages", groupID)
		if before > 0 {
			path += fmt.Sprintf("?before=%d", before)
		}
		a := b.do(fiber.MethodGet, path, nil)
		if a.status != fiber.StatusOK {
			t.Fatalf("%s: page answered %d %s", b.user.Email, a.status, a.body)
		}
		var p linePage
		a.json(t, &p)
		out = append(p.Items, out...)
		if !p.More || len(p.Items) == 0 {
			return out
		}
		before = p.Items[0].ID
	}
	t.Fatal("paging never ended")
	return nil
}

// olds counts the lines whose text starts with "eski".
func olds(lines []line) int {
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l.Body, "eski ") {
			n++
		}
	}
	return n
}

func search(t *testing.T, b *browser, groupID uint, q string) int {
	t.Helper()
	a := b.do(fiber.MethodGet, fmt.Sprintf("/api/v1/teams/groups/%d/search?q=%s", groupID, url.QueryEscape(q)), nil)
	if a.status != fiber.StatusOK {
		t.Fatalf("search answered %d %s", a.status, a.body)
	}
	var r struct {
		Items []line `json:"items"`
	}
	a.json(t, &r)
	return len(r.Items)
}

func media(t *testing.T, b *browser, groupID uint) map[uint]bool {
	t.Helper()
	a := b.do(fiber.MethodGet, fmt.Sprintf("/api/v1/teams/groups/%d/media?kind=file", groupID), nil)
	if a.status != fiber.StatusOK {
		t.Fatalf("media answered %d %s", a.status, a.body)
	}
	var r struct {
		Items []struct {
			ID uint `json:"id"`
		} `json:"items"`
	}
	a.json(t, &r)
	out := map[uint]bool{}
	for _, it := range r.Items {
		out[it.ID] = true
	}
	return out
}

type roomCard struct {
	ID          uint  `json:"id"`
	Unread      int64 `json:"unread"`
	LastMessage *line `json:"lastMessage"`
}

func overview(t *testing.T, b *browser) (map[uint]roomCard, []struct {
	ID      uint `json:"id"`
	GroupID uint `json:"groupId"`
}) {
	t.Helper()
	a := b.do(fiber.MethodGet, "/api/v1/teams/overview", nil)
	if a.status != fiber.StatusOK {
		t.Fatalf("overview answered %d %s", a.status, a.body)
	}
	var ov struct {
		Groups  []roomCard `json:"groups"`
		Invites []struct {
			ID      uint `json:"id"`
			GroupID uint `json:"groupId"`
		} `json:"invites"`
	}
	a.json(t, &ov)
	out := map[uint]roomCard{}
	for _, g := range ov.Groups {
		out[g.ID] = g
	}
	return out, ov.Invites
}

func send(t *testing.T, b *browser, groupID uint, body map[string]any) line {
	t.Helper()
	a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/messages", groupID), body)
	if a.status >= 300 {
		t.Fatalf("%s: send answered %d %s", b.user.Email, a.status, a.body)
	}
	var l line
	a.json(t, &l)
	return l
}

// fileOn records a ready shared file on a line, as an upload would leave it.
func fileOn(t *testing.T, db *gorm.DB, groupID, messageID, uploader uint, name string) uint {
	t.Helper()
	a := models.ChatAttachment{GroupID: groupID, MessageID: &messageID, UploaderID: uploader, Kind: "file", Name: name, Mime: "application/pdf", Size: 10, DriveID: "test-" + name, Status: "ready", CreatedAt: time.Now()}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestTeamsHistoryForNewMembers(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	owner := seedPeople(t, db, 1, enums.RoleManager, false)[0]
	people := seedPeople(t, db, 6, enums.RoleSalesTeam, false)
	ob := newBrowser(t, srv.app, nil, owner)
	ob.mustSignIn()

	a := ob.do(fiber.MethodPost, "/api/v1/teams/groups", map[string]any{"name": "Geçmiş testi", "postPolicy": "everyone"})
	if a.status >= 300 {
		t.Fatalf("group create answered %d %s", a.status, a.body)
	}
	var group struct {
		ID uint `json:"id"`
	}
	a.json(t, &group)
	gid := group.ID

	// 120 messages before anyone joins; files on the 10th and the 115th.
	old := make([]line, 0, 120)
	for i := 1; i <= 120; i++ {
		old = append(old, send(t, ob, gid, map[string]any{"body": fmt.Sprintf("eski %03d", i)}))
	}
	// A deleted and a grey line in between do not count towards "the last 50".
	ob.do(fiber.MethodDelete, fmt.Sprintf("/api/v1/teams/groups/%d/messages/%d", gid, old[100].ID), nil)
	oldFile := fileOn(t, db, gid, old[9].ID, owner.ID, "eski.pdf")
	newFile := fileOn(t, db, gid, old[114].ID, owner.ID, "yeni.pdf")
	topOld := old[len(old)-1].ID

	none, last50, last100, all, invited, invitedDefault := people[0], people[1], people[2], people[3], people[4], people[5]
	for _, c := range []struct {
		who     models.User
		history string
	}{{none, "none"}, {last50, "50"}, {last100, "100"}, {all, "all"}} {
		if a := ob.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/members", gid), map[string]any{"userIds": []uint{c.who.ID}, "history": c.history}); a.status >= 300 {
			t.Fatalf("adding with %s answered %d %s", c.history, a.status, a.body)
		}
	}
	if a := ob.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/members", gid), map[string]any{"userIds": []uint{invited.ID}, "history": "20"}); a.status != fiber.StatusBadRequest && a.status != fiber.StatusUnprocessableEntity {
		t.Errorf("an unknown history choice answered %d", a.status)
	}
	if a := ob.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/invites", gid), map[string]any{"userIds": []uint{invited.ID}, "history": "50"}); a.status >= 300 {
		t.Fatalf("invite answered %d %s", a.status, a.body)
	}
	// No choice sent: nothing earlier.
	if a := ob.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/invites", gid), map[string]any{"userIds": []uint{invitedDefault.ID}}); a.status >= 300 {
		t.Fatalf("invite answered %d %s", a.status, a.body)
	}

	browsers := map[uint]*browser{}
	for _, p := range people {
		b := newBrowser(t, srv.app, nil, p)
		b.mustSignIn()
		browsers[p.ID] = b
	}
	// Lines written between the invite and the answer are before the join.
	between := send(t, ob, gid, map[string]any{"body": "eski arada"})
	for _, p := range []models.User{invited, invitedDefault} {
		_, invites := overview(t, browsers[p.ID])
		accepted := false
		for _, inv := range invites {
			if inv.GroupID == gid {
				if a := browsers[p.ID].do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/invites/%d/accept", inv.ID), nil); a.status >= 300 {
					t.Fatalf("accept answered %d %s", a.status, a.body)
				}
				accepted = true
			}
		}
		if !accepted {
			t.Fatalf("%s has no invite to accept", p.Email)
		}
	}

	// Everyone joined; the owner answers the very first line.
	reply := send(t, ob, gid, map[string]any{"body": "yeni yanıt", "replyToId": old[0].ID})

	// Each newcomer: how many lines starting with "eski" they may read.
	// The 50 and 100 count back from the moment they joined, over messages
	// people wrote (the deleted 101st does not count); "eski arada" was
	// written after the first four joined and before the invites were
	// answered.
	cases := []struct {
		who      models.User
		olds     int
		first    bool // the first line, quoted above
		line30   bool // eski 030
		line71   bool // eski 071
		oldFile  bool
		newFile  bool
		inviteMe bool
	}{
		{who: none, olds: 1},
		{who: last50, olds: 51, line71: true, newFile: true},
		{who: last100, olds: 101, line30: true, line71: true, newFile: true},
		{who: all, olds: 120, first: true, line30: true, line71: true, oldFile: true, newFile: true},
		{who: invited, olds: 50, line71: true, newFile: true, inviteMe: true},
		{who: invitedDefault, olds: 0, inviteMe: true},
	}
	for _, c := range cases {
		b := browsers[c.who.ID]
		lines := allLines(t, b, gid)
		if got := olds(lines); got != c.olds {
			t.Errorf("%s: reads %d earlier lines, want %d", c.who.Email, got, c.olds)
		}
		var seen *line
		for i := range lines {
			if lines[i].ID == reply.ID {
				seen = &lines[i]
			}
		}
		if seen == nil {
			t.Errorf("%s: the line written after joining is missing", c.who.Email)
		} else if seen.ReplyTo == nil {
			t.Errorf("%s: the reply lost its quote", c.who.Email)
		} else if c.first && (seen.ReplyTo.Hidden || seen.ReplyTo.Body != "eski 001") {
			t.Errorf("%s: should read the quote, got %+v", c.who.Email, seen.ReplyTo)
		} else if !c.first && (!seen.ReplyTo.Hidden || seen.ReplyTo.Body != "" || seen.ReplyTo.Sender != "") {
			t.Errorf("%s: the quote should be hidden, got %+v", c.who.Email, seen.ReplyTo)
		}
		for q, want := range map[string]bool{"eski 001": c.first, "eski 030": c.line30, "eski 071": c.line71, "yeni yanıt": true} {
			if got := search(t, b, gid, q) > 0; got != want {
				t.Errorf("%s: search %q found=%v, want %v", c.who.Email, q, got, want)
			}
		}
		files := media(t, b, gid)
		if files[oldFile] != c.oldFile || files[newFile] != c.newFile {
			t.Errorf("%s: files %v, want old=%v new=%v", c.who.Email, files, c.oldFile, c.newFile)
		}
		if st := b.do(fiber.MethodGet, fmt.Sprintf("/api/v1/teams/attachments/%d/thumb", oldFile), nil).status; (st == fiber.StatusNotFound) == c.oldFile {
			t.Errorf("%s: opening the old file answered %d", c.who.Email, st)
		}
		around := b.do(fiber.MethodGet, fmt.Sprintf("/api/v1/teams/groups/%d/messages?around=%d", gid, old[0].ID), nil)
		if (around.status == fiber.StatusOK) != c.first {
			t.Errorf("%s: jumping to the first line answered %d", c.who.Email, around.status)
		}
		if st := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/messages/%d/reactions", gid, old[0].ID), map[string]any{"emoji": "👍"}).status; (st < 300) != c.first {
			t.Errorf("%s: reacting to the first line answered %d", c.who.Email, st)
		}
		if st := b.do(fiber.MethodGet, fmt.Sprintf("/api/v1/teams/groups/%d/messages/%d/receipts", gid, old[0].ID), nil).status; (st == fiber.StatusOK) != c.first {
			t.Errorf("%s: the first line's info answered %d", c.who.Email, st)
		}

		// Unread: only what came after joining, never the history.
		rooms, _ := overview(t, b)
		card, ok := rooms[gid]
		if !ok {
			t.Fatalf("%s: the room is not in the list", c.who.Email)
		}
		if card.Unread < 1 || card.Unread > 8 {
			t.Errorf("%s: %d unread, want only the lines after joining", c.who.Email, card.Unread)
		}
		var seat models.ChatMember
		db.Where("group_id = ? AND user_id = ?", gid, c.who.ID).First(&seat)
		if seat.LastReadID < topOld || seat.LastDeliveredID < topOld {
			t.Errorf("%s: starts at read %d / delivered %d, want at least %d", c.who.Email, seat.LastReadID, seat.LastDeliveredID, topOld)
		}
		if c.inviteMe && seat.LastReadID < between.ID {
			t.Errorf("%s: accepting an invite left read at %d, before the newest line %d", c.who.Email, seat.LastReadID, between.ID)
		}
		// Reading the room never logs receipts for the history.
		if a := b.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/read", gid), map[string]any{}); a.status >= 300 {
			t.Errorf("mark read answered %d %s", a.status, a.body)
		}
		var logged int64
		db.Raw("SELECT count(*) FROM chat_receipts WHERE user_id = ? AND message_id <= ?", c.who.ID, topOld).Scan(&logged)
		if logged != 0 {
			t.Errorf("%s: %d receipts logged for lines from before joining", c.who.Email, logged)
		}
	}

	// A newcomer cannot quote a line they may not read; the line goes out plain.
	plain := send(t, browsers[none.ID], gid, map[string]any{"body": "selam", "replyToId": old[0].ID})
	if plain.ReplyTo != nil {
		t.Errorf("a hidden line was quoted: %+v", plain.ReplyTo)
	}

	// The owner's info for the first line lists only who may read it.
	var receipts struct {
		Items []struct {
			ID uint `json:"id"`
		} `json:"items"`
	}
	ob.do(fiber.MethodGet, fmt.Sprintf("/api/v1/teams/groups/%d/messages/%d/receipts", gid, old[0].ID), nil).json(t, &receipts)
	listed := map[uint]bool{}
	for _, r := range receipts.Items {
		listed[r.ID] = true
	}
	for _, p := range people {
		if listed[p.ID] != (p.ID == all.ID) {
			t.Errorf("first line's info lists %s = %v", p.Email, listed[p.ID])
		}
	}
	// When everything after joining is gone, the preview never falls back
	// to a line the person may not read.
	db.Exec("UPDATE chat_messages SET deleted_at = now() WHERE group_id = ? AND id > ?", gid, between.ID)
	for _, c := range []struct {
		who  models.User
		want string
	}{{invitedDefault, ""}, {none, "eski arada"}, {all, "eski arada"}} {
		rooms, _ := overview(t, browsers[c.who.ID])
		card := rooms[gid]
		got := ""
		if card.LastMessage != nil {
			got = card.LastMessage.Body
		}
		if got != c.want {
			t.Errorf("%s: preview %q, want %q", c.who.Email, got, c.want)
		}
	}
}

// A direct conversation keeps its whole history for both people.
func TestTeamsHistoryLeavesDirectMessages(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	two := seedPeople(t, db, 2, enums.RoleSalesTeam, false)
	x, y := newBrowser(t, srv.app, nil, two[0]), newBrowser(t, srv.app, nil, two[1])
	x.mustSignIn()
	y.mustSignIn()
	a := x.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/dm/%d", two[1].ID), nil)
	if a.status >= 300 {
		t.Fatalf("open dm answered %d %s", a.status, a.body)
	}
	var dm struct {
		ID uint `json:"id"`
	}
	a.json(t, &dm)
	for i := range 3 {
		send(t, x, dm.ID, map[string]any{"body": fmt.Sprintf("eski %d", i)})
	}
	if got := olds(allLines(t, y, dm.ID)); got != 3 {
		t.Errorf("the other side reads %d lines, want 3", got)
	}
	if st := x.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/members", dm.ID), map[string]any{"userIds": []uint{two[0].ID}, "history": "none"}).status; st < 400 {
		t.Errorf("adding to a direct conversation answered %d", st)
	}
	var froms []uint
	db.Model(&models.ChatMember{}).Where("group_id = ?", dm.ID).Pluck("history_from", &froms)
	for _, f := range froms {
		if f != 0 {
			t.Errorf("a direct conversation seat starts at %d", f)
		}
	}
}

// The busy room with its long history: a newcomer with "the last 100"
// reads exactly that and nothing counts as unread, while the room keeps
// its pace. The first run fills the history (about a minute).
func TestLoadTeamsHistoryBusyRoom(t *testing.T) {
	loadTest(t)
	srv, db := testServer(t, officeSecurity)
	busy := bigHistory(t, db)
	owner := seedPeople(t, db, 1, enums.RoleManager, false)[0]
	late := seedPeople(t, db, 10, enums.RoleSalesTeam, false)
	if err := db.Exec("INSERT INTO chat_members (group_id, user_id, role) VALUES (?, ?, 'owner')", busy, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM chat_members WHERE group_id = ? AND user_id IN ?", busy, append(ids(late), owner.ID))
		db.Exec("DELETE FROM chat_receipts WHERE user_id IN ?", ids(late))
	})
	stats := newTally()
	ob := newBrowser(t, srv.app, stats, owner)
	ob.mustSignIn()
	choices := []string{"none", "50", "100", "all"}
	began := time.Now()
	// Ten people added one by one, each with a choice, while they start
	// reading at once.
	for i, p := range late {
		if a := ob.do(fiber.MethodPost, fmt.Sprintf("/api/v1/teams/groups/%d/members", busy), map[string]any{"userIds": []uint{p.ID}, "history": choices[i%len(choices)]}); a.status >= 300 {
			t.Fatalf("adding answered %d %s", a.status, a.body)
		}
	}
	browsers := make([]*browser, len(late))
	for i, p := range late {
		browsers[i] = newBrowser(t, srv.app, stats, p)
		browsers[i].mustSignIn()
	}
	together(len(browsers), func(i int) {
		b := browsers[i]
		path := fmt.Sprintf("/api/v1/teams/groups/%d/messages", busy)
		for range 5 {
			for _, r := range []string{path, "/api/v1/teams/overview", fmt.Sprintf("/api/v1/teams/groups/%d/search?q=mesaj", busy)} {
				if a := b.do(fiber.MethodGet, r, nil); a.status >= 300 {
					t.Errorf("%s answered %d %s", r, a.status, a.body)
				}
			}
		}
	})
	elapsed := time.Since(began)
	for i, b := range browsers {
		rooms, _ := overview(t, b)
		// At most the grey "added" lines posted after they joined.
		if u := rooms[busy].Unread; u > int64(len(late)) {
			t.Errorf("%s: %d unread in the busy room", b.user.Email, u)
		}
		if choices[i%len(choices)] != "100" {
			continue
		}
		var n int64
		for _, l := range allLines(t, b, busy) {
			if strings.HasPrefix(l.Body, "Eski mesaj ") {
				n++
			}
		}
		if n != 100 {
			t.Errorf("%s: reads %d lines of the long history, want 100", b.user.Email, n)
		}
	}
	stats.report(t, "teams history", elapsed)
}
