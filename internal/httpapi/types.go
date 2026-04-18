package httpapi

import (
	"time"

	"github.com/alan/not-scrabble/internal/game"
)

// GameView is the redacted game state returned to a player. Other players'
// racks are hidden; only tile counts are exposed.
type GameView struct {
	ID            string             `json:"id"`
	CreatorID     string             `json:"creatorId"`
	InviteCode    string             `json:"inviteCode"`
	Status        game.Status        `json:"status"`
	NumPlayers    int                `json:"numPlayers"`
	OpenSeats     int                `json:"openSeats"`
	CreatedAt     time.Time          `json:"createdAt"`
	StartedAt     *time.Time         `json:"startedAt,omitempty"`
	EndedAt       *time.Time         `json:"endedAt,omitempty"`
	Players       []PlayerView       `json:"players"`
	Turn          int                `json:"turn"`
	CurrentIdx    int                `json:"currentIdx"`
	Board         *game.Board        `json:"board"`
	BagSize       int                `json:"bagSize"`
	History       []game.TurnRecord  `json:"history"`
	Winners       []int              `json:"winners,omitempty"`
	YourPlayerIdx int                `json:"yourPlayerIdx"` // -1 if not a player
	LastPlay      *game.TurnRecord   `json:"lastPlay,omitempty"`
}

// PlayerView is a player's public-facing state. Rack is only populated for the
// viewer.
type PlayerView struct {
	UserID   string        `json:"userId"`
	Name     string        `json:"name"`
	Score    int           `json:"score"`
	Rack     []game.Letter `json:"rack,omitempty"`
	RackSize int           `json:"rackSize"`
}

// viewFor returns a GameView with the given user's perspective.
func viewFor(g *game.Game, userID string) *GameView {
	v := &GameView{
		ID:         g.ID,
		CreatorID:  g.CreatorID,
		InviteCode: g.InviteCode,
		Status:     g.Status,
		NumPlayers: g.NumPlayers,
		OpenSeats:  g.OpenSeats(),
		CreatedAt:  g.CreatedAt,
		StartedAt:  g.StartedAt,
		EndedAt:    g.EndedAt,
		Turn:       g.Turn,
		Board:      g.Board,
		BagSize:    len(g.Bag),
		History:    g.History,
		Winners:    g.Winners,
	}
	v.YourPlayerIdx = -1
	for i, p := range g.Players {
		pv := PlayerView{
			UserID:   p.UserID,
			Name:     p.Name,
			Score:    p.Score,
			RackSize: len(p.Rack),
		}
		// Only show rack to the owning player; hide open seats' racks.
		if p.UserID != "" && p.UserID == userID {
			v.YourPlayerIdx = i
			pv.Rack = append([]game.Letter(nil), p.Rack...)
		}
		v.Players = append(v.Players, pv)
	}
	if len(g.Players) > 0 {
		v.CurrentIdx = g.Turn % len(g.Players)
	}
	if n := len(g.History); n > 0 {
		last := g.History[n-1]
		v.LastPlay = &last
	}
	return v
}

// CreateGameRequest is the body of POST /api/games.
type CreateGameRequest struct {
	NumPlayers int `json:"numPlayers"`
}

// CreateGameResponse is returned by POST /api/games.
type CreateGameResponse struct {
	GameID     string `json:"gameId"`
	InviteCode string `json:"inviteCode"`
}

// JoinRequest is the body of POST /api/games/join.
type JoinRequest struct {
	InviteCode string `json:"inviteCode"`
}

// PlayRequest is the body of POST /api/games/{id}/plays.
type PlayRequest struct {
	Type       game.TurnType     `json:"type"` // "play", "exchange", "pass"
	Placements []game.Placement  `json:"placements,omitempty"`
	Exchange   []game.Letter     `json:"exchange,omitempty"`
}

// PlayResponse is the success response for a play.
type PlayResponse struct {
	Result *game.PlayResult `json:"result,omitempty"`
	Game   *GameView        `json:"game"`
}

// ErrorResponse is a JSON error.
type ErrorResponse struct {
	Error        string   `json:"error"`
	InvalidWords []string `json:"invalidWords,omitempty"`
}

// UserSummary is the current-user response.
type UserSummary struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Email  string `json:"email,omitempty"`
}

// GameSummary is one entry in the user's games list.
type GameSummary struct {
	ID         string      `json:"id"`
	Status     game.Status `json:"status"`
	CreatedAt  time.Time   `json:"createdAt"`
	PlayerNames []string   `json:"playerNames"`
	YourTurn   bool        `json:"yourTurn"`
}
