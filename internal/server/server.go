package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"carcasever/internal/game"
	"carcasever/internal/store"
	"github.com/coder/websocket"
	"golang.org/x/crypto/bcrypt"
)

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type Server struct {
	db      store.Store
	webDir  string
	mu      sync.Mutex
	locks   map[string]*sync.Mutex
	hub     *hub
	clockMu sync.Mutex
	clocks  map[string]*time.Timer
}

var errUnchanged = errors.New("unchanged")

func New(db store.Store, webDir string) *Server {
	s := &Server{db: db, webDir: webDir, locks: map[string]*sync.Mutex{}, hub: newHub(), clocks: map[string]*time.Timer{}}
	go s.resumeClocks()
	return s
}

func (s *Server) hold(id string) func() {
	s.mu.Lock()
	lk, ok := s.locks[id]
	if !ok {
		lk = &sync.Mutex{}
		s.locks[id] = lk
	}
	s.mu.Unlock()
	lk.Lock()
	return lk.Unlock
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/session", s.createSession)
	mux.HandleFunc("PATCH /api/session", s.patchSession)
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/catalog", s.catalog)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
	mux.HandleFunc("POST /api/games", s.createGame)
	mux.HandleFunc("GET /api/leaderboard", s.leaderboard)
	mux.HandleFunc("GET /api/games", s.listGames)
	mux.HandleFunc("POST /api/games/join", s.joinGame)
	mux.HandleFunc("GET /api/games/{id}", s.getGame)
	mux.HandleFunc("POST /api/games/{id}/leave", s.leaveGame)
	mux.HandleFunc("GET /ws", s.serveWS)
	art := os.Getenv("ART_DIR")
	if art == "" {
		art = "resources"
	}
	mux.Handle("GET /img/", http.StripPrefix("/img/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.FileServer(http.Dir(art)).ServeHTTP(w, r)
	})))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.FileServer(http.Dir(s.webDir)).ServeHTTP(w, r)
	}))
	return mux
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, game.Catalog)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	name, err := cleanName(body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	acc := store.Account{ID: newID(), Token: newID() + newID(), Name: name}
	if err := s.db.PutAccount(r.Context(), acc); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": acc.Token, "id": acc.ID, "name": acc.Name})
}

