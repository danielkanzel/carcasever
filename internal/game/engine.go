package game

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

const (
	StatusLobby  = "lobby"
	StatusPlace  = "place"
	StatusMeeple = "meeple"
	StatusDone   = "done"

	TurnLimit = 40 * time.Second
	SkipLimit = 5
	BotPause  = 900 * time.Millisecond

	BotEasy   = 1
	BotMedium = 2
	BotHard   = 3
)

var (
	ErrNotHost       = errors.New("not_host")
	ErrNotYourTurn   = errors.New("not_your_turn")
	ErrBadPhase      = errors.New("bad_phase")
	ErrNeedPlayers   = errors.New("need_players")
	ErrFull          = errors.New("game_full")
	ErrStarted       = errors.New("game_started")
	ErrPlaying       = errors.New("still_playing")
	ErrIllegalPlace  = errors.New("illegal_place")
	ErrIllegalMeeple = errors.New("illegal_meeple")
	ErrNoMeeples     = errors.New("no_meeples")
	ErrNotInGame     = errors.New("not_in_game")
	ErrBadSettings   = errors.New("bad_settings")
	ErrClosed        = errors.New("game_closed")
	ErrNotFound      = errors.New("not_found")
	ErrAFK           = errors.New("not_afk")
)

var palette = []string{"#c23b3b", "#2c6bed", "#2e8b4a", "#e0a825", "#222222", "#d45d9d"}

var deltas = [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}

type Meeple struct {
	PlayerID string `json:"playerId" bson:"playerId"`
	Kind     string `json:"kind" bson:"kind"`
	Index    int    `json:"index" bson:"index"`
}

type Player struct {
	ID        string `json:"id" bson:"id"`
	Name      string `json:"name" bson:"name"`
	Color     string `json:"color" bson:"color"`
	Meeples   int    `json:"meeples" bson:"meeples"`
	Score     int    `json:"score" bson:"score"`
	Vote      bool   `json:"vote" bson:"vote"`
	Connected bool   `json:"connected" bson:"connected"`
	Skips     int    `json:"skips" bson:"skips"`
	Bot       int    `json:"bot,omitempty" bson:"bot,omitempty"`
}

type Placed struct {
	X      int     `json:"x" bson:"x"`
	Y      int     `json:"y" bson:"y"`
	TileID string  `json:"id" bson:"tileId"`
	Rot    int     `json:"rot" bson:"rot"`
	Meeple *Meeple `json:"meeple,omitempty" bson:"meeple,omitempty"`
}

type Event struct {
	Kind    string `json:"kind" bson:"kind"`
	Name    string `json:"name,omitempty" bson:"name,omitempty"`
	Feature string `json:"feature,omitempty" bson:"feature,omitempty"`
	Points  int    `json:"points,omitempty" bson:"points,omitempty"`
	N       int    `json:"n,omitempty" bson:"n,omitempty"`
}

type State struct {
	ID          string     `json:"id" bson:"_id"`
	Code        string     `json:"code" bson:"code"`
	HostID      string     `json:"hostId" bson:"hostId"`
	Open        bool       `json:"open" bson:"open"`
	Status      string     `json:"status" bson:"status"`
	MaxPlayers  int        `json:"maxPlayers" bson:"maxPlayers"`
	MeepleCount int        `json:"meepleCount" bson:"meepleCount"`
	Players     []Player   `json:"players" bson:"players"`
	Tiles       []Placed   `json:"tiles" bson:"tiles"`
	CurrentID   string     `json:"currentId,omitempty" bson:"currentId,omitempty"`
	Turn        int        `json:"turn" bson:"turn"`
	Events      []Event    `json:"events" bson:"events"`
	Chat        []ChatLine `json:"chat,omitempty" bson:"chat,omitempty"`
	Winners     []string   `json:"winners,omitempty" bson:"winners,omitempty"`
	LastX       int        `json:"lastX" bson:"lastX"`
	LastY       int        `json:"lastY" bson:"lastY"`
	HasLast     bool       `json:"hasLast" bson:"hasLast"`
	CreatedAt   time.Time  `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt" bson:"updatedAt"`
	Deadline    time.Time  `json:"deadline,omitempty" bson:"deadline,omitempty"`

	rng *rand.Rand
}

func (s *State) UseRand(r *rand.Rand) { s.rng = r }

func (s *State) rand() *rand.Rand {
	if s.rng == nil {
		s.rng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0xC0FFEE))
	}
	return s.rng
}

func (p Placed) view() View {
	d, ok := Def(p.TileID)
	if !ok {
		return View{}
	}
	return d.View(p.Rot)
}

func (s *State) playerIndex(id string) int {
	for i, p := range s.Players {
		if p.ID == id {
			return i
		}
	}
	return -1
}

