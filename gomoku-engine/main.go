package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
)

// ============================================================
// Stone / Board
// ============================================================

type Stone int

const (
	Empty Stone = iota
	Black
	White
)

const boardSize = 15
const winLength = 5

type Board struct {
	cells [boardSize][boardSize]Stone
}

func NewBoard() *Board {
	return &Board{}
}

func (b *Board) Get(y, x int) Stone {
	return b.cells[y][x]
}

func (b *Board) Set(y, x int, s Stone) {
	b.cells[y][x] = s
}

func inBounds(y, x int) bool {
	return y >= 0 && y < boardSize && x >= 0 && x < boardSize
}

func opponent(s Stone) Stone {
	if s == Black {
		return White
	}
	return Black
}

// {dx, dy}: 가로, 세로, 대각선(↘), 대각선(↙)
var directions = [4][2]int{
	{1, 0},
	{0, 1},
	{1, 1},
	{1, -1},
}

// ============================================================
// Renju 금수 규칙 설정 (on/off 토글)
// ============================================================

// RenjuConfig: Enabled를 false로 하면 모든 금수 규칙이 꺼지고 자유 오목이 됩니다.
// 전통 렌주룰은 흑(Black)에게만 적용됩니다.
type RenjuConfig struct {
	Enabled           bool // 마스터 스위치
	ForbidDoubleThree bool // 3-3 금수
	ForbidDoubleFour  bool // 4-4 금수
	ForbidOverline    bool // 장목(6목 이상) 금수
}

func DefaultRenjuConfig() RenjuConfig {
	return RenjuConfig{
		Enabled:           true,
		ForbidDoubleThree: true,
		ForbidDoubleFour:  true,
		ForbidOverline:    true,
	}
}

func NoRenjuConfig() RenjuConfig {
	return RenjuConfig{Enabled: false}
}

// ============================================================
// 라인 분석 유틸
// ============================================================

// runLength: (y,x)에 stone이 이미 놓여있다고 가정하고, (dx,dy) 방향 양쪽으로
// 이어지는 연속된 길이를 반환합니다.
func runLength(b *Board, y, x, dx, dy int, stone Stone) int {
	length := 1
	cy, cx := y+dy, x+dx
	for inBounds(cy, cx) && b.Get(cy, cx) == stone {
		length++
		cy += dy
		cx += dx
	}
	cy, cx = y-dy, x-dx
	for inBounds(cy, cx) && b.Get(cy, cx) == stone {
		length++
		cy -= dy
		cx -= dx
	}
	return length
}

func maxRunLength(b *Board, y, x int, stone Stone) int {
	max := 0
	for _, d := range directions {
		l := runLength(b, y, x, d[0], d[1], stone)
		if l > max {
			max = l
		}
	}
	return max
}

// CheckWinAt: (y,x)에 stone을 놓아 정확히 5목 이상을 완성했는지 확인 (승리 판정용)
func CheckWinAt(b *Board, y, x int, stone Stone) bool {
	for _, d := range directions {
		if runLength(b, y, x, d[0], d[1], stone) >= winLength {
			return true
		}
	}
	return false
}

// lineToString: (y,x) 중심, (dx,dy) 방향으로 radius칸씩 문자열로 추출.
// 'S' = 해당 stone, '_' = 빈칸, 'o' = 상대 돌 또는 보드 밖(막힘)
func lineToString(b *Board, y, x, dx, dy, radius int, stone Stone) string {
	var sb strings.Builder
	for i := -radius; i <= radius; i++ {
		cy, cx := y+dy*i, x+dx*i
		if !inBounds(cy, cx) {
			sb.WriteByte('o')
			continue
		}
		c := b.Get(cy, cx)
		switch c {
		case stone:
			sb.WriteByte('S')
		case Empty:
			sb.WriteByte('_')
		default:
			sb.WriteByte('o')
		}
	}
	return sb.String()
}

// countOpenThrees: (y,x)에 stone을 놓았을 때 만들어지는 "열린 3" 개수.
// 간이 판정: 9칸 윈도우 안에 "_SSS_" 패턴이 있으면 열린 3으로 간주.
// (완전한 렌주룰의 "살아있는 3" 판정보다는 단순화된 근사치입니다)
func countOpenThrees(b *Board, y, x int, stone Stone) int {
	count := 0
	for _, d := range directions {
		line := lineToString(b, y, x, d[0], d[1], 4, stone)
		if strings.Contains(line, "_SSS_") {
			count++
		}
	}
	return count
}

