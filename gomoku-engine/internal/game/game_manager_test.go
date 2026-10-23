package game

import (
	"testing"

	"gomoku-engine/internal/board"
	"gomoku-engine/internal/engine"
	"gomoku-engine/internal/renju"
)

func TestEachGameSessionOwnsSeparateTranspositionTable(t *testing.T) {
	m := &GameManager{games: make(map[string]*GameSession)}

	createSession := func() *GameSession {
		t.Helper()
		resp := make(chan jobResult, 1)
		m.processCreateGame(job{depth: 1, maxNodes: 100, resp: resp})
		result := <-resp
		if result.err != nil {
			t.Fatalf("processCreateGame failed: %v", result.err)
		}
		return m.games[result.state.GameID]
	}

	g1 := createSession()
	g2 := createSession()
	if g1.transpositionTable == nil || g2.transpositionTable == nil {
		t.Fatal("new game sessions must have a transposition table")
	}
	if g1.transpositionTable == g2.transpositionTable {
		t.Fatal("different game sessions must not share a transposition table")
	}

	table := g1.transpositionTable
	_ = engine.NewEngineWithTranspositionTable(g1.Config, g1.Depth, table)
	if g1.transpositionTable != table {
		t.Fatal("the session's transposition table pointer must remain stable")
	}
}

func TestBestMoveUsesAndRetainsSessionTranspositionTable(t *testing.T) {
	g := &GameSession{
		ID:                 "session-test",
		Board:              board.NewBoard(),
		Turn:               board.White,
		Config:             renju.NoRenjuConfig(),
		Depth:              2,
		MaxNodes:           5000,
		Status:             StatusPlaying,
		transpositionTable: engine.NewTranspositionTable(0),
	}
	g.Board.Set(7, 7, board.Black)

	m := &GameManager{games: map[string]*GameSession{g.ID: g}}
	table := g.transpositionTable
	resp := make(chan jobResult, 1)
	m.processBestMove(job{gameID: g.ID, place: true, resp: resp})
	result := <-resp
	if result.err != nil {
		t.Fatalf("processBestMove failed: %v", result.err)
	}
	if g.transpositionTable != table {
		t.Fatal("best-move search replaced the session's transposition table")
	}
	if _, ok := table.Probe(g.Board.HashForTurn(g.Turn)); !ok {
		t.Fatal("the position reached by the computer move was not retained in the session table")
	}
}
