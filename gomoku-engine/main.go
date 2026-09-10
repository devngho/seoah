package main

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
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

var directions = [4][2]int{
	{1, 0},
	{0, 1},
	{1, 1},
	{1, -1},
}

// ============================================================
// Renju 금수 규칙 설정 (on/off 토글)
// ============================================================

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

// lineWindow: 렌주 금수 판정용 9칸(radius=4 고정) 윈도우.
// 'S' = 해당 stone, '_' = 빈칸, 'o' = 상대 돌 또는 보드 밖(막힘)
const lineWindowRadius = 4
const lineWindowSize = lineWindowRadius*2 + 1 // 9

func lineWindow(b *Board, y, x, dx, dy int, stone Stone) [lineWindowSize]byte {
	var win [lineWindowSize]byte
	for i := -lineWindowRadius; i <= lineWindowRadius; i++ {
		cy, cx := y+dy*i, x+dx*i
		idx := i + lineWindowRadius
		if !inBounds(cy, cx) {
			win[idx] = 'o'
			continue
		}
		switch b.Get(cy, cx) {
		case stone:
			win[idx] = 'S'
		case Empty:
			win[idx] = '_'
		default:
			win[idx] = 'o'
		}
	}
	return win
}

// countOpenThrees: (y,x)에 stone을 놓았을 때 만들어지는 "열린 3" 개수.
func countOpenThrees(b *Board, y, x int, stone Stone) int {
	count := 0
	for _, d := range directions {
		win := lineWindow(b, y, x, d[0], d[1], stone)

		found := false
		for i := 0; i+5 <= lineWindowSize; i++ {
			if win[i] == '_' && win[i+1] == 'S' && win[i+2] == 'S' && win[i+3] == 'S' && win[i+4] == '_' {
				found = true
				break
			}
		}
		if !found {
			for i := 0; i+6 <= lineWindowSize; i++ {
				if win[i] == '_' && win[i+1] == 'S' && win[i+2] == 'S' && win[i+3] == '_' && win[i+4] == 'S' && win[i+5] == '_' {
					found = true
					break
				}
				if win[i] == '_' && win[i+1] == 'S' && win[i+2] == '_' && win[i+3] == 'S' && win[i+4] == 'S' && win[i+5] == '_' {
					found = true
					break
				}
			}
		}
		if found {
			count++
		}
	}
	return count
}