// countFours: (y,x)에 stone을 놓았을 때 만들어지는 "4" 위협 개수.
// 5칸 윈도우 안에 S가 4개, 빈칸이 1개면 (한 수로 5목 완성 가능) 4로 간주.
func countFours(b *Board, y, x int, stone Stone) int {
	count := 0
	for _, d := range directions {
		line := lineToString(b, y, x, d[0], d[1], 4, stone)
		found := false
		for i := 0; i+5 <= len(line); i++ {
			w := line[i : i+5]
			if strings.Count(w, "S") == 4 && strings.Count(w, "_") == 1 {
				found = true
				break
			}
		}
		if found {
			count++
		}
	}
	return count
}

// IsForbidden: (y,x)에 stone을 놓는 것이 렌주 금수인지 확인.
// 전통 렌주룰에 따라 Black에게만 적용됩니다. cfg.Enabled가 false면 항상 false.
func IsForbidden(b *Board, y, x int, stone Stone, cfg RenjuConfig) bool {
	if !cfg.Enabled || stone != Black {
		return false
	}
	if b.Get(y, x) != Empty {
		return false
	}

	b.Set(y, x, stone)
	defer b.Set(y, x, Empty)

	run := maxRunLength(b, y, x, stone)

	// 정확히 5목이면 무조건 승리 - 금수 규칙보다 우선
	if run == winLength {
		return false
	}

	if run > winLength {
		return cfg.ForbidOverline
	}

	if cfg.ForbidDoubleFour && countFours(b, y, x, stone) >= 2 {
		return true
	}
	if cfg.ForbidDoubleThree && countOpenThrees(b, y, x, stone) >= 2 {
		return true
	}

	return false
}

// ============================================================
// 평가 함수
// ============================================================

func Evaluate(b *Board, player Stone) int {
	return evaluateForStone(b, player) - evaluateForStone(b, opponent(player))*12/10
}

func evaluateForStone(b *Board, s Stone) int {
	total := 0
	for _, d := range directions {
		total += evaluateDirection(b, s, d[0], d[1])
	}
	return total
}

func evaluateDirection(b *Board, s Stone, dx, dy int) int {
	total := 0
	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			if b.Get(y, x) != s {
				continue
			}
			py, px := y-dy, x-dx
			if inBounds(py, px) && b.Get(py, px) == s {
				continue // 이미 세어진 라인의 중간 -> 스킵
			}
			length := 0
			cy, cx := y, x
			for inBounds(cy, cx) && b.Get(cy, cx) == s {
				length++
				cy += dy
				cx += dx
			}
			openStart := inBounds(py, px) && b.Get(py, px) == Empty
			openEnd := inBounds(cy, cx) && b.Get(cy, cx) == Empty
			total += patternScore(length, openStart, openEnd)
		}
	}
	return total
}

func patternScore(length int, openStart, openEnd bool) int {
	openCount := 0
	if openStart {
		openCount++
	}
	if openEnd {
		openCount++
	}
	switch {
	case length >= 5:
		return 1000000
	case length == 4:
		if openCount == 2 {
			return 100000
		} else if openCount == 1 {
			return 10000
		}
		return 0
	case length == 3:
		if openCount == 2 {
			return 1000
		} else if openCount == 1 {
			return 100
		}
		return 0
	case length == 2:
		if openCount == 2 {
			return 100
		} else if openCount == 1 {
			return 10
		}
		return 0
	case length == 1:
		return 1
	default:
		return 0
	}
}

// ============================================================
// 후보 수 생성
// ============================================================

// GenerateMoves: 기존 돌 주변 radius칸 이내의 빈칸만 후보로 반환 (탐색 범위 축소)
func GenerateMoves(b *Board, radius int) [][2]int {
	moves := make(map[[2]int]bool)
	hasStone := false

	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			if b.Get(y, x) == Empty {
				continue
			}
			hasStone = true
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					ny, nx := y+dy, x+dx
					if inBounds(ny, nx) && b.Get(ny, nx) == Empty {
						moves[[2]int{ny, nx}] = true
					}
				}
			}
		}
	}

	if !hasStone {
		return [][2]int{{boardSize / 2, boardSize / 2}}
	}

	result := make([][2]int, 0, len(moves))
	for m := range moves {
		result = append(result, m)
	}
	return result
}

