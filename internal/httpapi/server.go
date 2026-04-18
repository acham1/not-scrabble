package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/alan/not-scrabble/internal/game"
	"github.com/alan/not-scrabble/internal/push"
	"github.com/alan/not-scrabble/internal/store"
)

// Deps is the set of dependencies the HTTP server needs.
type Deps struct {
	Store         store.Store
	Dict          game.WordSet
	Auth          Authenticator
	GoogleAuth    *GoogleAuth  // optional; if set, mounts Google callback routes
	Allowlist     *Allowlist   // optional; if set, checks email against the list
	Push          *push.Notifier // optional; if set, enables Web Push
	Now           func() time.Time
	RandSeed      func() int64
	StaticFS      fs.FS // optional; if set, served at "/"
	AllowDevLogin bool  // when true, mounts POST /api/auth/dev/login
}

// Server wires routes onto http.ServeMux.
type Server struct {
	deps Deps
	mux  *http.ServeMux
}

// New returns a Server with routes registered.
func New(deps Deps) *Server {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.RandSeed == nil {
		deps.RandSeed = defaultRandSeed
	}
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	api := func(path string, h http.HandlerFunc) {
		s.mux.HandleFunc(path, s.withAuth(h))
	}
	// Auth
	if s.deps.AllowDevLogin {
		s.mux.HandleFunc("POST /api/auth/dev/login", s.handleDevLogin)
		s.mux.HandleFunc("POST /api/auth/dev/logout", s.handleDevLogout)
	}
	if s.deps.GoogleAuth != nil {
		s.mux.HandleFunc("POST /api/auth/google/callback", s.deps.GoogleAuth.HandleCallback)
		s.mux.HandleFunc("POST /api/auth/google/logout", s.deps.GoogleAuth.HandleLogout)
		s.mux.HandleFunc("GET /api/auth/config", s.handleAuthConfig)
	}

	api("GET /api/users/me", s.handleUserMe)
	api("GET /api/users/me/games", s.handleUserGames)

	// Games
	api("POST /api/games", s.handleCreateGame)
	api("POST /api/games/join", s.handleJoinGame)
	api("GET /api/games/{id}", s.handleGetGame)
	api("POST /api/games/{id}/plays", s.handlePlay)

	// Push
	if s.deps.Push != nil {
		api("POST /api/push/subscribe", s.handlePushSubscribe)
		s.mux.HandleFunc("GET /api/push/vapid-key", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"key": s.deps.Push.VAPIDPublicKey()})
		})
	}

	// Health
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Static frontend
	if s.deps.StaticFS != nil {
		s.mux.Handle("/", http.FileServer(http.FS(s.deps.StaticFS)))
	}
}

func (s *Server) withAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.deps.Auth.Authenticate(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		// Ensure a user record exists for the identity.
		if _, err := s.deps.Store.GetOrCreateUser(r.Context(), id.UserID, id.Name, id.Email); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		h.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
	}
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"devLogin": s.deps.AllowDevLogin,
	}
	if s.deps.GoogleAuth != nil {
		resp["googleClientId"] = s.deps.GoogleAuth.ClientID()
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- handlers ---

func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"userId"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.UserID == "" {
		writeErr(w, http.StatusBadRequest, "userId required")
		return
	}
	name := req.Name
	if name == "" {
		name = req.UserID
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dev_user",
		Value:    req.UserID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 30,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "dev_name",
		Value:    name,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 30,
	})
	writeJSON(w, http.StatusOK, UserSummary{UserID: req.UserID, Name: name})
}

func (s *Server) handleDevLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "dev_user", Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "dev_name", Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUserMe(w http.ResponseWriter, r *http.Request) {
	id, _ := identityFrom(r.Context())
	writeJSON(w, http.StatusOK, UserSummary{UserID: id.UserID, Name: id.Name, Email: id.Email})
}

func (s *Server) handleUserGames(w http.ResponseWriter, r *http.Request) {
	id, _ := identityFrom(r.Context())
	user, err := s.deps.Store.GetUser(r.Context(), id.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]GameSummary, 0, len(user.GameIDs))
	for _, gid := range user.GameIDs {
		g, err := s.deps.Store.GetGame(r.Context(), gid)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(g.Players))
		yourTurn := false
		for i, p := range g.Players {
			if p.UserID != "" {
				names = append(names, p.Name)
			}
			if g.Status == game.StatusActive && i == g.Turn%len(g.Players) && p.UserID == id.UserID {
				yourTurn = true
			}
		}
		out = append(out, GameSummary{
			ID:          g.ID,
			Status:      g.Status,
			CreatedAt:   g.CreatedAt,
			PlayerNames: names,
			YourTurn:    yourTurn,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	id, _ := identityFrom(r.Context())
	if s.deps.Allowlist != nil && !s.deps.Allowlist.Contains(id.Email) {
		writeErr(w, http.StatusForbidden, "your account is not allowed to create games — please talk to the person who invited you to learn about getting access or self-hosting")
		return
	}
	var req CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Accept empty body (defaults to 2 players).
		req.NumPlayers = 2
	}
	if req.NumPlayers < 2 || req.NumPlayers > 4 {
		req.NumPlayers = 2
	}
	gameID := newID(16)
	invite := newInvite()
	g := game.NewGame(gameID, id.UserID, id.Name, invite, req.NumPlayers, s.deps.RandSeed(), s.deps.Now())
	if err := s.deps.Store.CreateGame(r.Context(), g); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.deps.Store.AddGameToUser(r.Context(), id.UserID, gameID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, CreateGameResponse{GameID: gameID, InviteCode: invite})
}