// countFours: (y,x)에 stone을 놓았을 때 만들어지는 "4" 위협 개수.
func countFours(b *Board, y, x int, stone Stone) int {
	count := 0
	for _, d := range directions {
		win := lineWindow(b, y, x, d[0], d[1], stone)
		found := false
		for i := 0; i+5 <= lineWindowSize; i++ {
			sCount, emptyCount := 0, 0
			for j := i; j < i+5; j++ {
				switch win[j] {
				case 'S':
					sCount++
				case '_':
					emptyCount++
				}
			}
			if sCount == 4 && emptyCount == 1 {
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

// Evaluate: player 관점의 (내 점수 - 상대 점수).
func Evaluate(b *Board, player Stone) int {
	var blackScore, whiteScore int
	for _, d := range directions {
		bs, ws := evaluateDirectionBoth(b, d[0], d[1])
		blackScore += bs
		whiteScore += ws
	}
	if player == Black {
		return blackScore - whiteScore
	}
	return whiteScore - blackScore
}

func evaluateDirectionBoth(b *Board, dx, dy int) (blackTotal, whiteTotal int) {
	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			s := b.Get(y, x)
			if s == Empty {
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
			score := patternScore(length, openStart, openEnd)
			if s == Black {
				blackTotal += score
			} else {
				whiteTotal += score
			}
		}
	}
	return blackTotal, whiteTotal
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
func (e *Engine) GenerateMoves(b *Board, depth int) [][2]int {
	radius := e.Radius
	buf := &e.visitedBuf
	for i := range buf {
		buf[i] = [boardSize]bool{}
	}

	hasStone := false
	result := e.movesBufs[depth][:0]

	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			if b.Get(y, x) == Empty {
				continue
			}
			hasStone = true

			yLo, yHi := y-radius, y+radius
			if yLo < 0 {
				yLo = 0
			}
			if yHi >= boardSize {
				yHi = boardSize - 1
			}
			xLo, xHi := x-radius, x+radius
			if xLo < 0 {
				xLo = 0
			}
			if xHi >= boardSize {
				xHi = boardSize - 1
			}

			for ny := yLo; ny <= yHi; ny++ {
				for nx := xLo; nx <= xHi; nx++ {
					if b.Get(ny, nx) == Empty && !buf[ny][nx] {
						buf[ny][nx] = true
						result = append(result, [2]int{ny, nx})
					}
				}
			}
		}
	}

	if !hasStone {
		return [][2]int{{boardSize / 2, boardSize / 2}}
	}

	e.movesBufs[depth] = result
	return result
}

// ============================================================
// 엔진: 미니맥스 + 알파베타 (negamax 형태)
// ============================================================

const winScore = 1_000_000_000

const (
	posInf = 1 << 62
	negInf = -posInf
)

type Engine struct {
	Config   RenjuConfig
	MaxDepth int
	Radius   int // 후보 수 생성 반경

	// GenerateMoves/orderMoves가 노드마다 재할당/재해싱하지 않도록 재사용하는 버퍼.
	// alphabeta는 재귀 호출이므로 버퍼를 하나만 공유하면 자식 호출이 부모가
	// 아직 순회 중인 moves 슬라이스를 덮어써 버린다(같은 백킹 배열이므로).
	// 대신 재귀 "depth" 값으로 버퍼를 인덱싱한다: alphabeta(depth)는 항상
	// depth-1, depth-2, ...로만 재귀하므로 같은 depth 값이 콜스택에 동시에
	// 두 번 나타나지 않는다. 즉 buf[depth]는 depth 레벨의 노드가 사용하는
	// 동안 그 자식들(더 작은 depth)이 절대 건드리지 않아 안전하다.
	// 싱글스레드 순차 탐색 전제.
	visitedBuf [boardSize][boardSize]bool
	movesBufs  [][][2]int
	scoreBufs  [][]int

	// PV(Principal Variation) 수 우선 탐색:
	// iterative deepening에서 직전 depth가 찾은 최선 수를 다음 depth의
	// 후보 정렬에서 최우선으로 배치한다. 대개 한 수 깊어져도 최선 수는
	// 크게 바뀌지 않으므로, 이 수를 가장 먼저 탐색하면 알파-베타 알파값이
	// 빠르게 올라가서 이후 형제 수들의 컷오프 효율이 좋아진다.
	pvMove [2]int
	hasPV  bool

	// Softmax + top-p 샘플링: 활성화하면 마지막 depth의 루트 수 선택에서
	// 단순 argmax 대신, 각 후보 수의 평가치에 softmax를 적용해 확률적으로
	// 하나를 뽑는다. top-p(누적 확률 p)로 후보를 추리고, 그 안에서만
	// 확률에 따라 샘플링한다. rng는 Seed로 고정되어 재현 가능하다.
	Softmax SoftmaxTopPConfig
	rng     *rand.Rand

	// MaxNodes: 한 번의 FindBestMove(=API 요청 한 번) 동안 탐색할 노드 수 상한. (0 = 무제한)
	MaxNodes     int64
	nodesVisited *int64
}

type SoftmaxTopPConfig struct {
	Enabled     bool    // false면 기존처럼 argmax(최고 점수)로만 선택
	Temperature float64 // softmax 온도. 1.0이 기본, 작을수록 최고점 수에 더 쏠림
	TopP        float64 // 0 < TopP <= 1. 확률 높은 수부터 누적해 p가 될 때까지만 후보로 남김
	Seed        int64   // 난수 시드 (하드코딩해서 재현 가능하게 사용)
}

func (e *Engine) SetSoftmaxTopP(cfg SoftmaxTopPConfig) {
	e.Softmax = cfg
	e.rng = rand.New(rand.NewSource(cfg.Seed))
}

func NewEngine(cfg RenjuConfig, maxDepth int) *Engine {
	e := &Engine{
		Config:   cfg,
		MaxDepth: maxDepth,
		Radius:   2,
	}
	e.movesBufs = make([][][2]int, maxDepth+1)
	e.scoreBufs = make([][]int, maxDepth+1)
	for i := 0; i <= maxDepth; i++ {
		e.movesBufs[i] = make([][2]int, 0, boardSize*boardSize)
		e.scoreBufs[i] = make([]int, 0, boardSize*boardSize)
	}
	e.nodesVisited = new(int64)
	return e
}

func (e *Engine) budgetExceeded() bool {
	return e.MaxNodes > 0 && atomic.LoadInt64(e.nodesVisited) >= e.MaxNodes
}

func (e *Engine) countNode() {
	atomic.AddInt64(e.nodesVisited, 1)
}

func (e *Engine) NodesVisited() int64 {
	return atomic.LoadInt64(e.nodesVisited)
}

func (e *Engine) isIllegal(b *Board, y, x int, player Stone) bool {
	if b.Get(y, x) != Empty {
		return true
	}
	return IsForbidden(b, y, x, player, e.Config)
}

// FindBestMove: 반복 심화(iterative deepening)로 depth 1부터 MaxDepth까지 탐색.
// 각 depth의 루트 탐색은 searchRootParallel로 goroutine 병렬화되어 있다.
func (e *Engine) FindBestMove(b *Board, player Stone) (int, int, int) {
	start := time.Now()

	bestY, bestX := -1, -1
	bestScore := negInf

	// 매 FindBestMove 호출(=매 API 요청)은 서로 다른 국면일 수 있으므로,
	// 이전 호출에서 남은 PV 수를 그대로 들고 가면 안 된다. depth=1 탐색
	// 전에는 PV 수가 없는 상태로 시작한다.
	e.hasPV = false
	atomic.StoreInt64(e.nodesVisited, 0) // 이번 요청의 노드 카운트를 0부터 다시 시작

	for depth := 1; depth <= e.MaxDepth; depth++ {
		if e.budgetExceeded() {
			break // 이전 depth에서 이미 노드 예산을 다 썼으면 더 깊이 들어가지 않음
		}
		var y, x, score int
		if e.Softmax.Enabled && depth == e.MaxDepth {
			// 마지막 depth에서만 확률적 선택 사용. 그 이전 depth들은 PV(최선 수)를
			// 안정적으로 쌓아서 move ordering 품질을 유지하기 위해 argmax 그대로 둔다.
			y, x, score = e.searchRootSoftmaxTopP(b, player, depth)
		} else {
			y, x, score = e.searchRootParallel(b, player, depth)
		}
		if y != -1 {
			bestY, bestX, bestScore = y, x, score
			// 다음(depth+1) 반복에서 이 수를 최우선으로 탐색하도록 기록.
			e.pvMove = [2]int{y, x}
			e.hasPV = true
		}
		if bestScore >= winScore {
			break
		}
	}

	end := time.Since(start)
	fmt.Println(end)
	return bestY, bestX, bestScore
}

func (e *Engine) searchRoot(b *Board, player Stone, depth int) (int, int, int) {
	moves := e.GenerateMoves(b, depth)
	moves = e.orderMoves(b, moves, player, depth, true)

	bestY, bestX := -1, -1
	best := negInf
	alpha, beta := negInf, posInf

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

type moveResult struct {
	y, x, score int
}

// evaluateRootMoves: 루트의 합법 후보 수 전부에 대해 (병렬로) 점수를 계산해서
// 리스트로 반환한다. searchRootParallel(argmax 선택)과 searchRootSoftmaxTopP
func (e *Engine) evaluateRootMoves(b *Board, player Stone, depth int) []moveResult {
	moves := e.GenerateMoves(b, depth)
	moves = e.orderMoves(b, moves, player, depth, true)

	// 불법 수는 미리 걸러낸다. isIllegal은 b를 일시적으로 바꿨다가 되돌리는
	// 것 말고는 부수효과가 없고, 여기서는 아직 goroutine을 띄우기 전이라
	// 순차 실행이므로 메인 엔진(e)으로 처리해도 안전하다.
	legal := make([][2]int, 0, len(moves))
	for _, m := range moves {
		if !e.isIllegal(b, m[0], m[1], player) {
			legal = append(legal, m)
		}
	}
	if len(legal) == 0 {
		return nil
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > len(legal) {
		numWorkers = len(legal)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobs := make(chan [2]int, len(legal))
	for _, m := range legal {
		jobs <- m
	}
	close(jobs)

	results := make(chan moveResult, len(legal))
	sharedAlpha := int64(negInf)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 워커 전용 Engine: 부모(e)와 Config/Radius/MaxDepth는 같지만
			// 탐색 버퍼는 독립적이라 데이터 레이스 없이 안전하게 재귀할 수 있다.
			// MaxNodes/nodesVisited는 "요청 전체"에 대한 예산이므로 부모의
			// 공유 카운터 포인터를 그대로 물려받는다 (워커마다 따로 세면 안 됨).
			workerEngine := NewEngine(e.Config, e.MaxDepth)
			workerEngine.Radius = e.Radius
			workerEngine.MaxNodes = e.MaxNodes
			workerEngine.nodesVisited = e.nodesVisited

			for m := range jobs {
				if e.budgetExceeded() {
					// 예산 소진: 이 후보는 더 이상 평가하지 않는다. results에 아무것도
					// 안 보내면 evaluateRootMoves는 그냥 이 수를 "안 본 것"으로 취급하고
					// 이미 계산된 다른 후보들 중에서만 고른다.
					continue
				}

				y, x := m[0], m[1]

				boardCopy := *b // Board가 고정 배열이라 값 복사됨(각자 독립된 보드)
				boardCopy.Set(y, x, player)
				e.countNode()

				var score int
				if CheckWinAt(&boardCopy, y, x, player) {
					score = winScore
				} else {
					alpha := int(atomic.LoadInt64(&sharedAlpha))
					score = -workerEngine.alphabeta(&boardCopy, depth-1, negInf, -alpha, opponent(player))
				}

				results <- moveResult{y, x, score}

				// 다른 워커가 참고할 수 있도록 공유 alpha를 락 없이 갱신 (CAS 루프).
				for {
					cur := atomic.LoadInt64(&sharedAlpha)
					if int64(score) <= cur {
						break
					}
					if atomic.CompareAndSwapInt64(&sharedAlpha, cur, int64(score)) {
						break
					}
				}
			}
		}()
	}

	wg.Wait()
	close(results)

	all := make([]moveResult, 0, len(legal))
	for r := range results {
		all = append(all, r)
	}
	return all
}

// searchRootParallel: 루트 후보 수들을 병렬 평가한 뒤, 가장 점수가 높은 수를 고른다
// (argmax). searchRoot과 결과 자체는 같지만 goroutine으로 나눠서 계산한다.
func (e *Engine) searchRootParallel(b *Board, player Stone, depth int) (int, int, int) {
	results := e.evaluateRootMoves(b, player, depth)
	if len(results) == 0 {
		return -1, -1, negInf
	}

	bestY, bestX := -1, -1
	best := negInf
	for _, r := range results {
		if r.score > best {
			best = r.score
			bestY, bestX = r.y, r.x
		}
	}
	return bestY, bestX, best
}

// searchRootSoftmaxTopP: 루트 후보 수들을 병렬 평가하는 것까지는 searchRootParallel과
// 같지만, 가장 점수가 높은 수를 그대로 고르는 대신 softmax + top-p 샘플링으로
// 확률적으로 하나를 선택한다.
func (e *Engine) searchRootSoftmaxTopP(b *Board, player Stone, depth int) (int, int, int) {
	results := e.evaluateRootMoves(b, player, depth)
	if len(results) == 0 {
		return -1, -1, negInf
	}

	y, x, score := selectSoftmaxTopP(e.rng, results, e.Softmax.Temperature, e.Softmax.TopP)
	return y, x, score
}

// selectSoftmaxTopP: 후보 수들의 점수에 softmax를 적용해 확률분포를 만들고,
// 점수 높은 순으로 누적 확률이 topP에 도달할 때까지의 수들만 남겨 재정규화한 뒤
// (top-p / nucleus sampling), 그 안에서 rng로 하나를 뽑는다.
func selectSoftmaxTopP(rng *rand.Rand, results []moveResult, temperature, topP float64) (int, int, int) {
	n := len(results)

	// 점수 내림차순 정렬 (원본 순서를 바꾸지 않도록 복사본에서)
	sorted := make([]moveResult, n)
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].score > sorted[j].score })

	temp := temperature
	if temp <= 0 {
		temp = 1.0
	}
	maxScore := float64(sorted[0].score)

	// 오버플로우 방지를 위해 최댓값 기준으로 shift한 뒤 지수화 (안정적 softmax)
	weights := make([]float64, n)
	sum := 0.0
	for i, r := range sorted {
		shifted := (float64(r.score) - maxScore) / temp
		w := math.Exp(shifted)
		weights[i] = w
		sum += w
	}

	probs := make([]float64, n)
	for i, w := range weights {
		probs[i] = w / sum
	}

	// top-p: 확률 높은 것부터 누적해서 p에 도달하는 지점까지만 후보로 남김
	p := topP
	if p <= 0 || p > 1 {
		p = 1.0
	}
	cutoff := n
	cum := 0.0
	for i, pr := range probs {
		cum += pr
		if cum >= p {
			cutoff = i + 1
			break
		}
	}

	// 남은 후보들만 재정규화
	total := 0.0
	for i := 0; i < cutoff; i++ {
		total += probs[i]
	}

	r := rng.Float64() * total
	acc := 0.0
	chosen := cutoff - 1 // 부동소수점 오차로 못 걸리는 경우 마지막 후보를 fallback으로
	for i := 0; i < cutoff; i++ {
		acc += probs[i]
		if r <= acc {
			chosen = i
			break
		}
	}

	m := sorted[chosen]
	return m.y, m.x, m.score
}