func (s *Server) patchSession(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	name, err := cleanName(body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	acc.Name = name
	if err := s.db.PutAccount(r.Context(), acc); err != nil {
		writeErr(w, err)
		return
	}
	if acc.ActiveGame != "" {
		_ = s.withGame(r.Context(), acc.ActiveGame, func(st *game.State) error {
			st.Rename(acc.ID, name)
			return nil
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": acc.ID, "name": acc.Name, "login": acc.Login, "activeGame": acc.ActiveGame})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	login, err := cleanLogin(body.Login)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := cleanPassword(body.Password); err != nil {
		writeErr(w, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = login
	}
	name, err = cleanName(name)
	if err != nil {
		writeErr(w, err)
		return
	}
	taken, err := s.db.AccountByLogin(r.Context(), login)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeErr(w, err)
		return
	}
	acc, has := s.optionalAccount(r)
	if has && acc.Login != "" {
		writeErr(w, errors.New("already_registered"))
		return
	}
	if taken.ID != "" && taken.ID != acc.ID {
		writeErr(w, errors.New("login_taken"))
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !has {
		acc = store.Account{ID: newID(), Token: newID() + newID()}
	}
	acc.Name = name
	acc.Login = login
	acc.PassHash = string(hash)
	if err := s.db.PutAccount(r.Context(), acc); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.db.AddScore(r.Context(), acc.ID, acc.Name, 0)
	if acc.ActiveGame != "" {
		_ = s.withGame(r.Context(), acc.ActiveGame, func(st *game.State) error {
			st.Rename(acc.ID, name)
			return nil
		})
	}
	writeJSON(w, http.StatusOK, accountJSON(acc))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	login, err := cleanLogin(body.Login)
	if err != nil {
		writeErr(w, errors.New("bad_password"))
		return
	}
	acc, err := s.db.AccountByLogin(r.Context(), login)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(acc.PassHash), []byte(body.Password)) != nil {
		writeErr(w, errors.New("bad_password"))
		return
	}
	writeJSON(w, http.StatusOK, accountJSON(acc))
}

func accountJSON(acc store.Account) map[string]string {
	return map[string]string{
		"token":      acc.Token,
		"id":         acc.ID,
		"name":       acc.Name,
		"login":      acc.Login,
		"activeGame": acc.ActiveGame,
	}
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": acc.ID, "name": acc.Name, "login": acc.Login, "activeGame": acc.ActiveGame})
}

func (s *Server) createGame(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Open       bool `json:"open"`
		MaxPlayers int  `json:"maxPlayers"`
		Meeples    int  `json:"meeples"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.MaxPlayers == 0 {
		body.MaxPlayers = 5
	}
	if body.Meeples == 0 {
		body.Meeples = 7
	}
	if body.MaxPlayers < 2 || body.MaxPlayers > 6 || body.Meeples < 5 || body.Meeples > 20 {
		writeErr(w, game.ErrBadSettings)
		return
	}
	if err := s.leaveActiveLobby(r.Context(), &acc); err != nil {
		writeErr(w, err)
		return
	}
	st := &game.State{
		ID: newID(), Code: s.freshCode(r.Context()), HostID: acc.ID, Open: body.Open,
		Status: game.StatusLobby, MaxPlayers: body.MaxPlayers, MeepleCount: body.Meeples,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.AddPlayer(acc.ID, acc.Name); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.PutGame(r.Context(), st); err != nil {
		writeErr(w, err)
		return
	}
	acc.ActiveGame = st.ID
	if err := s.db.PutAccount(r.Context(), acc); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game.Project(st))
}

func (s *Server) listGames(w http.ResponseWriter, r *http.Request) {
	if _, err := s.account(r); err != nil {
		writeErr(w, err)
		return
	}
	games, err := s.db.OpenLobbies(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []game.Summary{}
	for _, g := range games {
		if len(g.Players) >= g.MaxPlayers {
			continue
		}
		out = append(out, game.Summarize(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) joinGame(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Code string `json:"code"`
		ID   string `json:"id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	var st *game.State
	if body.Code != "" {
		st, err = s.db.GameByCode(r.Context(), normalizeCode(body.Code))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, errors.New("bad_code"))
				return
			}
			writeErr(w, err)
			return
		}
	} else if body.ID != "" {
		st, err = s.db.Game(r.Context(), body.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !st.Open {
			writeErr(w, game.ErrClosed)
			return
		}
	} else {
		writeErr(w, errors.New("bad_code"))
		return
	}
	if contains(st, acc.ID) {
		acc.ActiveGame = st.ID
		_ = s.db.PutAccount(r.Context(), acc)
		writeJSON(w, http.StatusOK, game.Project(st))
		return
	}
	if err := s.leaveActiveLobby(r.Context(), &acc); err != nil {
		writeErr(w, err)
		return
	}
	err = s.withGame(r.Context(), st.ID, func(g *game.State) error {
		if !g.Open && body.ID != "" && body.Code == "" {
			return game.ErrClosed
		}
		return g.AddPlayer(acc.ID, acc.Name)
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	acc.ActiveGame = st.ID
	if err := s.db.PutAccount(r.Context(), acc); err != nil {
		writeErr(w, err)
		return
	}
	fresh, err := s.db.Game(r.Context(), st.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game.Project(fresh))
}

func contains(st *game.State, id string) bool {
	for _, p := range st.Players {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) getGame(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := s.db.Game(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !contains(st, acc.ID) {
		writeErr(w, game.ErrNotInGame)
		return
	}
	writeJSON(w, http.StatusOK, game.Project(st))
}

func (s *Server) leaveGame(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	id := r.PathValue("id")
	banked, err := s.depart(r.Context(), id, acc.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if acc.ActiveGame == id {
		acc.ActiveGame = ""
		_ = s.db.PutAccount(r.Context(), acc)
	}
	mine := 0
	for _, b := range banked {
		if b.ID == acc.ID {
			mine = b.Score
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "score": mine})
}

func (s *Server) depart(ctx context.Context, gameID, playerID string) ([]game.Bank, error) {
	var banks []game.Bank
	err := s.withGame(ctx, gameID, func(g *game.State) error {
		got, err := g.Leave(playerID)
		banks = got
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, b := range banks {
		if b.Score <= 0 {
			continue
		}
		if err := s.db.AddScore(ctx, b.ID, b.Name, b.Score); err != nil {
			return nil, err
		}
	}
	return banks, nil
}

func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request) {
	if _, err := s.account(r); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := s.db.Leaderboard(r.Context(), 30)
	if err != nil {
		writeErr(w, err)
		return
	}
	if rows == nil {
		rows = []store.Standing{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) leaveActiveLobby(ctx context.Context, acc *store.Account) error {
	if acc.ActiveGame == "" {
		return nil
	}
	st, err := s.db.Game(ctx, acc.ActiveGame)
	if errors.Is(err, store.ErrNotFound) {
		acc.ActiveGame = ""
		return s.db.PutAccount(ctx, *acc)
	}
	if err != nil {
		return err
	}
	if contains(st, acc.ID) && st.Status != game.StatusDone {
		if _, err := s.depart(ctx, st.ID, acc.ID); err != nil {
			return err
		}
	}
	acc.ActiveGame = ""
	return s.db.PutAccount(ctx, *acc)
}

func (s *Server) withGame(ctx context.Context, id string, fn func(*game.State) error) error {
	unlock := s.hold(id)
	defer unlock()
	st, err := s.db.Game(ctx, id)
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		if errors.Is(err, errUnchanged) {
			s.watch(st)
			return nil
		}
		return err
	}
	if st.Status == game.StatusLobby && len(st.Players) == 0 {
		s.stopClock(id)
		if err := s.db.DeleteGame(ctx, id); err != nil {
			return err
		}
		s.hub.broadcast(id, mustJSON(map[string]string{"type": "gone"}))
		return nil
	}
	st.UpdatedAt = time.Now()
	if err := s.db.PutGame(ctx, st); err != nil {
		return err
	}
	s.push(st)
	s.watch(st)
	return nil
}

func (s *Server) resumeClocks() {
	games, err := s.db.LiveGames(context.Background())
	if err != nil {
		return
	}
	for _, g := range games {
		s.watch(g)
	}
}

func (s *Server) stopClock(id string) {
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	if t, ok := s.clocks[id]; ok {
		t.Stop()
		delete(s.clocks, id)
	}
}

func (s *Server) watch(st *game.State) {
	id := st.ID
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	if t, ok := s.clocks[id]; ok {
		t.Stop()
		delete(s.clocks, id)
	}
	if st.Deadline.IsZero() || (st.Status != game.StatusPlace && st.Status != game.StatusMeeple) {
		return
	}
	wait := time.Until(st.Deadline)
	if wait < 0 {
		wait = 0
	}
	s.clocks[id] = time.AfterFunc(wait, func() { s.expire(id) })
}

func (s *Server) expire(id string) {
	var banks []game.Bank
	err := s.withGame(context.Background(), id, func(g *game.State) error {
		got, ok := g.Expire(time.Now())
		if !ok {
			return errUnchanged
		}
		banks = got
		return nil
	})
	if err != nil {
		return
	}
	for _, b := range banks {
		if b.Score > 0 {
			_ = s.db.AddScore(context.Background(), b.ID, b.Name, b.Score)
		}
	}
}

func (s *Server) push(st *game.State) {
	s.hub.broadcast(st.ID, mustJSON(map[string]any{"type": "state", "state": game.Project(st)}))
}

type command struct {
	Type       string `json:"type"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Rot        int    `json:"rot"`
	Kind       string `json:"kind"`
	Index      int    `json:"index"`
	On         *bool  `json:"on"`
	PlayerID   string `json:"playerId"`
	Open       *bool  `json:"open"`
	MaxPlayers *int   `json:"maxPlayers"`
	Meeples    *int   `json:"meeples"`
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	gameID := r.URL.Query().Get("game")
	if token == "" || gameID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	acc, err := s.db.AccountByToken(r.Context(), token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	st, err := s.db.Game(r.Context(), gameID)
	if err != nil || !contains(st, acc.ID) {
		http.Error(w, "not_in_game", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 16)
	c := &client{conn: conn, gameID: gameID, playerID: acc.ID}
	n := s.hub.add(c)
	if n == 1 {
		_ = s.withGame(context.Background(), gameID, func(g *game.State) error {
			g.SetConnected(acc.ID, true)
			return nil
		})
	} else if fresh, err := s.db.Game(context.Background(), gameID); err == nil {
		c.write(mustJSON(map[string]any{"type": "state", "state": game.Project(fresh)}))
	}
	defer func() {
		s.hub.remove(c)
		conn.Close(websocket.StatusNormalClosure, "")
		if s.hub.count(gameID, acc.ID) == 0 {
			_ = s.withGame(context.Background(), gameID, func(g *game.State) error {
				g.SetConnected(acc.ID, false)
				return nil
			})
		}
	}()

	pingCtx, stopPing := context.WithCancel(r.Context())
	defer stopPing()
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-t.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				c.mu.Lock()
				err := conn.Ping(ctx)
				c.mu.Unlock()
				cancel()
				if err != nil {
					conn.Close(websocket.StatusGoingAway, "ping")
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var cmd command
		if err := json.Unmarshal(data, &cmd); err != nil {
			c.write(mustJSON(map[string]string{"type": "error", "error": "bad_json"}))
			continue
		}
		err = s.withGame(r.Context(), gameID, func(g *game.State) error {
			if !contains(g, acc.ID) {
				return game.ErrNotInGame
			}
			switch cmd.Type {
			case "hello":
				return nil
			case "start":
				return g.Start(acc.ID)
			case "kick":
				if err := g.Kick(acc.ID, cmd.PlayerID); err != nil {
					return err
				}
				target, err := s.db.AccountByID(r.Context(), cmd.PlayerID)
				if err == nil && target.ActiveGame == g.ID {
					target.ActiveGame = ""
					_ = s.db.PutAccount(r.Context(), target)
				}
				return nil
			case "settings":
				open := g.Open
				maxP := g.MaxPlayers
				meeples := g.MeepleCount
				if cmd.Open != nil {
					open = *cmd.Open
				}
				if cmd.MaxPlayers != nil {
					maxP = *cmd.MaxPlayers
				}
				if cmd.Meeples != nil {
					meeples = *cmd.Meeples
				}
				return g.ApplySettings(acc.ID, open, maxP, meeples)
			case "place":
				return g.Place(acc.ID, cmd.X, cmd.Y, cmd.Rot)
			case "meeple":
				return g.Meeple(acc.ID, cmd.Kind, cmd.Index)
			case "pass":
				return g.Pass(acc.ID)
			case "turn":
				dir := 1
				if cmd.Rot < 0 {
					dir = -1
				}
				return g.Rotate(acc.ID, dir)
			case "recall":
				return g.Recall(acc.ID)
			case "vote":
				on := false
				if cmd.On != nil {
					on = *cmd.On
				}
				return g.Vote(acc.ID, on)
			case "finish_partial":
				return g.FinishPartial(acc.ID)
			case "skip_afk":
				return g.SkipAFK(acc.ID)
			default:
				return errors.New("bad_command")
			}
		})
		if err != nil {
			c.write(mustJSON(map[string]string{"type": "error", "error": err.Error()}))
		}
	}
}

func (s *Server) optionalAccount(r *http.Request) (store.Account, bool) {
	h := r.Header.Get("Authorization")
	token := strings.TrimPrefix(h, "Bearer ")
	if token == "" || token == h {
		return store.Account{}, false
	}
	acc, err := s.db.AccountByToken(r.Context(), token)
	if err != nil {
		return store.Account{}, false
	}
	return acc, true
}

func (s *Server) account(r *http.Request) (store.Account, error) {
	h := r.Header.Get("Authorization")
	token := strings.TrimPrefix(h, "Bearer ")
	if token == "" || token == h {
		return store.Account{}, errors.New("unauthorized")
	}
	acc, err := s.db.AccountByToken(r.Context(), token)
	if errors.Is(err, store.ErrNotFound) {
		return store.Account{}, errors.New("unauthorized")
	}
	return acc, err
}

func (s *Server) freshCode(ctx context.Context) string {
	for i := 0; i < 8; i++ {
		code := newCode()
		_, err := s.db.GameByCode(ctx, code)
		if errors.Is(err, store.ErrNotFound) {
			return code
		}
	}
	return newCode()
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newCode() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	out := make([]byte, 6)
	for i := range out {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(out)
}

func normalizeCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	var b strings.Builder
	for _, r := range code {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 24 {
		return "", errors.New("name_required")
	}
	return name, nil
}

func cleanLogin(login string) (string, error) {
	login = strings.TrimSpace(strings.ToLower(login))
	n := len([]rune(login))
	if n < 3 || n > 24 {
		return "", errors.New("bad_login")
	}
	for _, r := range login {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return "", errors.New("bad_login")
		}
	}
	return login, nil
}

func cleanPassword(password string) error {
	n := len([]rune(password))
	if n < 6 || len(password) > 72 {
		return errors.New("bad_password")
	}
	return nil
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	if err := dec.Decode(dst); err != nil {
		return errors.New("bad_json")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, game.ErrNotFound):
		status = http.StatusNotFound
	case err.Error() == "unauthorized":
		status = http.StatusUnauthorized
	case errors.Is(err, game.ErrNotHost), errors.Is(err, game.ErrNotYourTurn), errors.Is(err, game.ErrNotInGame), errors.Is(err, game.ErrClosed):
		status = http.StatusForbidden
	}
	msg := err.Error()
	if !knownErr(msg) {
		log.Printf("api: %v", err)
		if status == http.StatusBadRequest {
			msg = "bad_request"
		}
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func knownErr(msg string) bool {
	switch msg {
	case "not_host", "not_your_turn", "bad_phase", "need_players", "game_full", "game_started",
		"still_playing", "illegal_place", "illegal_meeple", "no_meeples", "not_in_game", "bad_settings",
		"game_closed", "not_found", "not_afk", "bad_code", "name_required", "unauthorized", "bad_json", "bad_command",
		"bad_login", "bad_password", "login_taken", "already_registered":
		return true
	default:
		return false
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

type client struct {
	conn     *websocket.Conn
	gameID   string
	playerID string
	mu       sync.Mutex
}

func (c *client) write(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.conn.Write(ctx, websocket.MessageText, b)
}

type hub struct {
	mu    sync.Mutex
	rooms map[string]map[*client]struct{}
}

func newHub() *hub {
	return &hub{rooms: map[string]map[*client]struct{}{}}
}

func (h *hub) add(c *client) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.rooms[c.gameID]
	if set == nil {
		set = map[*client]struct{}{}
		h.rooms[c.gameID] = set
	}
	set[c] = struct{}{}
	return h.countLocked(c.gameID, c.playerID)
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.rooms[c.gameID]
	delete(set, c)
	if len(set) == 0 {
		delete(h.rooms, c.gameID)
	}
}

func (h *hub) count(gameID, playerID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.countLocked(gameID, playerID)
}

func (h *hub) countLocked(gameID, playerID string) int {
	n := 0
	for c := range h.rooms[gameID] {
		if c.playerID == playerID {
			n++
		}
	}
	return n
}

func (h *hub) broadcast(gameID string, payload []byte) {
	h.mu.Lock()
	var list []*client
	for c := range h.rooms[gameID] {
		list = append(list, c)
	}
	h.mu.Unlock()
	for _, c := range list {
		c.write(payload)
	}
}