func (s *State) CanInk(playerID string) error {
	if s.playerIndex(playerID) < 0 {
		return ErrNotInGame
	}
	if s.Status != StatusPlace && s.Status != StatusMeeple {
		return ErrBadPhase
	}
	if cur := s.current(); cur != nil && cur.ID == playerID {
		return ErrNotYourTurn
	}
	return nil
}

func (s *State) current() *Player {
	if s.Turn < 0 || s.Turn >= len(s.Players) {
		return nil
	}
	return &s.Players[s.Turn]
}

func (s *State) push(ev Event) {
	s.Events = append(s.Events, ev)
	if len(s.Events) > 30 {
		s.Events = s.Events[len(s.Events)-30:]
	}
}

func nextColor(ps []Player) string {
	used := map[string]bool{}
	for _, p := range ps {
		used[p.Color] = true
	}
	for _, c := range palette {
		if !used[c] {
			return c
		}
	}
	return palette[len(ps)%len(palette)]
}

func (s *State) AddPlayer(id, name string) error {
	if s.Status == StatusDone {
		return ErrStarted
	}
	if s.playerIndex(id) >= 0 {
		return nil
	}
	if len(s.Players) >= s.MaxPlayers {
		return ErrFull
	}
	p := Player{
		ID: id, Name: name, Color: nextColor(s.Players),
		Meeples: s.MeepleCount, Connected: true,
	}
	if s.Status == StatusLobby || len(s.Players) == 0 {
		s.Players = append(s.Players, p)
	} else {
		at := s.Turn
		if at < 0 || at > len(s.Players) {
			at = len(s.Players)
		}
		if at == len(s.Players) {
			s.Players = append(s.Players, p)
		} else {
			s.Players = append(s.Players, Player{})
			copy(s.Players[at+1:], s.Players[at:])
			s.Players[at] = p
			s.Turn++
		}
	}
	s.push(Event{Kind: "join", Name: name})
	return nil
}

func (s *State) Rename(id, name string) {
	if i := s.playerIndex(id); i >= 0 {
		s.Players[i].Name = name
	}
}

func (s *State) SetConnected(id string, on bool) {
	if i := s.playerIndex(id); i >= 0 {
		s.Players[i].Connected = on
	}
}

type Bank struct {
	ID    string
	Name  string
	Score int
}

func (s *State) Leave(id string) ([]Bank, error) {
	i := s.playerIndex(id)
	if i < 0 {
		return nil, ErrNotInGame
	}
	if s.Status == StatusDone {
		return nil, nil
	}
	active := s.Status == StatusPlace || s.Status == StatusMeeple
	humansLeft := 0
	for _, p := range s.Players {
		if p.ID != id && p.Bot == 0 {
			humansLeft++
		}
	}
	ending := (active && len(s.Players) <= 2) || (humansLeft == 0 && (active || len(s.Players) > 1))
	if active && !ending && i == s.Turn && s.Status == StatusMeeple {
		s.resolve(false)
	}
	if ending {
		s.finalize()
	}
	var banks []Bank
	if ending {
		for _, p := range s.Players {
			if p.Bot == 0 && p.Score > 0 {
				banks = append(banks, Bank{ID: p.ID, Name: p.Name, Score: p.Score})
			}
		}
	} else if active && s.Players[i].Bot == 0 && s.Players[i].Score > 0 {
		banks = append(banks, Bank{ID: id, Name: s.Players[i].Name, Score: s.Players[i].Score})
	}
	if active && !ending {
		s.releaseMeeples(id)
	}
	name := s.Players[i].Name
	wasTurn := i == s.Turn
	s.Players = append(s.Players[:i], s.Players[i+1:]...)
	s.push(Event{Kind: "leave", Name: name})
	if s.HostID == id && len(s.Players) > 0 {
		s.HostID = s.Players[0].ID
		for _, p := range s.Players {
			if p.Bot == 0 {
				s.HostID = p.ID
				break
			}
		}
	}
	if ending || len(s.Players) == 0 {
		s.Status = StatusDone
		s.CurrentID = ""
		return banks, nil
	}
	if !active {
		return banks, nil
	}
	if i < s.Turn {
		s.Turn--
	} else if wasTurn {
		if s.Turn >= len(s.Players) {
			s.Turn = 0
		}
		s.CurrentID = ""
		s.Status = StatusPlace
		s.draw()
	}
	return banks, nil
}

func (s *State) releaseMeeples(id string) {
	for ti := range s.Tiles {
		if s.Tiles[ti].Meeple != nil && s.Tiles[ti].Meeple.PlayerID == id {
			s.Tiles[ti].Meeple = nil
		}
	}
}