// ============================================================
// 엔진: 미니맥스 + 알파베타 (negamax 형태)
// ============================================================

const winScore = 1_000_000_000

type Engine struct {
	Config   RenjuConfig
	MaxDepth int
	Radius   int // 후보 수 생성 반경
}

func NewEngine(cfg RenjuConfig, maxDepth int) *Engine {
	return &Engine{Config: cfg, MaxDepth: maxDepth, Radius: 5}
}

func (e *Engine) isIllegal(b *Board, y, x int, player Stone) bool {
	if b.Get(y, x) != Empty {
		return true
	}
	return IsForbidden(b, y, x, player, e.Config)
}

// FindBestMove: 반복 심화(iterative deepening)로 depth 1부터 MaxDepth까지 탐색.
func (e *Engine) FindBestMove(b *Board, player Stone) (int, int, int) {
	bestY, bestX := -1, -1
	bestScore := math.MinInt64

	for depth := 1; depth <= e.MaxDepth; depth++ {
		y, x, score := e.searchRoot(b, player, depth)
		if y != -1 {
			bestY, bestX, bestScore = y, x, score
		}
		if bestScore >= winScore {
			break // 이미 강제 승리를 찾았으면 더 깊이 볼 필요 없음
		}
	}
	return bestY, bestX, bestScore
}

func (e *Engine) searchRoot(b *Board, player Stone, depth int) (int, int, int) {
	moves := GenerateMoves(b, e.Radius)
	moves = e.orderMoves(b, moves, player)

	bestY, bestX := -1, -1
	best := math.MinInt64
	alpha, beta := math.MinInt64, math.MaxInt64

	for _, m := range moves {
		y, x := m[0], m[1]
		if e.isIllegal(b, y, x, player) {
			continue
		}

		b.Set(y, x, player)
		var score int
		if CheckWinAt(b, y, x, player) {
			score = winScore
		} else {
			score = -e.alphabeta(b, depth-1, -beta, -alpha, opponent(player))
		}
		b.Set(y, x, Empty)

		if score > best {
			best = score
			bestY, bestX = y, x
		}
		if score > alpha {
			alpha = score
		}
	}
	return bestY, bestX, best
}

// alphabeta: negamax 형태. 반환값은 항상 "지금 둘 차례인 player" 관점의 점수.
func (e *Engine) alphabeta(b *Board, depth int, alpha, beta int, player Stone) int {
	if depth == 0 {
		return Evaluate(b, player)
	}

	moves := GenerateMoves(b, e.Radius)
	moves = e.orderMoves(b, moves, player)

	best := math.MinInt64
	movesTried := 0

	for _, m := range moves {
		y, x := m[0], m[1]
		if e.isIllegal(b, y, x, player) {
			continue
		}
		movesTried++

		b.Set(y, x, player)
		var score int
		if CheckWinAt(b, y, x, player) {
			score = winScore
		} else {
			score = -e.alphabeta(b, depth-1, -beta, -alpha, opponent(player))
		}
		b.Set(y, x, Empty)

		if score > best {
			best = score
		}
		if best > alpha {
			alpha = best
		}
		if alpha >= beta {
			break // cut-off
		}
	}

	if movesTried == 0 {
		// 둘 수 있는 합법 수가 없음 (렌주 금수로 다 막힌 극단적 경우 등)
		return Evaluate(b, player)
	}
	return best
}

// orderMoves: 각 후보 수를 놓아봤을 때의 즉석 점수(연속 길이)로 정렬해서
// 알파베타 가지치기 효율을 높임.
func (e *Engine) orderMoves(b *Board, moves [][2]int, player Stone) [][2]int {
	type scored struct {
		move  [2]int
		score int
	}
	list := make([]scored, 0, len(moves))
	for _, m := range moves {
		b.Set(m[0], m[1], player)
		s := maxRunLength(b, m[0], m[1], player)
		b.Set(m[0], m[1], Empty)
		list = append(list, scored{m, s})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })

	result := make([][2]int, len(list))
	for i, s := range list {
		result[i] = s.move
	}
	return result
}

// ============================================================
// 데모
// ============================================================

func printBoard(b *Board) {
	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			switch b.Get(y, x) {
			case Black:
				fmt.Print("● ")
			case White:
				fmt.Print("○ ")
			default:
				fmt.Print(". ")
			}
		}
		fmt.Println()
	}
}

