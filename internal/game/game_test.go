package game

import (
	"errors"
	"testing"
	"time"
)

// mockDict accepts any non-empty list of provided words.
type mockDict map[string]bool

func (m mockDict) Contains(w string) bool { return m[w] }

func dict(words ...string) mockDict {
	d := mockDict{}
	for _, w := range words {
		d[w] = true
	}
	return d
}

func placement(row, col int, letter Letter, blank bool) Placement {
	return Placement{Row: row, Col: col, Letter: letter, Blank: blank}
}

// placeWord puts pre-set tiles directly on the board; used only in tests to
// establish state without going through Play.
func placeWord(b *Board, row, col int, horiz bool, word string, blanks ...int) {
	blankSet := map[int]bool{}
	for _, i := range blanks {
		blankSet[i] = true
	}
	for i := 0; i < len(word); i++ {
		r, c := row, col
		if horiz {
			c += i
		} else {
			r += i
		}
		b.Squares[r][c] = &PlacedTile{Letter: Letter(word[i]), Blank: blankSet[i]}
	}
}

// --- Bag ---

func TestBagHas100Tiles(t *testing.T) {
	b := NewBag(1)
	if len(b) != 100 {
		t.Fatalf("bag has %d tiles, want 100", len(b))
	}
	counts := map[Letter]int{}
	for _, l := range b {
		counts[l]++
	}
	for l, want := range LetterCounts {
		if counts[l] != want {
			t.Errorf("letter %q: got %d, want %d", string(l), counts[l], want)
		}
	}
}

func TestBagDeterministic(t *testing.T) {
	a := NewBag(42)
	b := NewBag(42)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("bag[%d] differs with same seed", i)
		}
	}
	c := NewBag(43)
	same := true
	for i := range a {
		if a[i] != c[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different seeds produced identical bag order")
	}
}

// --- Scoring ---

func TestFirstMoveMustCoverCenter(t *testing.T) {
	g := testGame(t, []Letter{'C', 'A', 'T', 'S', 'E', 'E', 'R'})
	// CAT at row 0
	_, err := g.Play("p1", []Placement{
		placement(0, 0, 'C', false),
		placement(0, 1, 'A', false),
		placement(0, 2, 'T', false),
	}, dict("CAT"), time.Now())
	if err == nil || !contains(err.Error(), "center") {
		t.Fatalf("expected center error, got %v", err)
	}
}

func TestBasicPlayAndScore(t *testing.T) {
	// CAT placed horizontally through center.
	// Expected: C=3, A=1, T=1 = 5. Center is DW -> 10.
	g := testGame(t, []Letter{'C', 'A', 'T', 'S', 'E', 'E', 'R'})
	res, err := g.Play("p1", []Placement{
		placement(7, 6, 'C', false),
		placement(7, 7, 'A', false),
		placement(7, 8, 'T', false),
	}, dict("CAT"), time.Now())
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if res.Score != 10 {
		t.Errorf("score = %d, want 10", res.Score)
	}
	if g.Players[0].Score != 10 {
		t.Errorf("player score = %d, want 10", g.Players[0].Score)
	}
	// Turn advances to the open seat; p1 can't play again until someone joins.
	if g.CurrentPlayer().UserID != "" {
		t.Errorf("expected turn on open seat, got %s", g.CurrentPlayer().UserID)
	}
	// Verify p1 is blocked from playing.
	_, err = g.Play("p1", []Placement{
		placement(7, 9, 'S', false),
	}, dict("CATS"), time.Now())
	if err == nil || !contains(err.Error(), "waiting") {
		t.Errorf("expected waiting error, got %v", err)
	}
	if len(g.Players[0].Rack) != 7 {
		t.Errorf("rack not refilled: len = %d", len(g.Players[0].Rack))
	}
}

func TestBingoBonus(t *testing.T) {
	// 7-tile play gets +50. REFOUND at row 7 cols 4..10.
	g := testGame(t, []Letter{'R', 'E', 'F', 'O', 'U', 'N', 'D'})
	res, err := g.Play("p1", []Placement{
		placement(7, 4, 'R', false),
		placement(7, 5, 'E', false),
		placement(7, 6, 'F', false),
		placement(7, 7, 'O', false),
		placement(7, 8, 'U', false),
		placement(7, 9, 'N', false),
		placement(7, 10, 'D', false),
	}, dict("REFOUND"), time.Now())
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if !res.Bingo {
		t.Error("expected bingo=true")
	}
	// Premiums on row 7: DL at col 3 and 11 only; center (7,7)=DW; TW at 0,14.
	// So only (7,7) is hit. Letters R1+E1+F4+O1+U1+N1+D2=11. Word x2 = 22. +50 bingo = 72.
	if res.Score != 72 {
		t.Errorf("bingo score = %d, want 72", res.Score)
	}
}