func (s *State) Kick(host, target string) error {
	if s.HostID != host {
		return ErrNotHost
	}
	if host == target {
		return ErrBadPhase
	}
	i := s.playerIndex(target)
	if i < 0 {
		return ErrNotInGame
	}
	if s.Status != StatusLobby {
		if s.Players[i].Bot == 0 {
			return ErrBadPhase
		}
		_, err := s.Leave(target)
		return err
	}
	name := s.Players[i].Name
	s.Players = append(s.Players[:i], s.Players[i+1:]...)
	s.push(Event{Kind: "kick", Name: name})
	return nil
}

func (s *State) ApplySettings(host string, open bool, maxPlayers, meeples int) error {
	if s.HostID != host {
		return ErrNotHost
	}
	if s.Status != StatusLobby {
		return ErrBadPhase
	}
	if maxPlayers < 2 || maxPlayers > 6 || meeples < 5 || meeples > 20 {
		return ErrBadSettings
	}
	if maxPlayers < len(s.Players) {
		return ErrBadSettings
	}
	s.Open = open
	s.MaxPlayers = maxPlayers
	s.MeepleCount = meeples
	for i := range s.Players {
		s.Players[i].Meeples = meeples
	}
	return nil
}

func (s *State) Start(host string) error {
	if s.HostID != host {
		return ErrNotHost
	}
	if s.Status != StatusLobby {
		return ErrBadPhase
	}
	if len(s.Players) < 2 {
		return ErrNeedPlayers
	}
	s.Tiles = []Placed{{X: 0, Y: 0, TileID: StartTileID, Rot: 0}}
	s.HasLast = true
	s.LastX, s.LastY = 0, 0
	s.Turn = 0
	s.Status = StatusPlace
	for i := range s.Players {
		s.Players[i].Score = 0
		s.Players[i].Vote = false
		s.Players[i].Meeples = s.MeepleCount
		s.Players[i].Connected = true
	}
	s.push(Event{Kind: "start"})
	s.draw()
	return nil
}

func (s *State) draw() {
	r := s.rand()
	total := 0
	for _, d := range Catalog {
		total += d.Weight
	}
	for try := 0; try < 48; try++ {
		pick := r.IntN(total)
		id := Catalog[0].ID
		for _, d := range Catalog {
			pick -= d.Weight
			if pick < 0 {
				id = d.ID
				break
			}
		}
		if len(legal(s, id)) > 0 {
			s.CurrentID = id
			s.Status = StatusPlace
			s.armClock()
			return
		}
	}
	for _, d := range Catalog {
		if len(legal(s, d.ID)) > 0 {
			s.CurrentID = d.ID
			s.Status = StatusPlace
			s.armClock()
			return
		}
	}
	s.Deadline = time.Time{}
	s.finalize()
}

func (s *State) armClock() {
	if s.Status != StatusPlace && s.Status != StatusMeeple {
		s.Deadline = time.Time{}
		return
	}
	wait := TurnLimit
	if cur := s.current(); cur != nil && cur.Bot > 0 {
		wait = BotPause
	}
	s.Deadline = time.Now().Add(wait)
}

func (s *State) Expire(now time.Time) ([]Bank, bool) {
	if s.Deadline.IsZero() || now.Before(s.Deadline) {
		return nil, false
	}
	if s.Status != StatusPlace && s.Status != StatusMeeple {
		return nil, false
	}
	cur := s.current()
	if cur == nil {
		return nil, false
	}
	if cur.Bot > 0 {
		if err := s.actBot(); err != nil {
			return nil, false
		}
		return nil, true
	}
	cur.Skips++
	if cur.Skips >= SkipLimit {
		s.push(Event{Kind: "idle", Name: cur.Name})
		banks, err := s.Leave(cur.ID)
		return banks, err == nil
	}
	s.push(Event{Kind: "skip", Name: cur.Name})
	if s.Status == StatusMeeple {
		s.finishTurn()
		return nil, true
	}
	s.CurrentID = ""
	if len(s.Players) == 0 {
		return nil, true
	}
	s.Turn = (s.Turn + 1) % len(s.Players)
	s.draw()
	return nil, true
}

func (s *State) index() map[[2]int]int {
	m := make(map[[2]int]int, len(s.Tiles))
	for i, t := range s.Tiles {
		m[[2]int{t.X, t.Y}] = i
	}
	return m
}

type Move struct {
	X    int   `json:"x"`
	Y    int   `json:"y"`
	Rots []int `json:"rots"`
}

func Legal(s *State) []Move {
	if s.Status != StatusPlace || s.CurrentID == "" {
		return nil
	}
	return legal(s, s.CurrentID)
}