// alphabeta: negamax 형태. 반환값은 항상 "지금 둘 차례인 player" 관점의 점수.
func (e *Engine) alphabeta(b *Board, depth int, alpha, beta int, player Stone) int {
	if depth == 0 || e.budgetExceeded() {
		// 노드 예산을 다 썼으면 더 안 내려가고 지금 국면을 정적 평가로 대신한다
		// (best-effort: 남은 depth는 포기하고 지금까지 본 것만으로 답한다).
		return Evaluate(b, player)
	}

	moves := e.GenerateMoves(b, depth)
	moves = e.orderMoves(b, moves, player, depth, false)

	best := negInf
	movesTried := 0

	for _, m := range moves {
		if e.budgetExceeded() {
			break // 예산 소진: 남은 형제 수들은 더 안 보고 지금까지의 best로 반환
		}
		y, x := m[0], m[1]
		if e.isIllegal(b, y, x, player) {
			continue
		}
		movesTried++
		e.countNode()

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
func (e *Engine) orderMoves(b *Board, moves [][2]int, player Stone, depth int, isRoot bool) [][2]int {
	scores := e.scoreBufs[depth][:0]
	for _, m := range moves {
		b.Set(m[0], m[1], player)
		s := maxRunLength(b, m[0], m[1], player)
		b.Set(m[0], m[1], Empty)
		if isRoot && e.hasPV && m == e.pvMove {
			s += 1_000_000 // PV 수는 무조건 최우선으로 탐색
		}
		scores = append(scores, s)
	}
	e.scoreBufs[depth] = scores

	sort.Sort(moveScoreOrder{moves: moves, scores: scores})
	return moves
}

// moveScoreOrder: sort.Slice(리플렉션 사용) 대신 sort.Sort로 정렬하기 위한
// 구체 타입. moves와 scores를 함께 스왑해서 별도 결과 슬라이스가 필요 없다.
type moveScoreOrder struct {
	moves  [][2]int
	scores []int
}

func (o moveScoreOrder) Len() int { return len(o.moves) }
func (o moveScoreOrder) Swap(i, j int) {
	o.moves[i], o.moves[j] = o.moves[j], o.moves[i]
	o.scores[i], o.scores[j] = o.scores[j], o.scores[i]
}
func (o moveScoreOrder) Less(i, j int) bool { return o.scores[i] > o.scores[j] } // 내림차순

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
// 게임 세션 (서버가 게임의 모든 상태를 관리)
// ============================================================

// isBoardFull: 보드에 빈 칸이 하나도 없으면 true (무승부 판정용)
func isBoardFull(b *Board) bool {
	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			if b.Get(y, x) == Empty {
				return false
			}
		}
	}
	return true
}