func (s *Server) handleJoinGame(w http.ResponseWriter, r *http.Request) {
	id, _ := identityFrom(r.Context())
	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.InviteCode = strings.TrimSpace(strings.ToUpper(req.InviteCode))
	if req.InviteCode == "" {
		writeErr(w, http.StatusBadRequest, "inviteCode required")
		return
	}
	g, err := s.deps.Store.FindGameByInvite(r.Context(), req.InviteCode)
	if err != nil {
		writeErr(w, http.StatusNotFound, "invite not found")
		return
	}
	// If the player is already in the game, just return the game view.
	alreadyIn := false
	for _, p := range g.Players {
		if p.UserID == id.UserID {
			alreadyIn = true
			break
		}
	}
	if alreadyIn {
		writeJSON(w, http.StatusOK, viewFor(g, id.UserID))
		return
	}
	updated, err := s.deps.Store.UpdateGame(r.Context(), g, func(g *game.Game) error {
		return g.AddPlayer(id.UserID, id.Name)
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.deps.Store.AddGameToUser(r.Context(), id.UserID, updated.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, viewFor(updated, id.UserID))

	// Notify existing players that someone joined (best-effort, non-blocking).
	if s.deps.Push != nil {
		for _, p := range updated.Players {
			if p.UserID == id.UserID {
				continue
			}
			go s.deps.Push.Notify(context.Background(), p.UserID, push.Notification{
				Title: "crossletters",
				Body:  id.Name + " joined your game!",
				URL:   "/?game=" + updated.ID,
			})
		}
	}
}

func (s *Server) handleGetGame(w http.ResponseWriter, r *http.Request) {
	id, _ := identityFrom(r.Context())
	gameID := r.PathValue("id")
	g, err := s.deps.Store.GetGame(r.Context(), gameID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	writeJSON(w, http.StatusOK, viewFor(g, id.UserID))
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	id, _ := identityFrom(r.Context())
	gameID := r.PathValue("id")
	var req PlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	g, err := s.deps.Store.GetGame(r.Context(), gameID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}

	var result *game.PlayResult
	updated, err := s.deps.Store.UpdateGame(r.Context(), g, func(g *game.Game) error {
		switch req.Type {
		case game.TurnPlay:
			res, err := g.Play(id.UserID, req.Placements, s.deps.Dict, s.deps.Now())
			if err != nil {
				return err
			}
			result = res
			return nil
		case game.TurnExchange:
			return g.Exchange(id.UserID, req.Exchange, s.deps.Now())
		case game.TurnPass:
			return g.Pass(id.UserID, s.deps.Now())
		default:
			return errors.New("invalid play type")
		}
	})
	if err != nil {
		if iw, ok := errors.AsType[*game.InvalidWordsError](err); ok {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error(), InvalidWords: iw.Words})
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, PlayResponse{Result: result, Game: viewFor(updated, id.UserID)})

	// Fire push notification to the next player (best-effort, non-blocking).
	if s.deps.Push != nil && updated.Status == game.StatusActive && len(updated.Players) > 0 {
		nextIdx := updated.Turn % len(updated.Players)
		nextPlayer := updated.Players[nextIdx]
		go s.deps.Push.Notify(context.Background(), nextPlayer.UserID, push.Notification{
			Title: "crossletters",
			Body:  id.Name + " played — it's your turn!",
			URL:   "/?game=" + gameID,
		})
	}
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	id, _ := identityFrom(r.Context())
	var sub push.Subscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid subscription")
		return
	}
	if err := s.deps.Store.SavePushSubscription(r.Context(), id.UserID, sub); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func newID(nBytes int) string {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

// newInvite produces a short, human-friendly uppercase code like "7K3QX9".
func newInvite() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I
	b := make([]byte, 6)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[n.Int64()]
	}
	return string(b)
}

func defaultRandSeed() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	var v int64
	for _, x := range b {
		v = v<<8 | int64(x)
	}
	return v
}