func legal(s *State, tileID string) []Move {
	d, ok := Def(tileID)
	if !ok {
		return nil
	}
	idx := s.index()
	if len(idx) == 0 {
		return []Move{{X: 0, Y: 0, Rots: []int{0}}}
	}
	cands := map[[2]int][]int{}
	for pos := range idx {
		for _, dlt := range deltas {
			nx, ny := pos[0]+dlt[0], pos[1]+dlt[1]
			if _, occupied := idx[[2]int{nx, ny}]; occupied {
				continue
			}
			if nx > 500 || nx < -500 || ny > 500 || ny < -500 {
				continue
			}
			key := [2]int{nx, ny}
			if _, seen := cands[key]; seen {
				continue
			}
			var rots []int
			for rot := 0; rot < 4; rot++ {
				if fits(idx, s, nx, ny, d.View(rot)) {
					rots = append(rots, rot)
				}
			}
			if len(rots) > 0 {
				cands[key] = rots
			}
		}
	}
	out := make([]Move, 0, len(cands))
	for pos, rots := range cands {
		out = append(out, Move{X: pos[0], Y: pos[1], Rots: rots})
	}
	return out
}

func fits(idx map[[2]int]int, s *State, x, y int, v View) bool {
	touch := false
	for e, dlt := range deltas {
		ti, ok := idx[[2]int{x + dlt[0], y + dlt[1]}]
		if !ok {
			continue
		}
		touch = true
		ov := s.Tiles[ti].view()
		if ov.Edges[(e+2)%4] != v.Edges[e] {
			return false
		}
	}
	return touch
}

func (s *State) Place(playerID string, x, y, rot int) error {
	if s.Status != StatusPlace {
		return ErrBadPhase
	}
	cur := s.current()
	if cur == nil || cur.ID != playerID {
		return ErrNotYourTurn
	}
	okRot := false
	for _, m := range legal(s, s.CurrentID) {
		if m.X == x && m.Y == y {
			for _, r := range m.Rots {
				if r == rot {
					okRot = true
				}
			}
		}
	}
	if !okRot {
		return ErrIllegalPlace
	}
	s.Tiles = append(s.Tiles, Placed{X: x, Y: y, TileID: s.CurrentID, Rot: rot})
	s.LastX, s.LastY, s.HasLast = x, y, true
	s.push(Event{Kind: "place", Name: cur.Name})
	cur.Skips = 0
	s.Status = StatusMeeple
	s.CurrentID = ""
	s.armClock()
	s.closeFields()
	return nil
}

func (s *State) closeFields() {
	if s.Status == StatusDone {
		return
	}
	idx := s.index()
	s.scoreFields(idx, collectCities(s, idx), false)
}

type Spot struct {
	Kind  string `json:"kind"`
	Index int    `json:"index"`
	Free  bool   `json:"free"`
}

func Spots(s *State) []Spot {
	if s.Status != StatusMeeple || len(s.Tiles) == 0 {
		return nil
	}
	last := s.Tiles[len(s.Tiles)-1]
	v := last.view()
	cur := s.current()
	has := cur != nil && cur.Meeples > 0
	var spots []Spot
	for i := range v.Cities {
		spots = append(spots, Spot{Kind: KindCity, Index: i, Free: has && !occupied(s, len(s.Tiles)-1, KindCity, i)})
	}
	for i := range v.Roads {
		spots = append(spots, Spot{Kind: KindRoad, Index: i, Free: has && !occupied(s, len(s.Tiles)-1, KindRoad, i)})
	}
	for i := range v.Fields {
		_, _, _, open := walkField(s, s.index(), nil, last.X, last.Y, i)
		spots = append(spots, Spot{Kind: KindField, Index: i, Free: has && open > 0 && !occupied(s, len(s.Tiles)-1, KindField, i)})
	}
	if v.Monastery {
		spots = append(spots, Spot{Kind: KindMonastery, Index: 0, Free: has && !occupied(s, len(s.Tiles)-1, KindMonastery, 0)})
	}
	return spots
}

func occupied(s *State, tileIndex int, kind string, group int) bool {
	switch kind {
	case KindMonastery:
		return s.Tiles[tileIndex].Meeple != nil
	case KindCity:
		_, meeples := walkCity(s, s.index(), s.Tiles[tileIndex].X, s.Tiles[tileIndex].Y, group)
		return len(meeples) > 0
	case KindRoad:
		_, meeples := walkRoad(s, s.index(), s.Tiles[tileIndex].X, s.Tiles[tileIndex].Y, group)
		return len(meeples) > 0
	case KindField:
		_, meeples, _, _ := walkField(s, s.index(), nil, s.Tiles[tileIndex].X, s.Tiles[tileIndex].Y, group)
		return len(meeples) > 0
	default:
		return true
	}
}

func (s *State) Meeple(playerID, kind string, index int) error {
	if s.Status != StatusMeeple {
		return ErrBadPhase
	}
	cur := s.current()
	if cur == nil || cur.ID != playerID {
		return ErrNotYourTurn
	}
	if cur.Meeples <= 0 {
		return ErrNoMeeples
	}
	free := false
	for _, sp := range Spots(s) {
		if sp.Kind == kind && sp.Index == index && sp.Free {
			free = true
		}
	}
	if !free {
		return ErrIllegalMeeple
	}
	ti := len(s.Tiles) - 1
	s.Tiles[ti].Meeple = &Meeple{PlayerID: playerID, Kind: kind, Index: index}
	s.Players[s.Turn].Meeples--
	s.Players[s.Turn].Skips = 0
	s.finishTurn()
	return nil
}