type GameStatus string

const (
	StatusPlaying GameStatus = "playing"
	StatusWin     GameStatus = "win"
	StatusDraw    GameStatus = "draw"
)

// MoveRecord: 기보(복기) 한 수. 웹은 게임이 끝나면 History를 통째로 들고 있다가
// 서버 통신 없이 로컬에서 복기를 재생한다.
type MoveRecord struct {
	Row   int   `json:"row"`
	Col   int   `json:"col"`
	Color Stone `json:"color"`
}

// GameSession: 게임 하나의 전체 상태. 이제 웹이 아니라 서버가 이 상태의
// 유일한 소유자다. 웹은 항상 이 상태의 스냅샷(GameStateDTO)만 받아서 그린다.
type GameSession struct {
	ID        string
	Board     *Board
	Turn      Stone
	Config    RenjuConfig
	Depth     int
	MaxNodes  int64
	History   []MoveRecord
	Status    GameStatus
	Winner    Stone // Status == StatusWin일 때만 의미 있음
	CreatedAt time.Time
}

// GameStateDTO: 클라이언트에 내려주는 게임 상태 스냅샷.
type GameStateDTO struct {
	GameID         string       `json:"gameId"`
	Board          [][]int      `json:"board"`
	Turn           int          `json:"turn"`
	Status         string       `json:"status"`
	Winner         int          `json:"winner,omitempty"`
	History        []MoveRecord `json:"history"`
	ForbiddenMoves [][2]int     `json:"forbiddenMoves"`
}

