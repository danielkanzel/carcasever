package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"carcasever/internal/game"
	"carcasever/internal/store"
	"github.com/coder/websocket"
)

func TestLobbyAndMatch(t *testing.T) {
	srv := New(store.NewMemory(), t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	host := postJSON(t, ts.URL+"/api/session", "", map[string]string{"name": "Аня"})
	guest := postJSON(t, ts.URL+"/api/session", "", map[string]string{"name": "Боря"})
	created := postJSON(t, ts.URL+"/api/games", str(host["token"]), map[string]any{"open": true, "maxPlayers": 5, "meeples": 7})
	if str(created["code"]) == "" || str(created["status"]) != "lobby" {
		t.Fatalf("created %#v", created)
	}
	bad := postStatus(t, ts.URL+"/api/games/join", str(guest["token"]), map[string]string{"code": "NOPE"})
	if bad != http.StatusBadRequest {
		t.Fatalf("bad code status %d", bad)
	}
	joined := postJSON(t, ts.URL+"/api/games/join", str(guest["token"]), map[string]string{"code": str(created["code"])})
	if len(asSlice(joined["players"])) != 2 {
		t.Fatalf("players %#v", joined["players"])
	}
	list := getJSON(t, ts.URL+"/api/games", str(host["token"]))
	if len(list) != 1 || list[0]["id"] != created["id"] {
		t.Fatalf("list %#v", list)
	}

	hconn := dial(t, ts.URL, str(host["token"]), str(created["id"]))
	gconn := dial(t, ts.URL, str(guest["token"]), str(created["id"]))
	defer hconn.Close(websocket.StatusNormalClosure, "")
	defer gconn.Close(websocket.StatusNormalClosure, "")
	hlive := pump(hconn)
	_ = pump(gconn)

	write(t, hconn, map[string]any{"type": "hello"})
	wait(t, hlive, func(s game.Public) bool { return s.Status == game.StatusLobby && len(s.Players) == 2 })
	write(t, hconn, map[string]any{"type": "start"})
	placed := wait(t, hlive, func(s game.Public) bool { return s.Status == game.StatusPlace && s.CurrentID != "" && len(s.Legal) > 0 })
	move := placed.Legal[0]
	write(t, hconn, map[string]any{"type": "place", "x": move.X, "y": move.Y, "rot": move.Rots[0]})
	wait(t, hlive, func(s game.Public) bool { return s.Status == game.StatusMeeple })
	write(t, hconn, map[string]any{"type": "pass"})
	wait(t, hlive, func(s game.Public) bool { return s.Status == game.StatusPlace && s.Turn == 1 })

	late := postJSON(t, ts.URL+"/api/session", "", map[string]string{"name": "Вера"})
	seated := postJSON(t, ts.URL+"/api/games/join", str(late["token"]), map[string]string{"id": str(created["id"])})
	if len(asSlice(seated["players"])) != 3 {
		t.Fatalf("late join %#v", seated["players"])
	}
	if seated["turn"] != float64(2) {
		t.Fatalf("turn after join %#v", seated["turn"])
	}
	names := asSlice(seated["players"])
	if names[2].(map[string]any)["id"] != guest["id"] || names[1].(map[string]any)["id"] != late["id"] {
		t.Fatalf("queue %#v", names)
	}

	if postStatus(t, ts.URL+"/api/games/"+str(created["id"])+"/leave", str(host["token"]), map[string]any{}) != http.StatusOK {
		t.Fatal("host leave")
	}
	if postStatus(t, ts.URL+"/api/games/"+str(created["id"])+"/leave", str(guest["token"]), map[string]any{}) != http.StatusOK {
		t.Fatal("guest leave")
	}
	board := getJSON(t, ts.URL+"/api/leaderboard", str(late["token"]))
	if board == nil {
		t.Fatal("leaderboard")
	}
}

func TestRegisterAndLogin(t *testing.T) {
	srv := New(store.NewMemory(), t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	guest := postJSON(t, ts.URL+"/api/session", "", map[string]string{"name": "Аня"})
	if postStatus(t, ts.URL+"/api/register", str(guest["token"]), map[string]string{"name": "Аня", "login": "an", "password": "secret1"}) != http.StatusBadRequest {
		t.Fatal("short login")
	}
	reg := postJSON(t, ts.URL+"/api/register", str(guest["token"]), map[string]string{"name": "Аня", "login": "Anya", "password": "secret1"})
	if str(reg["id"]) != str(guest["id"]) || str(reg["login"]) != "anya" || str(reg["token"]) != str(guest["token"]) {
		t.Fatalf("register %#v", reg)
	}
	if postStatus(t, ts.URL+"/api/login", "", map[string]string{"login": "anya", "password": "nope12"}) != http.StatusBadRequest {
		t.Fatal("bad password")
	}
	back := postJSON(t, ts.URL+"/api/login", "", map[string]string{"login": "anya", "password": "secret1"})
	if str(back["id"]) != str(guest["id"]) {
		t.Fatalf("login %#v", back)
	}
	if postStatus(t, ts.URL+"/api/register", str(guest["token"]), map[string]string{"name": "Аня", "login": "other", "password": "secret1"}) != http.StatusBadRequest {
		t.Fatal("second register")
	}
	if postStatus(t, ts.URL+"/api/register", "", map[string]string{"name": "Боря", "login": "anya", "password": "secret1"}) != http.StatusBadRequest {
		t.Fatal("taken login")
	}
	me := getObject(t, ts.URL+"/api/me", str(reg["token"]))
	if str(me["login"]) != "anya" {
		t.Fatalf("me %#v", me)
	}
}

func postJSON(t *testing.T, url, token string, body any) map[string]any {
	t.Helper()
	res := do(t, http.MethodPost, url, token, body)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var raw bytes.Buffer
		raw.ReadFrom(res.Body)
		t.Fatalf("%s %d %s", url, res.StatusCode, raw.String())
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func postStatus(t *testing.T, url, token string, body any) int {
	t.Helper()
	res := do(t, http.MethodPost, url, token, body)
	res.Body.Close()
	return res.StatusCode
}

func getObject(t *testing.T, url, token string) map[string]any {
	t.Helper()
	res := do(t, http.MethodGet, url, token, nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get %d", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func getJSON(t *testing.T, url, token string) []map[string]any {
	t.Helper()
	res := do(t, http.MethodGet, url, token, nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get %d", res.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func do(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func asSlice(v any) []any {
	b, _ := json.Marshal(v)
	var out []any
	_ = json.Unmarshal(b, &out)
	return out
}

func dial(t *testing.T, httpURL, token, gameID string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + httpURL[len("http"):] + "/ws?token=" + token + "&game=" + gameID
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func write(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func pump(conn *websocket.Conn) *atomic.Value {
	var latest atomic.Value
	go func() {
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var msg struct {
				Type  string      `json:"type"`
				State game.Public `json:"state"`
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "state" {
				latest.Store(msg.State)
			}
		}
	}()
	return &latest
}

func wait(t *testing.T, latest *atomic.Value, pred func(game.Public) bool) game.Public {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if v := latest.Load(); v != nil {
			st := v.(game.Public)
			if pred(st) {
				return st
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("timed out waiting for state")
	return game.Public{}
}