func (s *State) Pass(playerID string) error {
	if s.Status != StatusMeeple {
		return ErrBadPhase
	}
	cur := s.current()
	if cur == nil || cur.ID != playerID {
		return ErrNotYourTurn
	}
	cur.Skips = 0
	s.finishTurn()
	return nil
}

func (s *State) pending(playerID string) error {
	if s.Status != StatusMeeple || len(s.Tiles) == 0 {
		return ErrBadPhase
	}
	cur := s.current()
	if cur == nil || cur.ID != playerID {
		return ErrNotYourTurn
	}
	if s.Tiles[len(s.Tiles)-1].Meeple != nil {
		return ErrBadPhase
	}
	return nil
}

func (s *State) rotsAt(x, y int, tileID string) []int {
	d, ok := Def(tileID)
	if !ok {
		return nil
	}
	idx := s.index()
	delete(idx, [2]int{x, y})
	var rots []int
	for rot := 0; rot < 4; rot++ {
		if fits(idx, s, x, y, d.View(rot)) {
			rots = append(rots, rot)
		}
	}
	return rots
}

func (s *State) Rotate(playerID string, dir int) error {
	if err := s.pending(playerID); err != nil {
		return err
	}
	if dir >= 0 {
		dir = 1
	} else {
		dir = 3
	}
	last := &s.Tiles[len(s.Tiles)-1]
	rots := s.rotsAt(last.X, last.Y, last.TileID)
	allowed := map[int]bool{}
	for _, r := range rots {
		allowed[r] = true
	}
	for step := 1; step <= 3; step++ {
		cand := (last.Rot + dir*step) % 4
		if allowed[cand] {
			last.Rot = cand
			s.armClock()
			return nil
		}
	}
	s.armClock()
	return nil
}

func (s *State) Recall(playerID string) error {
	if err := s.pending(playerID); err != nil {
		return err
	}
	last := s.Tiles[len(s.Tiles)-1]
	s.Tiles = s.Tiles[:len(s.Tiles)-1]
	s.CurrentID = last.TileID
	s.Status = StatusPlace
	if n := len(s.Tiles); n > 0 {
		prev := s.Tiles[n-1]
		s.LastX, s.LastY, s.HasLast = prev.X, prev.Y, true
	} else {
		s.HasLast = false
	}
	s.armClock()
	return nil
}

func (s *State) Vote(playerID string, on bool) error {
	if s.Status != StatusPlace && s.Status != StatusMeeple {
		return ErrBadPhase
	}
	i := s.playerIndex(playerID)
	if i < 0 {
		return ErrNotInGame
	}
	s.Players[i].Vote = on
	s.push(Event{Kind: "vote", Name: s.Players[i].Name, N: b2i(on)})
	return nil
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *State) FinishPartial(playerID string) error {
	if s.Status != StatusPlace && s.Status != StatusMeeple {
		return ErrBadPhase
	}
	if s.playerIndex(playerID) < 0 || !s.Players[s.playerIndex(playerID)].Connected {
		return ErrNotInGame
	}
	disconnected := false
	for _, p := range s.Players {
		if p.Connected && !p.Vote {
			return ErrBadPhase
		}
		if !p.Connected {
			disconnected = true
		}
	}
	if !disconnected {
		return ErrBadPhase
	}
	return ErrBadPhase
}

func (s *State) SkipAFK(playerID string) error {
	if s.Status != StatusPlace && s.Status != StatusMeeple {
		return ErrBadPhase
	}
	if s.playerIndex(playerID) < 0 {
		return ErrNotInGame
	}
	cur := s.current()
	if cur == nil || cur.ID == playerID || cur.Connected {
		return ErrAFK
	}
	if s.Status == StatusPlace {
		moves := Legal(s)
		if len(moves) == 0 {
			s.finalize()
			return nil
		}
		best := moves[0]
		for _, m := range moves {
			if m.Y < best.Y || (m.Y == best.Y && m.X < best.X) {
				best = m
			}
		}
		rot := best.Rots[0]
		s.Tiles = append(s.Tiles, Placed{X: best.X, Y: best.Y, TileID: s.CurrentID, Rot: rot})
		s.LastX, s.LastY, s.HasLast = best.X, best.Y, true
		s.CurrentID = ""
		s.Status = StatusMeeple
		s.push(Event{Kind: "skip", Name: cur.Name})
	}
	s.finishTurn()
	return nil
}

func (s *State) finishTurn() {
	s.resolve(false)
	if s.Status == StatusDone || len(s.Players) == 0 {
		return
	}
	s.Turn = (s.Turn + 1) % len(s.Players)
	s.draw()
}

