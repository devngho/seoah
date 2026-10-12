package game

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gomoku-engine/internal/board"
	"gomoku-engine/internal/config"
	"gomoku-engine/internal/engine"
	"gomoku-engine/internal/renju"
)

type MoveRecord struct {
	Row   int         `json:"row"`
	Col   int         `json:"col"`
	Color board.Stone `json:"color"`
}

type GameStatus string

const (
	StatusPlaying GameStatus = "playing"
	StatusWin     GameStatus = "win"
	StatusDraw    GameStatus = "draw"
)

type jobKind int

const (
	jobCreateGame jobKind = iota
	jobGetState
	jobPlaceMove
	jobBestMove
)

type job struct {
	kind     jobKind
	gameID   string
	renjuCfg *renju.RenjuConfig
	softmax  engine.SoftmaxTopPConfig
	depth    int
	maxNodes int64
	row, col int
	place    bool

	resp chan jobResult
}

type jobResult struct {
	state       GameStateDTO
	bestY       int
	bestX       int
	bestScore   int
	hasBestMove bool
	err         error
}

type GameSession struct {
	ID        string
	Board     *board.Board
	Turn      board.Stone
	Config    renju.RenjuConfig
	Softmax   engine.SoftmaxTopPConfig
	Depth     int
	MaxNodes  int64
	History   []MoveRecord
	Status    GameStatus
	Winner    board.Stone
	CreatedAt time.Time
}

type GameStateDTO struct {
	GameID         string       `json:"gameId"`
	Board          [][]int      `json:"board"`
	Turn           int          `json:"turn"`
	Status         string       `json:"status"`
	Winner         int          `json:"winner,omitempty"`
	History        []MoveRecord `json:"history"`
	ForbiddenMoves [][2]int     `json:"forbiddenMoves"`
}

type GameManager struct {
	games map[string]*GameSession
	queue chan job
}

func (m *GameManager) run() {
	for j := range m.queue {
		m.process(j)
	}
}

func (m *GameManager) process(j job) {
	switch j.kind {
	case jobCreateGame:
		m.processCreateGame(j)
	case jobGetState:
		m.processGetState(j)
	case jobPlaceMove:
		m.processPlaceMove(j)
	case jobBestMove:
		m.processBestMove(j)
	}
}

func (m *GameManager) processCreateGame(j job) {
	cfg := renju.DefaultRenjuConfig()
	if j.renjuCfg != nil {
		cfg = *j.renjuCfg
	}
	depth := config.DefaultSearchDepth
	if j.depth > 0 {
		depth = j.depth
	}
	maxNodes := int64(config.DefaultMaxNodes)
	if j.maxNodes > 0 {
		maxNodes = j.maxNodes
	}

	g := &GameSession{
		ID:        newGameID(),
		Board:     board.NewBoard(),
		Turn:      board.Black,
		Config:    cfg,
		Softmax:   j.softmax,
		Depth:     depth,
		MaxNodes:  maxNodes,
		Status:    StatusPlaying,
		CreatedAt: time.Now(),
	}
	m.games[g.ID] = g
	j.resp <- jobResult{state: g.toDTO()}
}

func (m *GameManager) lookup(gameID string) (*GameSession, error) {
	g, ok := m.games[gameID]
	if !ok {
		return nil, fmt.Errorf("게임을 찾을 수 없습니다: %s", gameID)
	}
	return g, nil
}

func (m *GameManager) processGetState(j job) {
	g, err := m.lookup(j.gameID)
	if err != nil {
		j.resp <- jobResult{err: err}
		return
	}
	j.resp <- jobResult{state: g.toDTO()}
}

func (m *GameManager) processPlaceMove(j job) {
	g, err := m.lookup(j.gameID)
	if err != nil {
		j.resp <- jobResult{err: err}
		return
	}
	if err := applyMove(g, j.row, j.col, g.Turn); err != nil {
		j.resp <- jobResult{err: err}
		return
	}
	j.resp <- jobResult{state: g.toDTO()}
}

