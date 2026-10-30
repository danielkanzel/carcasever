package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"carcasever/internal/game"
)

var ErrNotFound = errors.New("not_found")

type Account struct {
	ID         string `bson:"_id" json:"id"`
	Token      string `bson:"token" json:"-"`
	Name       string `bson:"name" json:"name"`
	Login      string `bson:"login,omitempty" json:"login,omitempty"`
	PassHash   string `bson:"passHash,omitempty" json:"-"`
	ActiveGame string `bson:"activeGame,omitempty" json:"activeGame,omitempty"`
}

type Standing struct {
	ID    string `json:"id" bson:"_id"`
	Name  string `json:"name" bson:"name"`
	Score int    `json:"score" bson:"score"`
}

type Store interface {
	PutAccount(ctx context.Context, a Account) error
	AccountByToken(ctx context.Context, token string) (Account, error)
	AccountByID(ctx context.Context, id string) (Account, error)
	AccountByLogin(ctx context.Context, login string) (Account, error)
	PutGame(ctx context.Context, g *game.State) error
	Game(ctx context.Context, id string) (*game.State, error)
	GameByCode(ctx context.Context, code string) (*game.State, error)
	OpenLobbies(ctx context.Context) ([]*game.State, error)
	LiveGames(ctx context.Context) ([]*game.State, error)
	DeleteGame(ctx context.Context, id string) error
	AddScore(ctx context.Context, id, name string, delta int) error
	Leaderboard(ctx context.Context, limit int) ([]Standing, error)
}

type Memory struct {
	mu       sync.Mutex
	accounts map[string]Account
	byToken  map[string]string
	games    map[string]*game.State
	byCode   map[string]string
	byLogin  map[string]string
	board    map[string]Standing
}

func NewMemory() *Memory {
	return &Memory{
		accounts: map[string]Account{},
		byToken:  map[string]string{},
		byLogin:  map[string]string{},
		games:    map[string]*game.State{},
		byCode:   map[string]string{},
	}
}

func (m *Memory) PutAccount(_ context.Context, a Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.Login != "" {
		if other, ok := m.byLogin[a.Login]; ok && other != a.ID {
			return errors.New("login_taken")
		}
	}
	if prev, ok := m.accounts[a.ID]; ok {
		if prev.Token != a.Token {
			delete(m.byToken, prev.Token)
		}
		if prev.Login != "" && prev.Login != a.Login {
			delete(m.byLogin, prev.Login)
		}
	}
	m.accounts[a.ID] = a
	m.byToken[a.Token] = a.ID
	if a.Login != "" {
		m.byLogin[a.Login] = a.ID
	}
	return nil
}

func (m *Memory) AccountByToken(_ context.Context, token string) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byToken[token]
	if !ok {
		return Account{}, ErrNotFound
	}
	return m.accounts[id], nil
}

func (m *Memory) AccountByID(_ context.Context, id string) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	return a, nil
}

func (m *Memory) AccountByLogin(_ context.Context, login string) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byLogin[login]
	if !ok {
		return Account{}, ErrNotFound
	}
	return m.accounts[id], nil
}

func (m *Memory) PutGame(_ context.Context, g *game.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp, err := cloneState(g)
	if err != nil {
		return err
	}
	if prev, ok := m.games[g.ID]; ok && prev.Code != g.Code {
		delete(m.byCode, prev.Code)
	}
	m.games[g.ID] = cp
	m.byCode[g.Code] = g.ID
	return nil
}

func (m *Memory) Game(_ context.Context, id string) (*game.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.games[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneState(g)
}

func (m *Memory) GameByCode(_ context.Context, code string) (*game.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byCode[code]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneState(m.games[id])
}

func (m *Memory) OpenLobbies(_ context.Context) ([]*game.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*game.State
	for _, g := range m.games {
		if g.Open && (g.Status == game.StatusLobby || g.Status == game.StatusPlace || g.Status == game.StatusMeeple) {
			cp, err := cloneState(g)
			if err != nil {
				return nil, err
			}
			out = append(out, cp)
		}
	}
	return out, nil
}

func (m *Memory) LiveGames(_ context.Context) ([]*game.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*game.State
	for _, g := range m.games {
		if g.Status == game.StatusPlace || g.Status == game.StatusMeeple {
			cp, err := cloneState(g)
			if err != nil {
				return nil, err
			}
			out = append(out, cp)
		}
	}
	return out, nil
}

func (m *Memory) AddScore(_ context.Context, id, name string, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.board == nil {
		m.board = map[string]Standing{}
	}
	row := m.board[id]
	row.ID = id
	row.Name = name
	row.Score += delta
	m.board[id] = row
	return nil
}

func (m *Memory) Leaderboard(_ context.Context, limit int) ([]Standing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	out := make([]Standing, 0, len(m.board))
	for _, row := range m.board {
		if row.Score > 0 {
			out = append(out, row)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Score > out[j-1].Score || (out[j].Score == out[j-1].Score && out[j].Name < out[j-1].Name)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) DeleteGame(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok := m.games[id]; ok {
		delete(m.byCode, g.Code)
		delete(m.games, id)
	}
	return nil
}

func cloneState(g *game.State) (*game.State, error) {
	b, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	var out game.State
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