func (s *State) finalize() {
	if s.Status == StatusDone {
		return
	}
	s.resolve(true)
	s.Status = StatusDone
	s.CurrentID = ""
	s.Deadline = time.Time{}
	best := 0
	if len(s.Players) > 0 {
		best = s.Players[0].Score
		for _, p := range s.Players[1:] {
			if p.Score > best {
				best = p.Score
			}
		}
	}
	s.Winners = nil
	for _, p := range s.Players {
		if p.Score == best {
			s.Winners = append(s.Winners, p.ID)
		}
	}
	s.push(Event{Kind: "end"})
}

type claim struct {
	tile  int
	owner string
}

func (s *State) resolve(final bool) {
	if s.Status == StatusDone {
		return
	}
	idx := s.index()
	cities := collectCities(s, idx)
	for _, c := range cities {
		if len(c.meeples) == 0 {
			continue
		}
		complete := c.open == 0
		if !final && !complete {
			continue
		}
		pts := len(c.tiles) + c.shields
		if complete {
			pts = len(c.tiles)*2 + c.shields*2
		}
		s.award(c.meeples, pts, KindCity)
	}
	roads := collectRoads(s, idx)
	for _, rd := range roads {
		if len(rd.meeples) == 0 {
			continue
		}
		if !final && rd.open > 0 {
			continue
		}
		s.award(rd.meeples, len(rd.tiles), KindRoad)
	}
	for i, t := range s.Tiles {
		v := t.view()
		if !v.Monastery || t.Meeple == nil || t.Meeple.Kind != KindMonastery {
			continue
		}
		n := neighbors8(idx, t.X, t.Y)
		if !final && n < 8 {
			continue
		}
		s.award([]claim{{tile: i, owner: t.Meeple.PlayerID}}, 1+n, KindMonastery)
	}
	s.scoreFields(idx, cities, final)
}

func (s *State) scoreFields(idx map[[2]int]int, cities []feature, final bool) {
	cityAt := map[[3]int]string{}
	completeCity := map[string]bool{}
	for _, c := range cities {
		completeCity[c.id] = c.open == 0
		for _, e := range c.edges {
			cityAt[e] = c.id
		}
	}
	seenF := map[[3]int]bool{}
	for _, t := range s.Tiles {
		v := t.view()
		for g := range v.Fields {
			key := [3]int{t.X, t.Y, g}
			if seenF[key] {
				continue
			}
			_, meeples, visits, open := walkField(s, idx, &seenF, t.X, t.Y, g)
			if len(meeples) == 0 {
				continue
			}
			if !final && open > 0 {
				continue
			}
			adj := map[string]bool{}
			for _, vis := range visits {
				tv := s.Tiles[idx[[2]int{vis[0], vis[1]}]].view()
				for _, c := range tv.Fields[vis[2]] {
					for _, e := range cornerEdges(c) {
						if tv.Edges[e] == City {
							id := cityAt[[3]int{vis[0], vis[1], e}]
							if id != "" && (completeCity[id] || !final) {
								adj[id] = true
							}
						}
					}
				}
			}
			if len(adj) > 0 {
				s.award(meeples, 3*len(adj), KindField)
			} else {
				s.award(meeples, 0, KindField)
			}
		}
	}
}

func (s *State) award(meeples []claim, points int, feature string) {
	if len(meeples) == 0 {
		return
	}
	count := map[string]int{}
	order := []string{}
	seenTile := map[int]bool{}
	for _, m := range meeples {
		if seenTile[m.tile] {
			continue
		}
		seenTile[m.tile] = true
		if count[m.owner] == 0 {
			order = append(order, m.owner)
		}
		count[m.owner]++
	}
	best := 0
	for _, n := range count {
		if n > best {
			best = n
		}
	}
	if points > 0 {
		for _, id := range order {
			if count[id] != best {
				continue
			}
			if i := s.playerIndex(id); i >= 0 {
				s.Players[i].Score += points
				s.push(Event{Kind: "score", Name: s.Players[i].Name, Feature: feature, Points: points, N: count[id]})
			}
		}
	}
	for ti := range seenTile {
		mp := s.Tiles[ti].Meeple
		if mp == nil {
			continue
		}
		if i := s.playerIndex(mp.PlayerID); i >= 0 && s.Players[i].Meeples < s.MeepleCount {
			s.Players[i].Meeples++
		}
		s.Tiles[ti].Meeple = nil
	}
}

func neighbors8(idx map[[2]int]int, x, y int) int {
	n := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if _, ok := idx[[2]int{x + dx, y + dy}]; ok {
				n++
			}
		}
	}
	return n
}

type feature struct {
	tiles   []int
	shields int
	open    int
	meeples []claim
	id      string
	edges   [][3]int
}