func (m *GameManager) processBestMove(j job) {
	g, err := m.lookup(j.gameID)
	if err != nil {
		j.resp <- jobResult{err: err}
		return
	}
	if g.Status != StatusPlaying {
		j.resp <- jobResult{err: fmt.Errorf("이미 종료된 게임입니다")}
		return
	}

	eng := engine.NewEngine(g.Config, g.Depth)
	eng.SetSoftmaxTopP(g.Softmax)
	eng.MaxNodes = g.MaxNodes
	y, x, score := eng.FindBestMove(g.Board, g.Turn)

	if y == -1 {
		// 둘 수 있는 합법 수가 없는 극단적인 경우 -> 무승부로 종료
		g.Status = StatusDraw
		j.resp <- jobResult{state: g.toDTO()}
		return
	}

	if !j.place {
		// place=false: 실제로 두지 않고 추천 좌표만 응답
		j.resp <- jobResult{state: g.toDTO(), bestY: y, bestX: x, bestScore: score, hasBestMove: true}
		return
	}

	if err := applyMove(g, y, x, g.Turn); err != nil {
		j.resp <- jobResult{err: err}
		return
	}
	j.resp <- jobResult{state: g.toDTO(), bestY: y, bestX: x, bestScore: score, hasBestMove: true}
}

// applyMove: 검증(범위/중복/금수/게임 종료 여부) 후 실제로 착수하고,
// 승리/무승부/턴 전환까지 반영한다. 워커 goroutine에서만 호출되므로 락이 필요 없다.
func applyMove(g *GameSession, y, x int, color board.Stone) error {
	if g.Status != StatusPlaying {
		return fmt.Errorf("이미 종료된 게임입니다")
	}
	if !board.InBounds(y, x) {
		return fmt.Errorf("보드 범위를 벗어났습니다: (%d, %d)", y, x)
	}
	if g.Board.Get(y, x) != board.Empty {
		return fmt.Errorf("이미 돌이 있는 자리입니다: (%d, %d)", y, x)
	}
	if engine.IsForbidden(g.Board, y, x, color, g.Config) {
		return fmt.Errorf("렌주 금수 자리입니다: (%d, %d)", y, x)
	}

	g.Board.Set(y, x, color)
	g.History = append(g.History, MoveRecord{Row: y, Col: x, Color: color})

	if engine.CheckWinAt(g.Board, y, x, color) {
		g.Status = StatusWin
		g.Winner = color
		return nil
	}
	if isBoardFull(g.Board) {
		g.Status = StatusDraw
		return nil
	}
	g.Turn = board.Opponent(color)
	return nil
}

// submit: 요청을 큐에 넣고 워커가 처리를 마칠 때까지 블로킹으로 기다린다.
func (m *GameManager) submit(j job) jobResult {
	j.resp = make(chan jobResult, 1)
	m.queue <- j
	return <-j.resp
}

// ============================================================
// 외부(HTTP 계층)용 공개 API
// job 타입과 submit은 패키지 내부 전용이므로, main 패키지에서는
// 아래 메서드들만 호출하면 된다.
// ============================================================

// CreateGame: 새 게임을 만들고 초기 상태를 돌려준다.
// 설정값을 주지 않으면 해당 설정을 기본값으로 설정
func (m *GameManager) CreateGame(cfg *renju.RenjuConfig, softmax engine.SoftmaxTopPConfig, depth int, maxNodes int64) (GameStateDTO, error) {
	res := m.submit(job{kind: jobCreateGame, renjuCfg: cfg, softmax: softmax, depth: depth, maxNodes: maxNodes})
	return res.state, res.err
}

// GetState: 현재 게임 상태 조회.
func (m *GameManager) GetState(gameID string) (GameStateDTO, error) {
	res := m.submit(job{kind: jobGetState, gameID: gameID})
	return res.state, res.err
}

// PlaceMove: 사람 플레이어 착수.
func (m *GameManager) PlaceMove(gameID string, row, col int) (GameStateDTO, error) {
	res := m.submit(job{kind: jobPlaceMove, gameID: gameID, row: row, col: col})
	return res.state, res.err
}

// BestMove: 최선수 계산. place=true면 서버가 실제로 착수까지 한다.
func (m *GameManager) BestMove(gameID string, place bool) (BestMoveDTO, error) {
	res := m.submit(job{kind: jobBestMove, gameID: gameID, place: place})
	if res.err != nil {
		return BestMoveDTO{}, res.err
	}
	out := BestMoveDTO{GameStateDTO: res.state}
	if res.hasBestMove {
		out.Move = &MoveDTO{Y: res.bestY, X: res.bestX, Score: res.bestScore}
	}
	return out, nil
}

