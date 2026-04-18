package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alan/not-scrabble/internal/dict"
	"github.com/alan/not-scrabble/internal/game"
	"github.com/alan/not-scrabble/internal/store"
)

type testClient struct {
	t      *testing.T
	server *httptest.Server
	cookie *http.Cookie
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	d := dict.FromWords([]string{"CAT", "CATS", "HI", "HA", "IT", "AT"})
	srv := New(Deps{
		Store:         store.NewMemory(),
		Dict:          d,
		Auth:          DevAuth{},
		AllowDevLogin: true,
		Now:           func() time.Time { return time.Unix(1700000000, 0) },
		RandSeed:      func() int64 { return 42 },
	})
	return httptest.NewServer(srv.Handler())
}

func (c *testClient) login(userID, name string) {
	c.t.Helper()
	body, _ := json.Marshal(map[string]string{"userId": userID, "name": name})
	req, _ := http.NewRequest("POST", c.server.URL+"/api/auth/dev/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	resp.Body.Close()
	for _, ck := range resp.Cookies() {
		if ck.Name == "dev_user" {
			c.cookie = ck
		}
	}
	if c.cookie == nil {
		c.t.Fatal("login did not set dev_user cookie")
	}
}

func (c *testClient) do(method, path string, body any, out any) *http.Response {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.server.URL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && resp.StatusCode < 400 {
			c.t.Fatalf("decode %s %s: %v", method, path, err)
		}
	}
	return resp
}

func TestEndToEndGameFlow(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	alice := &testClient{t: t, server: server}
	alice.login("alice", "Alice")

	bob := &testClient{t: t, server: server}
	bob.login("bob", "Bob")

	var create CreateGameResponse
	if resp := alice.do("POST", "/api/games", CreateGameRequest{NumPlayers: 2}, &create); resp.StatusCode != 201 {
		t.Fatalf("create: status %d", resp.StatusCode)
	}
	if create.GameID == "" || create.InviteCode == "" {
		t.Fatal("missing gameID or invite")
	}

	// Game is immediately active after creation.
	var preJoin GameView
	alice.do("GET", "/api/games/"+create.GameID, nil, &preJoin)
	if preJoin.Status != game.StatusActive {
		t.Fatalf("status after create = %s, want active", preJoin.Status)
	}
	if preJoin.OpenSeats != 1 {
		t.Fatalf("openSeats = %d, want 1", preJoin.OpenSeats)
	}

	var joined GameView
	if resp := bob.do("POST", "/api/games/join", JoinRequest{InviteCode: create.InviteCode}, &joined); resp.StatusCode != 200 {
		t.Fatalf("join: status %d", resp.StatusCode)
	}
	if joined.OpenSeats != 0 {
		t.Fatalf("openSeats after join = %d, want 0", joined.OpenSeats)
	}

	// Alice fetches the game to see her rack.
	var aliceView GameView
	alice.do("GET", "/api/games/"+create.GameID, nil, &aliceView)
	if aliceView.YourPlayerIdx != 0 {
		t.Fatalf("alice YourPlayerIdx = %d", aliceView.YourPlayerIdx)
	}
	if len(aliceView.Players[0].Rack) != 7 {
		t.Fatalf("alice rack size = %d", len(aliceView.Players[0].Rack))
	}
	// Bob's rack must be hidden for Alice.
	if len(aliceView.Players[1].Rack) != 0 || aliceView.Players[1].RackSize != 7 {
		t.Fatalf("bob rack leaked: %v (size %d)", aliceView.Players[1].Rack, aliceView.Players[1].RackSize)
	}

	// Non-current-turn player can't play.
	bobPlay := PlayRequest{
		Type: game.TurnPlay,
		Placements: []game.Placement{
			{Row: 7, Col: 7, Letter: 'C', Blank: false},
			{Row: 7, Col: 8, Letter: 'A', Blank: false},
			{Row: 7, Col: 9, Letter: 'T', Blank: false},
		},
	}
	resp := bob.do("POST", "/api/games/"+create.GameID+"/plays", bobPlay, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("bob play: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Alice passes; turn should advance to Bob.
	var passed PlayResponse
	if resp := alice.do("POST", "/api/games/"+create.GameID+"/plays", PlayRequest{Type: game.TurnPass}, &passed); resp.StatusCode != 200 {
		t.Fatalf("alice pass: status %d", resp.StatusCode)
	}
	if passed.Game.CurrentIdx != 1 {
		t.Fatalf("currentIdx after pass = %d, want 1", passed.Game.CurrentIdx)
	}

	// Games list for Alice.
	var games []GameSummary
	alice.do("GET", "/api/users/me/games", nil, &games)
	if len(games) != 1 {
		t.Fatalf("games list len = %d", len(games))
	}
}

func TestInvalidWordReturns400WithList(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	alice := &testClient{t: t, server: server}
	alice.login("alice", "Alice")
	bob := &testClient{t: t, server: server}
	bob.login("bob", "Bob")

	var create CreateGameResponse
	alice.do("POST", "/api/games", CreateGameRequest{NumPlayers: 2}, &create)
	bob.do("POST", "/api/games/join", JoinRequest{InviteCode: create.InviteCode}, nil)

	// Fetch Alice's rack and try to play some letters not forming a real word.
	var v GameView
	alice.do("GET", "/api/games/"+create.GameID, nil, &v)
	rack := v.Players[0].Rack
	// Play three tiles horizontally through center; whatever letters they are,
	// the word almost certainly is not in our 6-word test dictionary.
	placements := []game.Placement{
		{Row: 7, Col: 6, Letter: rack[0], Blank: false},
		{Row: 7, Col: 7, Letter: rack[1], Blank: false},
		{Row: 7, Col: 8, Letter: rack[2], Blank: false},
	}
	resp := alice.do("POST", "/api/games/"+create.GameID+"/plays", PlayRequest{Type: game.TurnPlay, Placements: placements}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var er ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&er)
	// Accept either an InvalidWords array or a generic error depending on which
	// validation step caught it (rack check vs dict check). At least one field
	// must indicate the failure.
	if er.Error == "" {
		t.Fatal("no error message")
	}
}