func TestBlankScoresZero(t *testing.T) {
	// QA with Q as real Q (10) and A as blank. 10 + 0 = 10; center DW -> 20.
	g := testGame(t, []Letter{'Q', '?', 'E', 'E', 'R', 'T', 'U'})
	res, err := g.Play("p1", []Placement{
		placement(7, 7, 'Q', false),
		placement(7, 8, 'A', true),
	}, dict("QA"), time.Now())
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	// (7,7) DW. Q=10, A(blank)=0. Sum=10. Word x2 = 20.
	if res.Score != 20 {
		t.Errorf("score = %d, want 20", res.Score)
	}
	// Rack should have used the blank, not a real A.
	for _, l := range g.Players[0].Rack {
		if l == Blank {
			t.Error("blank not consumed from rack")
		}
	}
}

func TestCrossWordsScored(t *testing.T) {
	// Pre-place HI horizontally at (7,7-8). Play AT below, forming cross-words
	// HA at col 7 and IT at col 8.
	g := testGame(t, []Letter{'A', 'T', 'X', 'X', 'X', 'X', 'X'})
	placeWord(g.Board, 7, 7, true, "HI")
	g.Status = StatusActive
	res, err := g.Play("p1", []Placement{
		placement(8, 7, 'A', false),
		placement(8, 8, 'T', false),
	}, dict("AT", "HA", "IT"), time.Now())
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	// AT main (row 8): (8,7) none, (8,8) DL. A(1) + T(1*2) = 3.
	// HA cross (col 7): H pre-placed (4), A new at (8,7) no prem (1). Sum 5.
	// IT cross (col 8): I pre-placed (1), T new at (8,8) DL (1*2). Sum 3.
	// Total: 3 + 5 + 3 = 11.
	if res.Score != 11 {
		t.Errorf("score = %d, want 11", res.Score)
	}
	if len(res.Words) != 3 {
		t.Errorf("words formed = %d, want 3", len(res.Words))
	}
}

func TestPremiumConsumedOnce(t *testing.T) {
	// Establish CAT with C on TW at (0,0)? Actually let's test DW-not-reused.
	// Play CAT covering center DW, then play S at col 10 -> CATS only uses new
	// premium at (7,10) which has none -> score 6 (checked above).
	// This test verifies that a second play through center does NOT re-apply DW.
	g := testGame(t, []Letter{'S', 'E', 'X', 'X', 'X', 'X', 'X'})
	placeWord(g.Board, 7, 6, true, "CAT") // spans cols 6,7,8; center (7,7) = A
	g.Status = StatusActive
	// Place S at (7,9) forming CATS. (7,9) has no premium. Score should be C3+A1+T1+S1 = 6.
	res, err := g.Play("p1", []Placement{
		placement(7, 9, 'S', false),
	}, dict("CATS"), time.Now())
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if res.Score != 6 {
		t.Errorf("score = %d, want 6 (center premium must not re-apply)", res.Score)
	}
}

func TestPlayMustConnect(t *testing.T) {
	g := testGame(t, []Letter{'D', 'O', 'G', 'X', 'X', 'X', 'X'})
	placeWord(g.Board, 7, 6, true, "CAT")
	g.Status = StatusActive
	_, err := g.Play("p1", []Placement{
		placement(0, 0, 'D', false),
		placement(0, 1, 'O', false),
		placement(0, 2, 'G', false),
	}, dict("DOG"), time.Now())
	if err == nil || !contains(err.Error(), "connect") {
		t.Fatalf("want connect error, got %v", err)
	}
}

func TestGapInWordRejected(t *testing.T) {
	g := testGame(t, []Letter{'C', 'T', 'X', 'X', 'X', 'X', 'X'})
	// C at center, T two squares away with gap in between. No tile in between.
	_, err := g.Play("p1", []Placement{
		placement(7, 7, 'C', false),
		placement(7, 9, 'T', false),
	}, dict("CAT"), time.Now())
	if err == nil || !contains(err.Error(), "gap") {
		t.Fatalf("want gap error, got %v", err)
	}
}

func TestInvalidWordRejected(t *testing.T) {
	g := testGame(t, []Letter{'X', 'Q', 'Z', 'S', 'E', 'E', 'R'})
	_, err := g.Play("p1", []Placement{
		placement(7, 6, 'X', false),
		placement(7, 7, 'Q', false),
		placement(7, 8, 'Z', false),
	}, dict("CAT"), time.Now())
	var iw *InvalidWordsError
	if !errors.As(err, &iw) {
		t.Fatalf("want InvalidWordsError, got %v", err)
	}
	if len(iw.Words) == 0 || iw.Words[0] != "XQZ" {
		t.Errorf("words = %v, want [XQZ]", iw.Words)
	}
}