func newGameID() string {
	buf := make([]byte, 8)
	if _, err := cryptorand.Read(buf); err != nil {
		// crypto/rand 실패는 사실상 일어나지 않지만 방어적으로 시간 기반 fallback을 둔다.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func NewGameManager() *GameManager {
	m := &GameManager{
		games: make(map[string]*GameSession),
		queue: make(chan job, 256),
	}
	go m.run()
	return m
}

// ============================================================
// 게임 세션 (서버가 게임의 모든 상태를 관리)
// ============================================================

// isBoardFull: 보드에 빈 칸이 하나도 없으면 true (무승부 판정용)
func isBoardFull(b *board.Board) bool {
	for y := 0; y < config.BoardSize; y++ {
		for x := 0; x < config.BoardSize; x++ {
			if b.Get(y, x) == board.Empty {
				return false
			}
		}
	}
	return true
}

func boardToDTO(b *board.Board) [][]int {
	out := make([][]int, config.BoardSize)
	for y := 0; y < config.BoardSize; y++ {
		out[y] = make([]int, config.BoardSize)
		for x := 0; x < config.BoardSize; x++ {
			out[y][x] = int(b.Get(y, x))
		}
	}
	return out
}

// toDTO: 현재 세션 상태를 JSON 응답용 스냅샷으로 변환.
// 흑 차례이고 렌주룰이 켜져 있을 때만 금수 좌표 목록을 계산해서 함께 내려준다
// (예전에는 웹의 engine.ts가 매 렌더마다 이걸 직접 계산했는데, 이제 서버가
// 유일한 판정 주체이므로 여기서 계산해서 내려준다).
func (g *GameSession) toDTO() GameStateDTO {
	forbidden := make([][2]int, 0)
	if g.Status == StatusPlaying && g.Turn == board.Black && g.Config.Enabled {
		for y := 0; y < config.BoardSize; y++ {
			for x := 0; x < config.BoardSize; x++ {
				if g.Board.Get(y, x) == board.Empty && engine.IsForbidden(g.Board, y, x, board.Black, g.Config) {
					forbidden = append(forbidden, [2]int{y, x})
				}
			}
		}
	}
	history := g.History
	if history == nil {
		history = []MoveRecord{}
	}
	return GameStateDTO{
		GameID:         g.ID,
		Board:          boardToDTO(g.Board),
		Turn:           int(g.Turn),
		Status:         string(g.Status),
		Winner:         int(g.Winner),
		History:        history,
		ForbiddenMoves: forbidden,
	}
}

// RenjuConfigDTO: 게임 생성 요청에서 렌주 규칙을 선택적으로 오버라이드할 때 사용.
// 생략하면(=nil) 서버 기본값(DefaultRenjuConfig)을 사용합니다.
type RenjuConfigDTO struct {
	Enabled           bool `json:"enabled"`
	ForbidDoubleThree bool `json:"forbidDoubleThree"`
	ForbidDoubleFour  bool `json:"forbidDoubleFour"`
	ForbidOverline    bool `json:"forbidOverline"`
}

// ToConfig: 요청 DTO를 엔진이 쓰는 렌주 설정으로 변환.
func (d RenjuConfigDTO) ToConfig() renju.RenjuConfig {
	return renju.RenjuConfig{
		Enabled:           d.Enabled,
		ForbidDoubleThree: d.ForbidDoubleThree,
		ForbidDoubleFour:  d.ForbidDoubleFour,
		ForbidOverline:    d.ForbidOverline,
	}
}

func (d *SoftmaxTopPDTO) ToConfig() (engine.SoftmaxTopPConfig, error) {
	cfg := engine.SoftmaxTopPConfig{
		Enabled:     config.DefaultSoftmaxEnabled,
		Temperature: config.DefaultSoftmaxTemperature,
		TopP:        config.DefaultSoftmaxTopP,
		Seed:        config.DefaultSoftmaxSeed,
	}
	if d == nil {
		return cfg, nil
	}
	if d.Enabled != nil {
		cfg.Enabled = *d.Enabled
	}
	if d.Temperature != nil {
		if *d.Temperature <= 0 {
			return cfg, fmt.Errorf("temperature는 0보다 커야 합니다: %v", *d.Temperature)
		}
		cfg.Temperature = *d.Temperature
	}
	if d.TopP != nil {
		if *d.TopP <= 0 || *d.TopP > 1 {
			return cfg, fmt.Errorf("topP는 0 초과 1 이하여야 합니다: %v", *d.TopP)
		}
		cfg.TopP = *d.TopP
	}
	if d.Seed != nil {
		cfg.Seed = *d.Seed
	}
	return cfg, nil
}

type ErrorDTO struct {
	Error string `json:"error"`
}

type BestMoveDTO struct {
	GameStateDTO
	Move *MoveDTO `json:"move,omitempty"`
}

type MoveDTO struct {
	Y     int `json:"y"`
	X     int `json:"x"`
	Score int `json:"score"`
}

type SoftmaxTopPDTO struct {
	Enabled     *bool    `json:"enabled"`
	Temperature *float64 `json:"temperature"`
	TopP        *float64 `json:"topP"`
	Seed        *int64   `json:"seed"`
}