func collectCities(s *State, idx map[[2]int]int) []feature {
	seen := map[[3]int]bool{}
	var out []feature
	for _, t := range s.Tiles {
		v := t.view()
		for g := range v.Cities {
			key := [3]int{t.X, t.Y, g}
			if seen[key] {
				continue
			}
			f := walkCityFeature(s, idx, seen, t.X, t.Y, g)
			f.id = fmt.Sprintf("c%d", len(out))
			out = append(out, f)
		}
	}
	return out
}

func walkCity(s *State, idx map[[2]int]int, x, y, g int) (feature, []claim) {
	seen := map[[3]int]bool{}
	f := walkCityFeature(s, idx, seen, x, y, g)
	return f, f.meeples
}

func walkCityFeature(s *State, idx map[[2]int]int, seen map[[3]int]bool, x, y, g int) feature {
	type node struct{ x, y, g int }
	q := []node{{x, y, g}}
	seen[[3]int{x, y, g}] = true
	tileSeen := map[int]bool{}
	var f feature
	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		ti := idx[[2]int{n.x, n.y}]
		v := s.Tiles[ti].view()
		if !tileSeen[ti] {
			tileSeen[ti] = true
			f.tiles = append(f.tiles, ti)
			if v.Shield {
				f.shields++
			}
		}
		if mp := s.Tiles[ti].Meeple; mp != nil && mp.Kind == KindCity && mp.Index == n.g {
			f.meeples = append(f.meeples, claim{ti, mp.PlayerID})
		}
		for _, e := range v.Cities[n.g] {
			f.edges = append(f.edges, [3]int{n.x, n.y, e})
			nx, ny := n.x+deltas[e][0], n.y+deltas[e][1]
			ot, ok := idx[[2]int{nx, ny}]
			if !ok {
				f.open++
				continue
			}
			ov := s.Tiles[ot].view()
			oe := (e + 2) % 4
			if ov.Edges[oe] != City {
				f.open++
				continue
			}
			og := groupOf(ov.Cities, oe)
			key := [3]int{nx, ny, og}
			if seen[key] {
				continue
			}
			seen[key] = true
			q = append(q, node{nx, ny, og})
		}
	}
	return f
}

func collectRoads(s *State, idx map[[2]int]int) []feature {
	seen := map[[3]int]bool{}
	var out []feature
	for _, t := range s.Tiles {
		v := t.view()
		for g := range v.Roads {
			key := [3]int{t.X, t.Y, g}
			if seen[key] {
				continue
			}
			out = append(out, walkRoadFeature(s, idx, seen, t.X, t.Y, g))
		}
	}
	return out
}

func walkRoad(s *State, idx map[[2]int]int, x, y, g int) (feature, []claim) {
	seen := map[[3]int]bool{}
	f := walkRoadFeature(s, idx, seen, x, y, g)
	return f, f.meeples
}

func walkRoadFeature(s *State, idx map[[2]int]int, seen map[[3]int]bool, x, y, g int) feature {
	type node struct{ x, y, g int }
	q := []node{{x, y, g}}
	seen[[3]int{x, y, g}] = true
	tileSeen := map[int]bool{}
	var f feature
	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		ti := idx[[2]int{n.x, n.y}]
		v := s.Tiles[ti].view()
		if !tileSeen[ti] {
			tileSeen[ti] = true
			f.tiles = append(f.tiles, ti)
		}
		if mp := s.Tiles[ti].Meeple; mp != nil && mp.Kind == KindRoad && mp.Index == n.g {
			f.meeples = append(f.meeples, claim{ti, mp.PlayerID})
		}
		for _, e := range v.Roads[n.g] {
			nx, ny := n.x+deltas[e][0], n.y+deltas[e][1]
			ot, ok := idx[[2]int{nx, ny}]
			if !ok {
				f.open++
				continue
			}
			ov := s.Tiles[ot].view()
			oe := (e + 2) % 4
			switch ov.Edges[oe] {
			case City:
			case Road:
				og := groupOf(ov.Roads, oe)
				key := [3]int{nx, ny, og}
				if seen[key] {
					continue
				}
				seen[key] = true
				q = append(q, node{nx, ny, og})
			default:
				f.open++
			}
		}
	}
	return f
}

