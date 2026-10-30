package game

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestCatalog(t *testing.T) {
	weights := 0
	cities, roads, fields := 0, 0, 0
	seen := map[string]bool{}
	for _, d := range Catalog {
		if seen[d.ID] {
			t.Fatalf("duplicate %s", d.ID)
		}
		seen[d.ID] = true
		if d.Weight <= 0 {
			t.Fatalf("%s weight", d.ID)
		}
		weights += d.Weight
		used := map[int]string{}
		for _, g := range d.Cities {
			if len(g) == 0 {
				t.Fatalf("%s empty city", d.ID)
			}
			for _, e := range g {
				if d.Edges[e] != City || used[e] != "" {
					t.Fatalf("%s city edge %d", d.ID, e)
				}
				used[e] = City
			}
		}
		for _, g := range d.Roads {
			if len(g) == 0 {
				t.Fatalf("%s empty road", d.ID)
			}
			for _, e := range g {
				if d.Edges[e] != Road || used[e] != "" {
					t.Fatalf("%s road edge %d", d.ID, e)
				}
				used[e] = Road
			}
		}
		corners := map[int]bool{}
		for _, g := range d.Fields {
			if len(g) == 0 {
				t.Fatalf("%s empty field", d.ID)
			}
			for _, c := range g {
				if c < 0 || c > 3 || corners[c] {
					t.Fatalf("%s corner %d", d.ID, c)
				}
				corners[c] = true
			}
		}
		for e := 0; e < 4; e++ {
			switch d.Edges[e] {
			case City:
				cities += d.Weight
				if used[e] != City {
					t.Fatalf("%s missing city %d", d.ID, e)
				}
			case Road:
				roads += d.Weight
				if used[e] != Road {
					t.Fatalf("%s missing road %d", d.ID, e)
				}
			case Field:
				fields += d.Weight
				if used[e] != "" {
					t.Fatalf("%s field marked %d", d.ID, e)
				}
			default:
				t.Fatalf("%s bad edge", d.ID)
			}
			next := (e + 1) % 4
			if d.Edges[e] == City && d.Edges[next] == City && groupOf(d.Cities, e) == groupOf(d.Cities, next) {
				if corners[e] {
					t.Fatalf("%s city eats corner %d", d.ID, e)
				}
			}
		}
		if d.Shield && len(d.Cities) != 1 {
			t.Fatalf("%s shield on %d cities", d.ID, len(d.Cities))
		}
	}
	if weights != 72 || cities != 79 || roads != 94 || fields != 115 {
		t.Fatalf("mix tiles=%d city=%d road=%d field=%d", weights, cities, roads, fields)
	}
}

func newMatch(t *testing.T) *State {
	t.Helper()
	s := &State{
		ID: "g", Code: "ABCDEF", HostID: "p1", Open: true,
		Status: StatusLobby, MaxPlayers: 5, MeepleCount: 7,
		Players: []Player{
			{ID: "p1", Name: "Аня", Color: palette[0], Meeples: 7, Connected: true},
			{ID: "p2", Name: "Боря", Color: palette[1], Meeples: 7, Connected: true},
		},
	}
	s.UseRand(rand.New(rand.NewPCG(1, 2)))
	if err := s.Start("p1"); err != nil {
		t.Fatal(err)
	}
	return s
}

func meeplesOK(t *testing.T, s *State) {
	t.Helper()
	for _, p := range s.Players {
		on := 0
		for _, tile := range s.Tiles {
			if tile.Meeple != nil && tile.Meeple.PlayerID == p.ID {
				on++
			}
		}
		if on+p.Meeples != s.MeepleCount || p.Score < 0 {
			t.Fatalf("player %+v on board %d", p, on)
		}
	}
}

