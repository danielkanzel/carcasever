package game

import (
	"math/rand/v2"
	"testing"
	"time"
)

func lobby(t *testing.T) *State {
	t.Helper()
	s := &State{
		ID: "g", HostID: "p1", Open: true,
		Status: StatusLobby, MaxPlayers: 5, MeepleCount: 7,
	}
	s.UseRand(rand.New(rand.NewPCG(3, 4)))
	if err := s.AddPlayer("p1", "Аня"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBotPlaysTurn(t *testing.T) {
	s := lobby(t)
	if err := s.AddBot("p2", BotEasy); err == nil {
		t.Fatal("guest added a bot")
	}
	if err := s.AddBot("p1", BotEasy); err != nil {
		t.Fatal(err)
	}
	if err := s.Start("p1"); err != nil {
		t.Fatal(err)
	}
	move := Legal(s)[0]
	if err := s.Place("p1", move.X, move.Y, move.Rots[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.Pass("p1"); err != nil {
		t.Fatal(err)
	}
	if s.current() == nil || s.current().Bot == 0 {
		t.Fatal("bot should be on turn")
	}
	if _, ok := s.Expire(s.Deadline.Add(time.Second)); !ok {
		t.Fatal("bot did not place")
	}
	if s.Status == StatusMeeple && s.current() != nil && s.current().Bot > 0 {
		if _, ok := s.Expire(s.Deadline.Add(time.Second)); !ok {
			t.Fatal("bot did not finish the meeple")
		}
	}
	if s.Status == StatusPlace && s.current() != nil && s.current().ID != "p1" && s.current().Bot == 0 {
		t.Fatalf("turn %#v", s.current())
	}
}

func TestBotsPlaceMeeples(t *testing.T) {
	for _, level := range []int{BotEasy, BotMedium, BotHard} {
		s := lobby(t)
		s.UseRand(rand.New(rand.NewPCG(uint64(level)*17, 9)))
		if err := s.AddBot("p1", level); err != nil {
			t.Fatal(err)
		}
		if err := s.Start("p1"); err != nil {
			t.Fatal(err)
		}
		placed := 0
		botID := ""
		for _, p := range s.Players {
			if p.Bot == level {
				botID = p.ID
			}
		}
		for step := 0; step < 40 && placed < 2; step++ {
			if s.Status == StatusDone {
				break
			}
			cur := s.current()
			if cur == nil {
				t.Fatal("no current")
			}
			before := cur.Meeples
			id := cur.ID
			if cur.Bot == 0 {
				if s.Status == StatusPlace {
					move := Legal(s)[0]
					if err := s.Place(id, move.X, move.Y, move.Rots[0]); err != nil {
						t.Fatal(err)
					}
				} else if err := s.Pass(id); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if _, ok := s.Expire(s.Deadline.Add(time.Second)); !ok {
				t.Fatalf("level %d bot did not act", level)
			}
			for _, p := range s.Players {
				if p.ID == id && p.Meeples < before {
					placed++
				}
			}
		}
		if placed == 0 {
			left := 0
			for _, p := range s.Players {
				if p.ID == botID {
					left = p.Meeples
				}
			}
			t.Fatalf("level %d placed no meeples, left %d", level, left)
		}
	}
}

func TestOnlyBotsEndsMatch(t *testing.T) {
	s := lobby(t)
	if err := s.AddPlayer("p2", "Боря"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddBot("p1", BotMedium); err != nil {
		t.Fatal(err)
	}
	if err := s.Start("p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Leave("p2"); err != nil {
		t.Fatal(err)
	}
	if s.Status == StatusDone {
		t.Fatal("a human is still at the table")
	}

	s = lobby(t)
	if err := s.AddBot("p1", BotMedium); err != nil || s.AddBot("p1", BotHard) != nil {
		t.Fatal("add bots")
	}
	if err := s.Start("p1"); err != nil {
		t.Fatal(err)
	}
	for i := range s.Players {
		s.Players[i].Score = 6
	}
	banks, err := s.Leave("p1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusDone {
		t.Fatalf("status %s", s.Status)
	}
	sawHuman := false
	for _, b := range banks {
		if len(b.ID) >= 4 && b.ID[:4] == "bot-" {
			t.Fatalf("bot banked %#v", b)
		}
		if b.ID == "p1" {
			sawHuman = true
		}
	}
	if !sawHuman {
		t.Fatalf("human score missing %#v", banks)
	}
}