func walkField(s *State, idx map[[2]int]int, seen *map[[3]int]bool, x, y, g int) ([]int, []claim, [][3]int, int) {
	if seen == nil {
		m := map[[3]int]bool{}
		seen = &m
	}
	type node struct{ x, y, g int }
	q := []node{{x, y, g}}
	(*seen)[[3]int{x, y, g}] = true
	var tiles []int
	var meeples []claim
	var visits [][3]int
	open := 0
	tileSeen := map[int]bool{}
	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		ti, ok := idx[[2]int{n.x, n.y}]
		if !ok {
			continue
		}
		v := s.Tiles[ti].view()
		if n.g < 0 || n.g >= len(v.Fields) {
			continue
		}
		visits = append(visits, [3]int{n.x, n.y, n.g})
		if !tileSeen[ti] {
			tileSeen[ti] = true
			tiles = append(tiles, ti)
		}
		if mp := s.Tiles[ti].Meeple; mp != nil && mp.Kind == KindField && mp.Index == n.g {
			meeples = append(meeples, claim{ti, mp.PlayerID})
		}
		for _, c := range v.Fields[n.g] {
			for _, e := range cornerEdges(c) {
				if v.Edges[e] == City {
					continue
				}
				nc, ok := linkCorner(e, c)
				if !ok {
					continue
				}
				nx, ny := n.x+deltas[e][0], n.y+deltas[e][1]
				ot, exists := idx[[2]int{nx, ny}]
				if !exists {
					open++
					continue
				}
				ov := s.Tiles[ot].view()
				if ov.Edges[(e+2)%4] == City {
					continue
				}
				og := groupOf(ov.Fields, nc)
				if og < 0 {
					continue
				}
				key := [3]int{nx, ny, og}
				if (*seen)[key] {
					continue
				}
				(*seen)[key] = true
				q = append(q, node{nx, ny, og})
			}
		}
	}
	return tiles, meeples, visits, open
}

func cornerEdges(c int) [2]int {
	switch c {
	case 0:
		return [2]int{0, 1}
	case 1:
		return [2]int{1, 2}
	case 2:
		return [2]int{2, 3}
	default:
		return [2]int{3, 0}
	}
}

func linkCorner(edge, corner int) (int, bool) {
	switch edge {
	case 0:
		if corner == 0 {
			return 1, true
		}
		if corner == 3 {
			return 2, true
		}
	case 1:
		if corner == 0 {
			return 3, true
		}
		if corner == 1 {
			return 2, true
		}
	case 2:
		if corner == 1 {
			return 0, true
		}
		if corner == 2 {
			return 3, true
		}
	case 3:
		if corner == 2 {
			return 1, true
		}
		if corner == 3 {
			return 0, true
		}
	}
	return 0, false
}

func groupOf(groups [][]int, item int) int {
	for i, g := range groups {
		for _, v := range g {
			if v == item {
				return i
			}
		}
	}
	return -1
}

type Public struct {
	ID          string     `json:"id"`
	Code        string     `json:"code"`
	HostID      string     `json:"hostId"`
	Open        bool       `json:"open"`
	Status      string     `json:"status"`
	MaxPlayers  int        `json:"maxPlayers"`
	MeepleCount int        `json:"meepleCount"`
	Players     []Player   `json:"players"`
	Tiles       []Placed   `json:"tiles"`
	CurrentID   string     `json:"currentId,omitempty"`
	Turn        int        `json:"turn"`
	Events      []Event    `json:"events"`
	Chat        []ChatLine `json:"chat,omitempty"`
	Winners     []string   `json:"winners,omitempty"`
	LastX       int        `json:"lastX"`
	LastY       int        `json:"lastY"`
	HasLast     bool       `json:"hasLast"`
	Deadline    time.Time  `json:"deadline,omitempty"`
	Legal       []Move     `json:"legal,omitempty"`
	Spots       []Spot     `json:"spots,omitempty"`
}

func Project(s *State) Public {
	p := Public{
		ID: s.ID, Code: s.Code, HostID: s.HostID, Open: s.Open, Status: s.Status,
		MaxPlayers: s.MaxPlayers, MeepleCount: s.MeepleCount, Players: s.Players,
		Tiles: s.Tiles, CurrentID: s.CurrentID, Turn: s.Turn, Events: s.Events,
		Winners: s.Winners, LastX: s.LastX, LastY: s.LastY, HasLast: s.HasLast,
		Deadline: s.Deadline, Chat: s.Chat,
	}
	if p.Players == nil {
		p.Players = []Player{}
	}
	if p.Tiles == nil {
		p.Tiles = []Placed{}
	}
	if p.Events == nil {
		p.Events = []Event{}
	}
	if p.Chat == nil {
		p.Chat = []ChatLine{}
	}
	p.Legal = Legal(s)
	p.Spots = Spots(s)
	return p
}

type Summary struct {
	ID         string   `json:"id"`
	Host       string   `json:"host"`
	Names      []string `json:"names"`
	Players    int      `json:"players"`
	MaxPlayers int      `json:"maxPlayers"`
	Open       bool     `json:"open"`
	Status     string   `json:"status"`
}

func Summarize(s *State) Summary {
	sum := Summary{
		ID: s.ID, Players: len(s.Players), MaxPlayers: s.MaxPlayers, Open: s.Open, Status: s.Status,
	}
	for _, p := range s.Players {
		if p.ID == s.HostID {
			sum.Host = p.Name
		}
		sum.Names = append(sum.Names, p.Name)
	}
	return sum
}