// ============================================================
// HTTP API (SolidJS 프론트엔드용)
// ============================================================

// RenjuConfigDTO: 요청에서 렌주 규칙을 선택적으로 오버라이드할 때 사용.
// 필드를 생략하면(=nil) 서버 기본값(DefaultRenjuConfig)을 사용합니다.
type RenjuConfigDTO struct {
	Enabled           bool `json:"enabled"`
	ForbidDoubleThree bool `json:"forbidDoubleThree"`
	ForbidDoubleFour  bool `json:"forbidDoubleFour"`
	ForbidOverline    bool `json:"forbidOverline"`
}

// BestMoveRequest: SolidJS 쪽에서 보내는 요청 바디.
// board: 15x15 2차원 배열, 0=Empty, 1=Black, 2=White (Stone enum과 값이 동일)
// player: 최선의 수를 찾을 대상 색상 (1=Black, 2=White)
type BestMoveRequest struct {
	Board  [boardSize][boardSize]int `json:"board"`
	Player int                       `json:"player"`
	Depth  int                       `json:"depth,omitempty"` // 생략 시 서버 기본값 사용
	Renju  *RenjuConfigDTO           `json:"renju,omitempty"` // 생략 시 서버 기본값 사용
}

type BestMoveResponse struct {
	Y      int    `json:"y"`
	X      int    `json:"x"`
	Score  int    `json:"score"`
	NoMove bool   `json:"noMove"` // true면 둘 수 있는 합법 수가 없음
	Error  string `json:"error,omitempty"`
}

const defaultSearchDepth = 4

func requestToBoard(req *BestMoveRequest) (*Board, error) {
	b := NewBoard()
	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			v := req.Board[y][x]
			if v != int(Empty) && v != int(Black) && v != int(White) {
				return nil, fmt.Errorf("board[%d][%d] 값이 올바르지 않습니다: %d (0=빈칸,1=흑,2=백만 허용)", y, x, v)
			}
			b.Set(y, x, Stone(v))
		}
	}
	return b, nil
}

// withCORS: SolidJS 개발 서버(다른 포트)에서의 fetch를 허용.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(BestMoveResponse{Error: msg})
}

// handleBestMove: POST /api/best-move
// 요청 예시:
//
//	{
//	  "board": [[0,0,...], ...],  // 15x15
//	  "player": 1,                // 1=Black, 2=White
//	  "depth": 4,                 // 선택
//	  "renju": {                  // 선택 (생략하면 기본 렌주룰 적용)
//	    "enabled": true,
//	    "forbidDoubleThree": true,
//	    "forbidDoubleFour": true,
//	    "forbidOverline": true
//	  }
//	}
func handleBestMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST만 허용됩니다")
		return
	}

	var req BestMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "요청 JSON 파싱 실패: "+err.Error())
		return
	}

	if req.Player != int(Black) && req.Player != int(White) {
		writeJSONError(w, http.StatusBadRequest, "player는 1(Black) 또는 2(White)여야 합니다")
		return
	}

	board, err := requestToBoard(&req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg := DefaultRenjuConfig()
	if req.Renju != nil {
		cfg = RenjuConfig{
			Enabled:           req.Renju.Enabled,
			ForbidDoubleThree: req.Renju.ForbidDoubleThree,
			ForbidDoubleFour:  req.Renju.ForbidDoubleFour,
			ForbidOverline:    req.Renju.ForbidOverline,
		}
	}

	depth := defaultSearchDepth
	if req.Depth > 0 {
		depth = req.Depth
	}

	engine := NewEngine(cfg, depth)
	y, x, score := engine.FindBestMove(board, Stone(req.Player))

	fmt.Print(score, "\n")

	w.Header().Set("Content-Type", "application/json")

	if y == -1 {
		// 합법적으로 둘 수 있는 곳이 없는 극단적인 경우
		json.NewEncoder(w).Encode(BestMoveResponse{NoMove: true})
		return
	}

	json.NewEncoder(w).Encode(BestMoveResponse{Y: y, X: x, Score: score})
}

func main() {
	port := 8090

	http.HandleFunc("/api/best-move", withCORS(handleBestMove))

	addr := fmt.Sprintf(":%d", port)
	fmt.Println("Server is running on port " + addr + "...")
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to run server: %v\n", err)
	}
}
