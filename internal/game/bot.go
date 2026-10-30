package game

import "fmt"

func (s *State) AddBot(host string, level int) error {
	if s.HostID != host {
		return ErrNotHost
	}
	if s.Status == StatusDone {
		return ErrStarted
	}
	if level < BotEasy || level > BotHard {
		return ErrBadSettings
	}
	if len(s.Players) >= s.MaxPlayers {
		return ErrFull
	}
	n := 1
	for _, p := range s.Players {
		if p.Bot == level {
			n++
		}
	}
	name := botTitle(level)
	if n > 1 {
		name = fmt.Sprintf("%s %d", name, n)
	}
	id := fmt.Sprintf("bot-%d-%08x", level, s.rand().Uint32())
	if err := s.AddPlayer(id, name); err != nil {
		return err
	}
	i := s.playerIndex(id)
	s.Players[i].Bot = level
	s.Players[i].Connected = true
	return nil
}

func botTitle(level int) string {
	switch level {
	case BotEasy:
		return "Лёгкий бот"
	case BotHard:
		return "Сложный бот"
	default:
		return "Средний бот"
	}
}

func (s *State) actBot() error {
	cur := s.current()
	if cur == nil || cur.Bot == 0 {
		return nil
	}
	switch s.Status {
	case StatusPlace:
		moves := legal(s, s.CurrentID)
		if len(moves) == 0 {
			s.finalize()
			return nil
		}
		x, y, rot := s.choosePlace(cur)
		return s.Place(cur.ID, x, y, rot)
	case StatusMeeple:
		if kind, index, ok := s.chooseMeeple(cur); ok {
			return s.Meeple(cur.ID, kind, index)
		}
		return s.Pass(cur.ID)
	default:
		return nil
	}
}

func (s *State) choosePlace(p *Player) (int, int, int) {
	moves := legal(s, s.CurrentID)
	r := s.rand()
	if p.Bot == BotEasy || len(moves) == 1 && len(moves[0].Rots) == 1 {
		m := moves[r.IntN(len(moves))]
		return m.X, m.Y, m.Rots[r.IntN(len(m.Rots))]
	}
	bestScore := -1 << 30
	x, y, rot := moves[0].X, moves[0].Y, moves[0].Rots[0]
	for _, m := range moves {
		for _, rv := range m.Rots {
			score := s.ratePlace(p.ID, m.X, m.Y, rv)
			if p.Bot == BotMedium {
				score += r.IntN(7) - 3
			}
			if score > bestScore {
				bestScore = score
				x, y, rot = m.X, m.Y, rv
			}
		}
	}
	return x, y, rot
}

func (s *State) ratePlace(id string, x, y, rot int) int {
	before := map[string]int{}
	for _, p := range s.Players {
		before[p.ID] = p.Score
	}
	cp := s.clone()
	cp.Tiles = append(cp.Tiles, Placed{X: x, Y: y, TileID: cp.CurrentID, Rot: rot})
	cp.closeFields()
	cp.resolve(false)
	own, opp := 0, 0
	for _, p := range cp.Players {
		delta := p.Score - before[p.ID]
		if p.ID == id {
			own += delta
		} else if delta > 0 {
			opp += delta
		}
	}
	touch := 0
	idx := s.index()
	for _, dlt := range deltas {
		if _, ok := idx[[2]int{x + dlt[0], y + dlt[1]}]; ok {
			touch++
		}
	}
	weight := 2
	if cur := s.current(); cur != nil && cur.Bot == BotHard {
		weight = 4
	}
	return own*weight - opp*weight + touch
}

func (s *State) chooseMeeple(p *Player) (string, int, bool) {
	var free []Spot
	for _, sp := range Spots(s) {
		if sp.Free {
			free = append(free, sp)
		}
	}
	if len(free) == 0 {
		return "", 0, false
	}
	r := s.rand()
	if p.Bot == BotEasy {
		if r.IntN(4) != 0 {
			return "", 0, false
		}
		sp := free[r.IntN(len(free))]
		return sp.Kind, sp.Index, true
	}
	best, bestAt := -1, 0
	for i, sp := range free {
		value := s.rateSpot(sp)
		if p.Bot == BotMedium {
			value += r.IntN(3) - 1
		}
		if value > best {
			best, bestAt = value, i
		}
	}
	need := 4
	if p.Bot == BotHard {
		need = 6
		if p.Meeples <= 2 {
			need = 9
		}
	}
	if best < need {
		return "", 0, false
	}
	return free[bestAt].Kind, free[bestAt].Index, true
}

func (s *State) rateSpot(sp Spot) int {
	if len(s.Tiles) == 0 {
		return 0
	}
	last := s.Tiles[len(s.Tiles)-1]
	idx := s.index()
	switch sp.Kind {
	case KindCity:
		f, _ := walkCity(s, idx, last.X, last.Y, sp.Index)
		worth := len(f.tiles)*2 + f.shields*2
		if f.open <= 0 {
			return worth + 4
		}
		return worth * 2 / (f.open + 1)
	case KindRoad:
		f, _ := walkRoad(s, idx, last.X, last.Y, sp.Index)
		if f.open <= 0 {
			return len(f.tiles) + 2
		}
		return len(f.tiles) * 3 / (f.open + 1)
	case KindMonastery:
		return 1 + neighbors8(idx, last.X, last.Y)
	case KindField:
		_, _, _, open := walkField(s, idx, nil, last.X, last.Y, sp.Index)
		if open > 3 {
			return 1
		}
		return 5 - open
	default:
		return 0
	}
}

func (s *State) clone() *State {
	cp := *s
	cp.Players = append([]Player(nil), s.Players...)
	cp.Tiles = make([]Placed, len(s.Tiles))
	for i, tile := range s.Tiles {
		cp.Tiles[i] = tile
		if tile.Meeple != nil {
			meeple := *tile.Meeple
			cp.Tiles[i].Meeple = &meeple
		}
	}
	cp.Events = nil
	cp.Winners = nil
	return &cp
}