func boardToDTO(b *Board) [][]int {
	out := make([][]int, boardSize)
	for y := 0; y < boardSize; y++ {
		out[y] = make([]int, boardSize)
		for x := 0; x < boardSize; x++ {
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
	if g.Status == StatusPlaying && g.Turn == Black && g.Config.Enabled {
		for y := 0; y < boardSize; y++ {
			for x := 0; x < boardSize; x++ {
				if g.Board.Get(y, x) == Empty && IsForbidden(g.Board, y, x, Black, g.Config) {
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

// ============================================================
// 요청 큐: 서버가 관리하는 모든 게임에 대한 모든 요청을 전역으로 하나씩 처리한다.
//
// 왜 게임별이 아니라 전역 큐인가: 최선수 계산(alphabeta)이 이미 GOMAXPROCS만큼
// goroutine을 띄워 논리 프로세서를 전부 끌어다 쓰는 구조다(evaluateRootMoves 참고).
// 그래서 서로 다른 게임의 최선수 요청을 "동시에" 처리하게 두면 두 계산이 같은
// 코어들을 두고 경쟁만 하게 되어 오히려 둘 다 느려진다. 요청 종류/게임 ID에
// 상관없이 워커 하나가 순차적으로 처리하게 하면, 한 번에 하나의 계산만
// 논리 프로세서 전체를 온전히 쓸 수 있다.
//
// 클라이언트는 큐에 들어간 요청이 처리를 마칠 때까지 HTTP 응답을 기다린다
// (동기 응답). 워커가 하나뿐이므로 게임 상태(map, 각 GameSession) 자체에는
// 별도의 락이 필요 없다 - 오직 이 워커 goroutine만 상태를 읽고 쓴다.
// ============================================================

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
	renjuCfg *RenjuConfig
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

type GameManager struct {
	games map[string]*GameSession
	queue chan job
}

func NewGameManager() *GameManager {
	m := &GameManager{
		games: make(map[string]*GameSession),
		queue: make(chan job, 256),
	}
	go m.run()
	return m
}

func (m *GameManager) run() {
	for j := range m.queue {
		m.process(j)
	}
}

// submit: 요청을 큐에 넣고 워커가 처리를 마칠 때까지 블로킹으로 기다린다.
func (m *GameManager) submit(j job) jobResult {
	j.resp = make(chan jobResult, 1)
	m.queue <- j
	return <-j.resp
}

func newGameID() string {
	buf := make([]byte, 8)
	if _, err := cryptorand.Read(buf); err != nil {
		// crypto/rand 실패는 사실상 일어나지 않지만 방어적으로 시간 기반 fallback을 둔다.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (m *GameManager) process(j job) {
	switch j.kind {
	case jobCreateGame:
		m.handleCreateGame(j)
	case jobGetState:
		m.handleGetState(j)
	case jobPlaceMove:
		m.handlePlaceMove(j)
	case jobBestMove:
		m.handleBestMove(j)
	}
}

func (m *GameManager) handleCreateGame(j job) {
	cfg := DefaultRenjuConfig()
	if j.renjuCfg != nil {
		cfg = *j.renjuCfg
	}
	depth := defaultSearchDepth
	if j.depth > 0 {
		depth = j.depth
	}
	maxNodes := int64(defaultMaxNodes)
	if j.maxNodes > 0 {
		maxNodes = j.maxNodes
	}

	g := &GameSession{
		ID:        newGameID(),
		Board:     NewBoard(),
		Turn:      Black,
		Config:    cfg,
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

func (m *GameManager) handleGetState(j job) {
	g, err := m.lookup(j.gameID)
	if err != nil {
		j.resp <- jobResult{err: err}
		return
	}
	j.resp <- jobResult{state: g.toDTO()}
}

func (m *GameManager) handlePlaceMove(j job) {
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

func (m *GameManager) handleBestMove(j job) {
	g, err := m.lookup(j.gameID)
	if err != nil {
		j.resp <- jobResult{err: err}
		return
	}
	if g.Status != StatusPlaying {
		j.resp <- jobResult{err: fmt.Errorf("이미 종료된 게임입니다")}
		return
	}

	engine := NewEngine(g.Config, g.Depth)
	engine.SetSoftmaxTopP(SoftmaxTopPConfig{
		Enabled:     true,
		Temperature: 0.1,
		TopP:        0.9,
		Seed:        42,
	})
	engine.MaxNodes = g.MaxNodes
	y, x, score := engine.FindBestMove(g.Board, g.Turn)

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
func applyMove(g *GameSession, y, x int, color Stone) error {
	if g.Status != StatusPlaying {
		return fmt.Errorf("이미 종료된 게임입니다")
	}
	if !inBounds(y, x) {
		return fmt.Errorf("보드 범위를 벗어났습니다: (%d, %d)", y, x)
	}
	if g.Board.Get(y, x) != Empty {
		return fmt.Errorf("이미 돌이 있는 자리입니다: (%d, %d)", y, x)
	}
	if IsForbidden(g.Board, y, x, color, g.Config) {
		return fmt.Errorf("렌주 금수 자리입니다: (%d, %d)", y, x)
	}

	g.Board.Set(y, x, color)
	g.History = append(g.History, MoveRecord{Row: y, Col: x, Color: color})

	if CheckWinAt(g.Board, y, x, color) {
		g.Status = StatusWin
		g.Winner = color
		return nil
	}
	if isBoardFull(g.Board) {
		g.Status = StatusDraw
		return nil
	}
	g.Turn = opponent(color)
	return nil
}

// ============================================================
// HTTP API (SolidJS 프론트엔드용)
//
//   POST /api/games              게임 생성 (렌주룰 설정 포함) -> gameId + 초기 상태
//   GET  /api/games/{id}         현재 게임 상태 조회
//   POST /api/games/{id}/move    사람 플레이어 착수 { row, col }
//   POST /api/games/{id}/best-move  최선수 요청 { place: bool }
//                                 place=true  -> 서버가 실제로 두고 상태 반환
//                                 place=false -> 좌표만 추천, 상태는 그대로
// ============================================================

// RenjuConfigDTO: 게임 생성 요청에서 렌주 규칙을 선택적으로 오버라이드할 때 사용.
// 생략하면(=nil) 서버 기본값(DefaultRenjuConfig)을 사용합니다.
type RenjuConfigDTO struct {
	Enabled           bool `json:"enabled"`
	ForbidDoubleThree bool `json:"forbidDoubleThree"`
	ForbidDoubleFour  bool `json:"forbidDoubleFour"`
	ForbidOverline    bool `json:"forbidOverline"`
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

const defaultSearchDepth = 6
const defaultMaxNodes = 20000000

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorDTO{Error: msg})
}

// withCORS: SolidJS 개발 서버(다른 포트)에서의 fetch를 허용.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func handleCreateGame(m *GameManager) http.HandlerFunc {
	type reqBody struct {
		Renju    *RenjuConfigDTO `json:"renju,omitempty"`
		Depth    int             `json:"depth,omitempty"`
		MaxNodes int64           `json:"maxNodes,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body reqBody
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSONError(w, http.StatusBadRequest, "요청 JSON 파싱 실패: "+err.Error())
				return
			}
		}

		var cfg *RenjuConfig
		if body.Renju != nil {
			cfg = &RenjuConfig{
				Enabled:           body.Renju.Enabled,
				ForbidDoubleThree: body.Renju.ForbidDoubleThree,
				ForbidDoubleFour:  body.Renju.ForbidDoubleFour,
				ForbidOverline:    body.Renju.ForbidOverline,
			}
		}

		result := m.submit(job{kind: jobCreateGame, renjuCfg: cfg, depth: body.Depth, maxNodes: body.MaxNodes})
		writeJSON(w, http.StatusOK, result.state)
	}
}

func handleGetState(m *GameManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID := r.PathValue("id")
		result := m.submit(job{kind: jobGetState, gameID: gameID})
		if result.err != nil {
			writeJSONError(w, http.StatusNotFound, result.err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result.state)
	}
}

func handlePlaceMove(m *GameManager) http.HandlerFunc {
	type reqBody struct {
		Row int `json:"row"`
		Col int `json:"col"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		gameID := r.PathValue("id")
		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "요청 JSON 파싱 실패: "+err.Error())
			return
		}
		result := m.submit(job{kind: jobPlaceMove, gameID: gameID, row: body.Row, col: body.Col})
		if result.err != nil {
			writeJSONError(w, http.StatusBadRequest, result.err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result.state)
	}
}

func handleBestMove(m *GameManager) http.HandlerFunc {
	type reqBody struct {
		Place bool `json:"place"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		gameID := r.PathValue("id")
		var body reqBody
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSONError(w, http.StatusBadRequest, "요청 JSON 파싱 실패: "+err.Error())
				return
			}
		}
		result := m.submit(job{kind: jobBestMove, gameID: gameID, place: body.Place})
		if result.err != nil {
			writeJSONError(w, http.StatusBadRequest, result.err.Error())
			return
		}
		dto := BestMoveDTO{GameStateDTO: result.state}
		if result.hasBestMove {
			dto.Move = &MoveDTO{Y: result.bestY, X: result.bestX, Score: result.bestScore}
		}
		writeJSON(w, http.StatusOK, dto)
	}
}

func main() {
	port := 8090
	manager := NewGameManager()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/games", handleCreateGame(manager))
	mux.HandleFunc("GET /api/games/{id}", handleGetState(manager))
	mux.HandleFunc("POST /api/games/{id}/move", handlePlaceMove(manager))
	mux.HandleFunc("POST /api/games/{id}/best-move", handleBestMove(manager))

	addr := fmt.Sprintf(":%d", port)
	fmt.Println("Server is running on port " + addr + "...")
	if err := http.ListenAndServe(addr, withCORS(mux.ServeHTTP)); err != nil {
		log.Fatalf("Failed to run server: %v\n", err)
	}
}