func TestCloseCity(t *testing.T) {
	s := newMatch(t)
	s.CurrentID = "city1"
	if err := s.Place("p1", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Meeple("p1", KindCity, 0); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Score != 4 {
		t.Fatalf("score %d", s.Players[0].Score)
	}
	meeplesOK(t, s)
}

func TestJunctionEndsRoad(t *testing.T) {
	s := newMatch(t)
	s.CurrentID = "mon_road"
	if err := s.Place("p1", -1, 0, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.Meeple("p1", KindRoad, 0); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Meeples != 6 || s.Players[0].Score != 0 {
		t.Fatalf("open road %+v", s.Players[0])
	}
	s.CurrentID = "tee"
	if err := s.Place("p2", 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Pass("p2"); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Meeples != 7 || s.Players[0].Score != 3 {
		t.Fatalf("junction should close the road and return the thief: %+v", s.Players[0])
	}
	for _, tile := range s.Tiles {
		if tile.Meeple != nil {
			t.Fatalf("meeple stuck on %+v", tile)
		}
	}
}

func TestCannotPlaceMeepleInOccupiedCity(t *testing.T) {
	s := newMatch(t)
	s.CurrentID = "city3"
	if err := s.Place("p1", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Meeple("p1", KindCity, 0); err != nil {
		t.Fatal(err)
	}
	for _, def := range Catalog {
		for rot := 0; rot < 4; rot++ {
			for _, m := range legal(&State{Tiles: s.Tiles, Status: StatusPlace, CurrentID: def.ID}, def.ID) {
				for _, r := range m.Rots {
					if r != rot {
						continue
					}
					trial := cloneForTrial(s)
					trial.Status = StatusPlace
					trial.Turn = 1
					trial.CurrentID = def.ID
					if err := trial.Place("p2", m.X, m.Y, rot); err != nil {
						continue
					}
					for _, sp := range Spots(trial) {
						if sp.Kind != KindCity || !sp.Free {
							continue
						}
						if cityTouchesMeeple(trial, len(trial.Tiles)-1, sp.Index) {
							t.Fatalf("%s rot %d at %d,%d city %d joins an occupied city", def.ID, rot, m.X, m.Y, sp.Index)
						}
					}
				}
			}
		}
	}
}

func cloneForTrial(s *State) *State {
	out := *s
	out.Players = append([]Player(nil), s.Players...)
	out.Tiles = append([]Placed(nil), s.Tiles...)
	for i := range out.Tiles {
		if out.Tiles[i].Meeple != nil {
			mp := *out.Tiles[i].Meeple
			out.Tiles[i].Meeple = &mp
		}
	}
	return &out
}

func cityTouchesMeeple(s *State, tileIndex, group int) bool {
	_, meeples := walkCity(s, s.index(), s.Tiles[tileIndex].X, s.Tiles[tileIndex].Y, group)
	return len(meeples) > 0
}

func TestRoadBetweenMonasteries(t *testing.T) {
	s := newMatch(t)
	s.CurrentID = "mon_road"
	if err := s.Place("p1", -1, 0, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.Meeple("p1", KindRoad, 0); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Score != 0 || s.Players[0].Meeples != 6 {
		t.Fatalf("early %+v", s.Players[0])
	}
	s.CurrentID = "mon_road"
	if err := s.Place("p2", 1, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Pass("p2"); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Score != 3 || s.Players[0].Meeples != 7 {
		t.Fatalf("after %+v", s.Players[0])
	}
	meeplesOK(t, s)
}

func TestRecallAndTurn(t *testing.T) {
	s := newMatch(t)
	id := s.CurrentID
	moves := Legal(s)
	if len(moves) == 0 {
		t.Fatal("no moves")
	}
	m := moves[0]
	if err := s.Place("p1", m.X, m.Y, m.Rots[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.Rotate("p2", 1); err != ErrNotYourTurn {
		t.Fatalf("other player %v", err)
	}
	before := len(s.Tiles)
	rot := s.Tiles[before-1].Rot
	if err := s.Rotate("p1", 1); err != nil {
		t.Fatal(err)
	}
	if len(m.Rots) > 1 && s.Tiles[before-1].Rot == rot {
		t.Fatal("rotation did not change")
	}
	if err := s.Recall("p1"); err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusPlace || s.CurrentID != id || len(s.Tiles) != before-1 {
		t.Fatalf("after recall status=%s id=%s tiles=%d", s.Status, s.CurrentID, len(s.Tiles))
	}
	if err := s.Place("p1", m.X, m.Y, m.Rots[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.Pass("p1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Recall("p1"); err != ErrBadPhase {
		t.Fatalf("recall after pass %v", err)
	}
}

func TestRejectMismatchAndOccupied(t *testing.T) {
	s := newMatch(t)
	s.CurrentID = "road"
	if err := s.Place("p1", 0, 1, 0); err != ErrIllegalPlace {
		t.Fatalf("got %v", err)
	}
	s.CurrentID = "city3"
	if err := s.Place("p1", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Meeple("p1", KindCity, 0); err != nil {
		t.Fatal(err)
	}
	s.CurrentID = "city1"
	if err := s.Place("p2", 1, 1, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.Meeple("p2", KindCity, 0); err != ErrIllegalMeeple {
		t.Fatalf("occupied %v", err)
	}
	meeplesOK(t, s)
}

func TestTieAndShield(t *testing.T) {
	s := &State{
		Status: StatusPlace, MeepleCount: 7,
		Players: []Player{
			{ID: "p1", Name: "Аня", Meeples: 6},
			{ID: "p2", Name: "Боря", Meeples: 6},
		},
		Tiles: []Placed{
			{X: 0, Y: 0, TileID: "city1", Rot: 1, Meeple: &Meeple{PlayerID: "p1", Kind: KindCity, Index: 0}},
			{X: 1, Y: 0, TileID: "city1", Rot: 3, Meeple: &Meeple{PlayerID: "p2", Kind: KindCity, Index: 0}},
		},
	}
	s.resolve(false)
	if s.Players[0].Score != 4 || s.Players[1].Score != 4 {
		t.Fatalf("tie %d %d", s.Players[0].Score, s.Players[1].Score)
	}

	s = &State{
		Status: StatusPlace, MeepleCount: 7,
		Players: []Player{{ID: "p1", Name: "Аня", Meeples: 6}},
		Tiles: []Placed{
			{X: 0, Y: 0, TileID: "bridge_s", Meeple: &Meeple{PlayerID: "p1", Kind: KindCity, Index: 0}},
			{X: 0, Y: 1, TileID: "city1", Rot: 2},
			{X: 0, Y: -1, TileID: "city1", Rot: 0},
		},
	}
	s.resolve(false)
	if s.Players[0].Score != 8 {
		t.Fatalf("shield city %d", s.Players[0].Score)
	}
}

func TestMonasteryAndField(t *testing.T) {
	s := &State{
		Status: StatusMeeple, MeepleCount: 7,
		Players: []Player{{ID: "p1", Name: "Аня", Meeples: 5}},
		Tiles: []Placed{
			{X: 0, Y: 0, TileID: "mon", Meeple: &Meeple{PlayerID: "p1", Kind: KindMonastery}},
		},
	}
	for _, d := range [][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}} {
		s.Tiles = append(s.Tiles, Placed{X: d[0], Y: d[1], TileID: "mon"})
	}
	s.resolve(false)
	if s.Players[0].Score != 0 {
		t.Fatalf("early monastery %d", s.Players[0].Score)
	}
	s.Tiles = append(s.Tiles, Placed{X: 1, Y: 1, TileID: "mon"})
	s.resolve(false)
	if s.Players[0].Score != 9 || s.Players[0].Meeples != 6 {
		t.Fatalf("monastery %+v", s.Players[0])
	}

	s = &State{
		Status: StatusPlace, MeepleCount: 7,
		Players: []Player{{ID: "p1", Name: "Аня", Meeples: 6}},
		Tiles: []Placed{
			{X: 0, Y: 0, TileID: "city1", Rot: 0, Meeple: &Meeple{PlayerID: "p1", Kind: KindField, Index: 0}},
			{X: 0, Y: 1, TileID: "city1", Rot: 2},
		},
	}
	s.resolve(true)
	if s.Players[0].Score != 3 {
		t.Fatalf("field %d", s.Players[0].Score)
	}
}

func TestExpireSkipsThenRemoves(t *testing.T) {
	s := newMatch(t)
	if s.Deadline.IsZero() || time.Until(s.Deadline) < 30*time.Second {
		t.Fatalf("clock %s", s.Deadline)
	}
	first := s.current().ID
	s.Deadline = time.Now().Add(-time.Second)
	if _, ok := s.Expire(time.Now()); !ok {
		t.Fatal("expected skip")
	}
	if s.current().ID == first {
		t.Fatal("turn did not move")
	}
	var skips int
	for _, p := range s.Players {
		if p.ID == first {
			skips = p.Skips
		}
	}
	if skips != 1 {
		t.Fatalf("skips %d", skips)
	}
	s.Turn = 0
	for i := range s.Players {
		if s.Players[i].ID == first {
			s.Turn = i
			s.Players[i].Skips = SkipLimit - 1
		}
	}
	s.Status = StatusPlace
	s.Deadline = time.Now().Add(-time.Second)
	banks, ok := s.Expire(time.Now())
	if !ok || s.Status != StatusDone {
		t.Fatalf("kick ok=%v status=%s", ok, s.Status)
	}
	for _, b := range banks {
		if b.ID == first && b.Score < 0 {
			t.Fatal(b)
		}
	}
}

func TestJoinGoesLast(t *testing.T) {
	s := newMatch(t)
	current := s.current().ID
	if err := s.AddPlayer("p3", "Вера"); err != nil {
		t.Fatal(err)
	}
	if s.current().ID != current {
		t.Fatalf("turn moved to %s", s.current().ID)
	}
	if s.Players[len(s.Players)-1].ID == "p3" && s.Turn == 0 {
		t.Fatal("newcomer took the front of the queue")
	}
	seenCurrent := false
	for i := 0; i < len(s.Players); i++ {
		id := s.Players[(s.Turn+i)%len(s.Players)].ID
		if id == "p3" && !seenCurrent {
			t.Fatal("newcomer is ahead of the current player")
		}
		if id == current {
			seenCurrent = true
		}
		if id == "p3" && i != len(s.Players)-1 {
			t.Fatalf("newcomer plays at offset %d", i)
		}
	}
}

func TestLeaveScoresWhenTableEmpties(t *testing.T) {
	s := newMatch(t)
	s.CurrentID = "city3"
	if err := s.Place("p1", 0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Meeple("p1", KindCity, 0); err != nil {
		t.Fatal(err)
	}
	if s.Players[0].Score != 0 {
		t.Fatalf("should stay open, score %d", s.Players[0].Score)
	}
	banks, err := s.Leave("p2")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusDone {
		t.Fatal("expected the table to close when one player remains")
	}
	got := 0
	for _, b := range banks {
		if b.ID == "p1" {
			got = b.Score
		}
	}
	if got != 2 {
		t.Fatalf("final city %d", got)
	}
}

func TestRoadLoopClosesField(t *testing.T) {
	// 3x3 ring: bends in the corners, straight roads on the sides, farmer in the middle.
	s := &State{
		Status: StatusPlace, MeepleCount: 7,
		Players: []Player{{ID: "p1", Name: "Аня", Meeples: 6}},
		Tiles: []Placed{
			{X: 0, Y: 0, TileID: "bend", Rot: 0},
			{X: 2, Y: 0, TileID: "bend", Rot: 3},
			{X: 0, Y: 2, TileID: "bend", Rot: 1},
			{X: 2, Y: 2, TileID: "bend", Rot: 2},
			{X: 1, Y: 0, TileID: "road", Rot: 1},
			{X: 1, Y: 2, TileID: "road", Rot: 1},
			{X: 0, Y: 1, TileID: "road", Rot: 0},
			{X: 2, Y: 1, TileID: "road", Rot: 0},
			{X: 1, Y: 1, TileID: "mon", Meeple: &Meeple{PlayerID: "p1", Kind: KindField, Index: 0}},
		},
	}
	idx := s.index()
	_, meeples, _, open := walkField(s, idx, nil, 1, 1, 0)
	t.Logf("middle field open=%d meeples=%d", open, len(meeples))
	s.resolve(false)
	for _, tile := range s.Tiles {
		if tile.Meeple != nil && tile.Meeple.Kind == KindField {
			t.Fatalf("farmer stayed at (%d,%d) open was %d", tile.X, tile.Y, open)
		}
	}
}

func TestClosedFieldScoresImmediately(t *testing.T) {
	s := &State{
		Status: StatusPlace, MeepleCount: 7,
		Players: []Player{{ID: "p1", Name: "Аня", Meeples: 6}},
		Tiles: []Placed{
			{X: 0, Y: 0, TileID: "city2_road", Meeple: &Meeple{PlayerID: "p1", Kind: KindField, Index: 1}},
		},
	}
	s.resolve(false)
	if s.Tiles[0].Meeple == nil || s.Players[0].Meeples != 6 {
		t.Fatal("open field returned the farmer")
	}
	s.Tiles = append(s.Tiles,
		Placed{X: 1, Y: 0, TileID: "city1", Rot: 3},
		Placed{X: 0, Y: -1, TileID: "city1", Rot: 0},
	)
	s.resolve(false)
	if s.Tiles[0].Meeple != nil || s.Players[0].Meeples != 7 {
		t.Fatalf("closed field %+v meeples %d", s.Tiles[0].Meeple, s.Players[0].Meeples)
	}

	s = &State{
		Status: StatusPlace, MeepleCount: 7,
		Players: []Player{{ID: "p1", Name: "Аня", Meeples: 6}},
		Tiles: []Placed{
			{X: 0, Y: 0, TileID: "city3", Meeple: &Meeple{PlayerID: "p1", Kind: KindField, Index: 0}},
			{X: 0, Y: -1, TileID: "city1", Rot: 0},
		},
	}
	s.resolve(false)
	if s.Tiles[0].Meeple != nil || s.Players[0].Meeples != 7 || s.Players[0].Score != 3 {
		t.Fatalf("walled field %+v score %d meeples %d", s.Tiles[0].Meeple, s.Players[0].Score, s.Players[0].Meeples)
	}
}

func TestFieldSplitByRoad(t *testing.T) {
	s := &State{Tiles: []Placed{
		{X: 0, Y: 0, TileID: "road", Meeple: &Meeple{PlayerID: "p1", Kind: KindField, Index: 0}},
		{X: 0, Y: 1, TileID: "road"},
	}}
	idx := s.index()
	tiles, meeples, _, _ := walkField(s, idx, nil, 0, 0, 0)
	if len(tiles) != 2 || len(meeples) != 1 {
		t.Fatalf("east field tiles=%d meeples=%d", len(tiles), len(meeples))
	}
	_, meeples, _, _ = walkField(s, idx, nil, 0, 0, 1)
	if len(meeples) != 0 {
		t.Fatal("west field stole the farmer")
	}
}

func TestRandomPlayout(t *testing.T) {
	s := newMatch(t)
	r := rand.New(rand.NewPCG(7, 9))
	for step := 0; step < 50 && s.Status != StatusDone; step++ {
		id := s.current().ID
		switch s.Status {
		case StatusPlace:
			moves := Legal(s)
			if len(moves) == 0 {
				t.Fatal("no legal tile")
			}
			m := moves[r.IntN(len(moves))]
			if err := s.Place(id, m.X, m.Y, m.Rots[r.IntN(len(m.Rots))]); err != nil {
				t.Fatal(err)
			}
		case StatusMeeple:
			var free []Spot
			for _, sp := range Spots(s) {
				if sp.Free {
					free = append(free, sp)
				}
			}
			if len(free) > 0 && r.IntN(3) == 0 {
				sp := free[r.IntN(len(free))]
				if err := s.Meeple(id, sp.Kind, sp.Index); err != nil {
					t.Fatal(err)
				}
			} else if err := s.Pass(id); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("status %s", s.Status)
		}
		meeplesOK(t, s)
	}
}
