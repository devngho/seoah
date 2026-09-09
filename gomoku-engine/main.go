package main

import (
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
//
// 성능 노트: 예전 lineToString은 strings.Builder로 매번 힙에 문자열을
// 새로 할당했다. IsForbidden은 Black 착수 후보마다(렌주룰 활성화 시)
// 4방향씩 이 함수를 호출하므로 탐색 중 GC 압박이 컸다. 크기가 고정(9)이므로
// 스택에 올라가는 배열로 바꿔 할당을 없앤다.
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
// 연속 3("_SSS_")뿐 아니라 띈3("_SS_S_", "_S_SS_")도 함께 감지한다.
// (완전한 렌주룰의 "살아있는 3" 판정보다는 단순화된 근사치입니다)
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
		// 띈3: _SS_S_ / _S_SS_ (길이 6짜리 윈도우)
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
// 5칸 윈도우 안에 S가 4개, 빈칸이 1개면 (한 수로 5목 완성 가능) 4로 간주.
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

// Evaluate: player 관점의 (내 점수 - 상대 점수).
//
// 성능 노트: 예전에는 evaluateForStone을 Black/White에 대해 각각 호출했고,
// 그 안에서 4방향마다 보드 225칸을 전부 스캔했다 (총 2색 x 4방향 = 8회
// 전체 보드 스캔). 이 함수는 이 트리에서 depth==0인 모든 리프에서 호출되므로
// 탐색 성능에 가장 직접적인 영향을 준다. 여기서는 방향당 한 번의 스캔에서
// Black/White 점수를 동시에 누적해서 스캔 횟수를 4회로 절반 줄인다.
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
//
// 성능 노트: 예전에는 map[[2]int]bool로 중복을 제거했는데, 이 함수가
// 알파베타 탐색의 "모든 노드"에서 호출되다 보니(리프뿐 아니라 중간 노드까지)
// map 해싱/할당 비용이 그대로 누적되어 가장 큰 병목이었다. 여기서는 Engine이
// 들고 있는 고정 크기 bool 배열(visitedBuf)을 재사용해서 할당 없이 O(1)
// 중복 체크를 한다. 싱글스레드로 순차 탐색하는 현재 구조에서는 버퍼를
// 공유해도 안전하다(동시에 두 곳에서 쓰지 않음).
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

// posInf/negInf: math.MaxInt64/MinInt64 대신 사용하는 안전한 무한대 대용값.
// MinInt64는 절댓값이 MaxInt64보다 1 커서 -MinInt64 계산 시 오버플로우가 나므로
// (Go는 정수 오버플로우를 감싸서 값이 조용히 깨짐), 서로 부호만 바꿔도 안전한
// 대칭적인 큰 값을 사용한다. winScore(1e9)보다 훨씬 크면서 오버플로우 없이
// 음수로 뒤집을 수 있는 범위 안에 있다.
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
}

// SoftmaxTopPConfig: 최선 수 하나만 고르는 대신 확률적으로 수를 선택할 때 쓰는 설정.
type SoftmaxTopPConfig struct {
	Enabled     bool    // false면 기존처럼 argmax(최고 점수)로만 선택
	Temperature float64 // softmax 온도. 1.0이 기본, 작을수록 최고점 수에 더 쏠림
	TopP        float64 // 0 < TopP <= 1. 확률 높은 수부터 누적해 p가 될 때까지만 후보로 남김
	Seed        int64   // 난수 시드 (하드코딩해서 재현 가능하게 사용)
}

// SetSoftmaxTopP: softmax + top-p 샘플링 설정을 적용하고, Seed로 고정된 rng를 준비한다.
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
	return e
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

	for depth := 1; depth <= e.MaxDepth; depth++ {
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
			break // 이미 강제 승리를 찾았으면 더 깊이 볼 필요 없음
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

// moveResult: 루트의 한 후보 수와 그 수를 뒀을 때의 평가 점수.
type moveResult struct {
	y, x, score int
}

// evaluateRootMoves: 루트의 합법 후보 수 전부에 대해 (병렬로) 점수를 계산해서
// 리스트로 반환한다. searchRootParallel(argmax 선택)과 searchRootSoftmaxTopP
// (확률적 선택) 둘 다 이 함수를 공유해서 쓴다.
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
			workerEngine := NewEngine(e.Config, e.MaxDepth)
			workerEngine.Radius = e.Radius

			for m := range jobs {
				y, x := m[0], m[1]

				boardCopy := *b // Board가 고정 배열이라 값 복사됨(각자 독립된 보드)
				boardCopy.Set(y, x, player)

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
//
// 주의할 점 두 가지:
//
//  1. Engine의 GenerateMoves/orderMoves 버퍼(visitedBuf, movesBufs, scoreBufs)는
//     "싱글스레드 순차 탐색"을 전제로 depth 인덱싱만으로 안전하게 재사용하도록
//     만들어져 있다. 여러 goroutine이 같은 *Engine을 동시에 쓰면 이 버퍼들에서
//     데이터 레이스가 난다. 그래서 워커(고루틴)마다 자신만의 *Engine(=자신만의
//     버퍼)을 새로 만들어 쓴다. Engine 생성 자체는 슬라이스 몇 개 할당하는
//     가벼운 작업이라 워커 수(보통 CPU 코어 수)만큼만 만들면 비용이 크지 않다.
//
//  2. 순수 순차 알파베타는 형제 노드끼리 alpha를 갱신하며 가지치기 효율이
//     좋아지는데, 동시에 도는 goroutine들은 그 시점의 alpha를 실시간으로
//     공유하지 못한다. 완전히 무시하면 가지치기가 거의 안 되므로, 대신
//     "이미 완료된 형제 수의 점수 중 최댓값"을 atomic 변수로 공유해서,
//     새로 시작하는 워커가 그 값을 초기 alpha로 사용하게 한다. 이미 실제로
//     달성 가능한 것으로 확인된 점수이므로 최적성을 해치지 않는 안전한
//     하한선이며, 동시성 오버헤드도 거의 없다(락 없는 CAS 루프).
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
	if depth == 0 {
		return Evaluate(b, player)
	}

	moves := e.GenerateMoves(b, depth)
	moves = e.orderMoves(b, moves, player, depth, false)

	best := negInf
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
//
// 성능 노트: 예전에는 (a) 매 노드마다 scored 슬라이스를 새로 make()하고,
// (b) sort.Slice(리플렉션 기반)로 정렬한 뒤, (c) 결과를 담을 슬라이스를
// 또 make()해서 복사했다. 노드 수가 많은 알파베타에서 이 세 번의 할당이
// 누적되어 GC 부담이 컸다. 여기서는 depth별로 재사용하는 점수 버퍼만 새로
// 채우고, moves 슬라이스(=e.GenerateMoves가 depth별 버퍼에서 반환한 것)를
// moveScoreOrder를 통해 제자리에서(in-place) 정렬한다. 추가 할당 없음.
// isRoot: true면 이 정렬이 루트(현재 depth 반복의 첫 수 선택) 국면에서
// 호출된 것으로 간주해, iterative deepening의 직전 depth가 찾은 PV 수를
// 최우선 순위로 끌어올린다. 내부(재귀) 노드에서는 pvMove가 그 국면과
// 무관한 좌표일 수 있으므로 적용하지 않는다.
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

const defaultSearchDepth = 6

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
	engine.SetSoftmaxTopP(SoftmaxTopPConfig{
		Enabled:     true,
		Temperature: 0.5,
		TopP:        0.9,
		Seed:        42,
	})
	y, x, score := engine.FindBestMove(board, Stone(req.Player))

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
