package games

import (
	"context"
	"encoding/json"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// A stub room: the simulation needs no chat.
type quietRooms struct{}

func (quietRooms) IsMember(context.Context, uint, uint) bool                { return true }
func (quietRooms) MemberIDs(context.Context, uint) ([]uint, error)          { return nil, nil }
func (quietRooms) PostGame(context.Context, uint, uint, uint) (uint, error) { return 0, nil }
func (quietRooms) PostSystem(context.Context, uint, string)                 {}
func (quietRooms) Push([]uint, any)                                         {}

// TestHockeyNeverWedges drives two random, frantic mallets for a long
// while and checks the puck never leaves the table, never turns NaN and
// never sits still for long, whatever the mallets do to it.
func TestHockeyNeverWedges(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		t.Run("seed", func(t *testing.T) { runHockeyChaos(t, seed) })
	}
}

func runHockeyChaos(t *testing.T, seed int64) {
	k := &hockeyKind{}
	s := &Service{rooms: quietRooms{}, live: map[uint]*Match{}, rnd: rand.New(rand.NewSource(7))}
	m := &Match{G: &models.Game{ID: 1, Kind: "hockey", Status: statusPlaying, Winners: "[]"}, Kind: k, Config: Config{Rounds: 1000}, Paused: map[uint]bool{},
		Players: []Player{{UserID: 1, Name: "A"}, {UserID: 2, Name: "B"}}, Data: k.NewState()}
	if err := k.Start(m, s, context.Background()); err != nil {
		t.Fatal(err)
	}
	st := m.Data.(*hockeyState)
	rng := rand.New(rand.NewSource(seed))
	still := 0
	maxStill := 0
	for tick := 0; tick < 60000; tick++ {
		// Mallets chase the puck with jitter, sometimes lunging, sometimes
		// pinning it against a board.
		for seat := uint(1); seat <= 2; seat++ {
			var target [2]float64
			switch rng.Intn(4) {
			case 0:
				target = [2]float64{st.Puck[0] + rng.Float64()*20 - 10, st.Puck[1] + rng.Float64()*20 - 10}
			case 1:
				target = [2]float64{rng.Float64() * hkW, rng.Float64() * hkH}
			case 2:
				target = [2]float64{hkPuck, st.Puck[1]} // hug the side board
			default:
				target = [2]float64{st.Puck[0], st.Puck[1]} // sit on it
			}
			raw, _ := json.Marshal(map[string]float64{"x": target[0], "y": target[1]})
			if _, err := k.Act(m, s, context.Background(), seat, "move", raw); err != nil {
				t.Fatalf("move: %v", err)
			}
		}
		before := st.Puck
		k.step(m, s, context.Background())
		for i, v := range st.Puck {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("tick %d: puck[%d] is %v", tick, i, v)
			}
		}
		// In the goal phase the puck sits in the net, past the end board.
		if st.Phase == "play" && (st.Puck[0] < hkPuck-1e-6 || st.Puck[0] > hkW-hkPuck+1e-6 || st.Puck[1] < hkPuck-1e-6 || st.Puck[1] > hkH-hkPuck+1e-6) {
			t.Fatalf("tick %d: puck off the table at %.2f,%.2f", tick, st.Puck[0], st.Puck[1])
		}
		for i := 0; i < 2; i++ {
			if d := math.Hypot(st.Puck[0]-st.Pads[i][0], st.Puck[1]-st.Pads[i][1]); d < hkPad+hkPuck-0.5 && st.Phase == "play" {
				t.Fatalf("tick %d: puck inside mallet %d (%.2f) puck=%.2f,%.2f v=%.2f,%.2f pads=%v before=%v", tick, i, d, st.Puck[0], st.Puck[1], st.Puck[2], st.Puck[3], st.Pads, before)
			}
		}
		if st.Phase == "goal" {
			st.GoalAt = time.Now().Add(-hkGoalPause - time.Second)
			continue
		}
		if math.Hypot(st.Puck[0]-before[0], st.Puck[1]-before[1]) < 0.05 {
			still++
			if still > maxStill {
				maxStill = still
			}
		} else {
			still = 0
		}
	}
	if maxStill > 70 {
		t.Fatalf("puck sat still for %d ticks", maxStill)
	}
	if st.Score[0]+st.Score[1] == 0 {
		t.Fatal("no goal in a minute of chaos, the puck must be stuck")
	}
}