func TestCrossWordValidated(t *testing.T) {
	// Pre-place CAT horizontally. Append S below A to make CAT + AS (cross).
	// AS must be a valid word.
	g := testGame(t, []Letter{'S', 'X', 'X', 'X', 'X', 'X', 'X'})
	placeWord(g.Board, 7, 6, true, "CAT") // A is at (7,7)
	g.Status = StatusActive
	// Place S at (8,7) -> main word is vertical AS (cells (7,7)A and (8,7)S).
	// That's the only word formed; single-tile play so main is whichever dir gives >=2.
	res, err := g.Play("p1", []Placement{
		placement(8, 7, 'S', false),
	}, dict("AS"), time.Now())
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	// AS: A1+S1 = 2. (8,7) no premium.
	if res.Score != 2 {
		t.Errorf("score = %d, want 2", res.Score)
	}
	// Should fail without AS in dict.
	g2 := testGame(t, []Letter{'S', 'X', 'X', 'X', 'X', 'X', 'X'})
	placeWord(g2.Board, 7, 6, true, "CAT")
	g2.Status = StatusActive
	_, err = g2.Play("p1", []Placement{placement(8, 7, 'S', false)}, dict("CAT"), time.Now())
	var iw *InvalidWordsError
	if !errors.As(err, &iw) {
		t.Fatalf("want invalid-word error, got %v", err)
	}
}

// --- Turn machinery ---

func TestTurnAdvances(t *testing.T) {
	g := testTwoPlayerGame(t)
	if g.CurrentPlayer().UserID != "p1" {
		t.Fatalf("expected p1 first, got %s", g.CurrentPlayer().UserID)
	}
	err := g.Pass("p1", time.Now())
	if err != nil {
		t.Fatalf("Pass: %v", err)
	}
	if g.CurrentPlayer().UserID != "p2" {
		t.Errorf("expected p2 after pass, got %s", g.CurrentPlayer().UserID)
	}
	// Not p1's turn now.
	err = g.Pass("p1", time.Now())
	if err == nil {
		t.Error("expected not-your-turn error")
	}
}

func TestExchangeRequires7InBag(t *testing.T) {
	g := testTwoPlayerGame(t)
	g.Bag = g.Bag[:6] // fewer than RackSize
	err := g.Exchange("p1", []Letter{g.Players[0].Rack[0]}, time.Now())
	if err == nil {
		t.Error("expected exchange rejection when bag < 7")
	}
}

func TestGameEndsWhenPlayerGoesOut(t *testing.T) {
	g := testTwoPlayerGame(t)
	// Empty the bag and give p1 exactly one tile that forms a valid word
	// through the center.
	g.Bag = nil
	g.Players[0].Rack = []Letter{'A'}
	g.Players[1].Rack = []Letter{'B', 'C'}
	// Need a tile already on the board so A connects; place an X at center,
	// then play A to the right making XA.
	g.Board.Squares[7][7] = &PlacedTile{Letter: 'X'}
	_, err := g.Play("p1", []Placement{placement(7, 8, 'A', false)}, dict("XA"), time.Now())
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if g.Status != StatusCompleted {
		t.Fatalf("game should be completed, status=%s", g.Status)
	}
	// p2 had BC (3+3 = 6) remaining -> subtracted from p2 and added to p1.
	// p1 score = AX with X on double-word-side... wait X at (7,7) already placed.
	// Let's just check the delta accounting.
	// Actually center was pre-filled so DW doesn't re-apply. XA: X8+A1 = 9. No premium.
	// p1 initial = 0, gains 9 from play, then +6 from p2's rack = 15.
	// p2 = 0 - 6 = -6.
	if g.Players[0].Score != 15 {
		t.Errorf("p1 score = %d, want 15", g.Players[0].Score)
	}
	if g.Players[1].Score != -6 {
		t.Errorf("p2 score = %d, want -6", g.Players[1].Score)
	}
	if len(g.Winners) != 1 || g.Winners[0] != 0 {
		t.Errorf("winners = %v, want [0]", g.Winners)
	}
}

// --- helpers ---

func testGame(t *testing.T, rack []Letter) *Game {
	t.Helper()
	g := NewGame("g1", "p1", "Alice", "INV", 2, 1, time.Now())
	g.Players[0].Rack = rack
	return g
}

func testTwoPlayerGame(t *testing.T) *Game {
	t.Helper()
	g := NewGame("g1", "p1", "Alice", "INV", 2, 7, time.Now())
	if err := g.AddPlayer("p2", "Bob"); err != nil {
		t.Fatal(err)
	}
	return g
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
