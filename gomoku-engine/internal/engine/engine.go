package engine

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gomoku-engine/internal/board"
	"gomoku-engine/internal/config"
	"gomoku-engine/internal/renju"
)

// ============================================================
// 라인 분석 유틸
// ============================================================

// runLength: (y,x)에 stone이 이미 놓여있다고 가정하고, (dx,dy) 방향 양쪽으로
// 이어지는 연속된 길이를 반환합니다.
func runLength(b *board.Board, y, x, dx, dy int, stone board.Stone) int {
	length := 1
	cy, cx := y+dy, x+dx
	for board.InBounds(cy, cx) && b.Get(cy, cx) == stone {
		length++
		cy += dy
		cx += dx
	}
	cy, cx = y-dy, x-dx
	for board.InBounds(cy, cx) && b.Get(cy, cx) == stone {
		length++
		cy -= dy
		cx -= dx
	}
	return length
}

func maxRunLength(b *board.Board, y, x int, stone board.Stone) int {
	max := 0
	for _, d := range board.Directions {
		l := runLength(b, y, x, d[0], d[1], stone)
		if l > max {
			max = l
		}
	}
	return max
}

// CheckWinAt: (y,x)에 stone을 놓아 정확히 5목 이상을 완성했는지 확인 (승리 판정용)
func CheckWinAt(b *board.Board, y, x int, stone board.Stone) bool {
	for _, d := range board.Directions {
		if runLength(b, y, x, d[0], d[1], stone) >= config.WinLength {
			return true
		}
	}
	return false
}

func lineWindow(b *board.Board, y, x, dx, dy int, stone board.Stone) [config.LineWindowSize]byte {
	var win [config.LineWindowSize]byte
	for i := -config.LineWindowRadius; i <= config.LineWindowRadius; i++ {
		cy, cx := y+dy*i, x+dx*i
		idx := i + config.LineWindowRadius
		if !board.InBounds(cy, cx) {
			win[idx] = 'o'
			continue
		}
		switch b.Get(cy, cx) {
		case stone:
			win[idx] = 'S'
		case board.Empty:
			win[idx] = '_'
		default:
			win[idx] = 'o'
		}
	}
	return win
}

// countOpenThrees: (y,x)에 stone을 놓았을 때 만들어지는 "열린 3" 개수.
func countOpenThrees(b *board.Board, y, x int, stone board.Stone) int {
	count := 0
	for _, d := range board.Directions {
		win := lineWindow(b, y, x, d[0], d[1], stone)

		found := false
		for i := 0; i+5 <= config.LineWindowSize; i++ {
			if win[i] == '_' && win[i+1] == 'S' && win[i+2] == 'S' && win[i+3] == 'S' && win[i+4] == '_' {
				found = true
				break
			}
		}
		if !found {
			for i := 0; i+6 <= config.LineWindowSize; i++ {
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
func countFours(b *board.Board, y, x int, stone board.Stone) int {
	count := 0
	for _, d := range board.Directions {
		win := lineWindow(b, y, x, d[0], d[1], stone)
		found := false
		for i := 0; i+5 <= config.LineWindowSize; i++ {
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
func IsForbidden(b *board.Board, y, x int, stone board.Stone, renjuCfg renju.RenjuConfig) bool {
	if !renjuCfg.Enabled || stone != board.Black {
		return false
	}
	if b.Get(y, x) != board.Empty {
		return false
	}

	b.Set(y, x, stone)
	defer b.Set(y, x, board.Empty)

	run := maxRunLength(b, y, x, stone)

	if run == config.WinLength {
		return false
	}

	if run > config.WinLength {
		return renjuCfg.ForbidOverline
	}

	if renjuCfg.ForbidDoubleFour && countFours(b, y, x, stone) >= 2 {
		return true
	}
	if renjuCfg.ForbidDoubleThree && countOpenThrees(b, y, x, stone) >= 2 {
		return true
	}

	return false
}

// ============================================================
// 평가 함수
// ============================================================

// Evaluate: player 관점의 (내 점수 - 상대 점수).
func Evaluate(b *board.Board, player board.Stone) int {
	var blackScore, whiteScore int
	for _, d := range board.Directions {
		bs, ws := evaluateDirectionBoth(b, d[0], d[1])
		blackScore += bs
		whiteScore += ws
	}
	if player == board.Black {
		return blackScore - whiteScore
	}
	return whiteScore - blackScore
}

func evaluateDirectionBoth(b *board.Board, dx, dy int) (blackTotal, whiteTotal int) {
	for y := 0; y < config.BoardSize; y++ {
		for x := 0; x < config.BoardSize; x++ {
			s := b.Get(y, x)
			if s == board.Empty {
				continue
			}
			py, px := y-dy, x-dx
			if board.InBounds(py, px) && b.Get(py, px) == s {
				continue // 이미 세어진 라인의 중간 -> 스킵
			}
			length := 0
			cy, cx := y, x
			for board.InBounds(cy, cx) && b.Get(cy, cx) == s {
				length++
				cy += dy
				cx += dx
			}
			openStart := board.InBounds(py, px) && b.Get(py, px) == board.Empty
			openEnd := board.InBounds(cy, cx) && b.Get(cy, cx) == board.Empty
			score := patternScore(length, openStart, openEnd)
			if s == board.Black {
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
func (e *Engine) GenerateMoves(b *board.Board, depth int) [][2]int {
	radius := e.Radius
	buf := &e.visitedBuf
	for i := range buf {
		buf[i] = [config.BoardSize]bool{}
	}

	hasStone := false
	result := e.movesBufs[depth][:0]

	for y := 0; y < config.BoardSize; y++ {
		for x := 0; x < config.BoardSize; x++ {
			if b.Get(y, x) == board.Empty {
				continue
			}
			hasStone = true

			yLo, yHi := y-radius, y+radius
			if yLo < 0 {
				yLo = 0
			}
			if yHi >= config.BoardSize {
				yHi = config.BoardSize - 1
			}
			xLo, xHi := x-radius, x+radius
			if xLo < 0 {
				xLo = 0
			}
			if xHi >= config.BoardSize {
				xHi = config.BoardSize - 1
			}

			for ny := yLo; ny <= yHi; ny++ {
				for nx := xLo; nx <= xHi; nx++ {
					if b.Get(ny, nx) == board.Empty && !buf[ny][nx] {
						buf[ny][nx] = true
						result = append(result, [2]int{ny, nx})
					}
				}
			}
		}
	}

	if !hasStone {
		return [][2]int{{config.BoardSize / 2, config.BoardSize / 2}}
	}

	e.movesBufs[depth] = result
	return result
}

// ============================================================
// 엔진: 미니맥스 + 알파베타 (negamax 형태)
// ============================================================

type Engine struct {
	Config   renju.RenjuConfig
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
	visitedBuf [config.BoardSize][config.BoardSize]bool
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

func NewEngine(cfg renju.RenjuConfig, maxDepth int) *Engine {
	e := &Engine{
		Config:   cfg,
		MaxDepth: maxDepth,
		Radius:   2,
	}
	e.movesBufs = make([][][2]int, maxDepth+1)
	e.scoreBufs = make([][]int, maxDepth+1)
	for i := 0; i <= maxDepth; i++ {
		e.movesBufs[i] = make([][2]int, 0, config.BoardSize*config.BoardSize)
		e.scoreBufs[i] = make([]int, 0, config.BoardSize*config.BoardSize)
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

func (e *Engine) isIllegal(b *board.Board, y, x int, player board.Stone) bool {
	if b.Get(y, x) != board.Empty {
		return true
	}
	return IsForbidden(b, y, x, player, e.Config)
}

// FindBestMove: 반복 심화(iterative deepening)로 depth 1부터 MaxDepth까지 탐색.
// 각 depth의 루트 탐색은 searchRootParallel로 goroutine 병렬화되어 있다.
func (e *Engine) FindBestMove(b *board.Board, player board.Stone) (int, int, int) {
	start := time.Now()

	bestY, bestX := -1, -1
	bestScore := config.NegInf

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
		if bestScore >= config.WinScore {
			break
		}
	}

	end := time.Since(start)
	fmt.Println(end)
	return bestY, bestX, bestScore
}

func (e *Engine) searchRoot(b *board.Board, player board.Stone, depth int) (int, int, int) {
	moves := e.GenerateMoves(b, depth)
	moves = e.orderMoves(b, moves, player, depth, true)

	bestY, bestX := -1, -1
	best := config.NegInf
	alpha, beta := config.NegInf, config.PosInf

	for _, m := range moves {
		y, x := m[0], m[1]
		if e.isIllegal(b, y, x, player) {
			continue
		}

		b.Set(y, x, player)
		var score int
		if CheckWinAt(b, y, x, player) {
			score = config.WinScore
		} else {
			score = -e.alphabeta(b, depth-1, -beta, -alpha, board.Opponent(player))
		}
		b.Set(y, x, board.Empty)

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
func (e *Engine) evaluateRootMoves(b *board.Board, player board.Stone, depth int) []moveResult {
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
	sharedAlpha := int64(config.NegInf)

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
					score = config.WinScore
				} else {
					alpha := int(atomic.LoadInt64(&sharedAlpha))
					score = -workerEngine.alphabeta(&boardCopy, depth-1, config.NegInf, -alpha, board.Opponent(player))
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
func (e *Engine) searchRootParallel(b *board.Board, player board.Stone, depth int) (int, int, int) {
	results := e.evaluateRootMoves(b, player, depth)
	if len(results) == 0 {
		return -1, -1, config.NegInf
	}

	bestY, bestX := -1, -1
	best := config.NegInf
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
func (e *Engine) searchRootSoftmaxTopP(b *board.Board, player board.Stone, depth int) (int, int, int) {
	results := e.evaluateRootMoves(b, player, depth)
	if len(results) == 0 {
		return -1, -1, config.NegInf
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
func (e *Engine) alphabeta(b *board.Board, depth int, alpha, beta int, player board.Stone) int {
	if depth == 0 || e.budgetExceeded() {
		// 노드 예산을 다 썼으면 더 안 내려가고 지금 국면을 정적 평가로 대신한다
		// (best-effort: 남은 depth는 포기하고 지금까지 본 것만으로 답한다).
		return Evaluate(b, player)
	}

	moves := e.GenerateMoves(b, depth)
	moves = e.orderMoves(b, moves, player, depth, false)

	best := config.NegInf
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
			score = config.WinScore
		} else {
			score = -e.alphabeta(b, depth-1, -beta, -alpha, board.Opponent(player))
		}
		b.Set(y, x, board.Empty)

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
func (e *Engine) orderMoves(b *board.Board, moves [][2]int, player board.Stone, depth int, isRoot bool) [][2]int {
	scores := e.scoreBufs[depth][:0]
	for _, m := range moves {
		b.Set(m[0], m[1], player)
		s := maxRunLength(b, m[0], m[1], player)
		b.Set(m[0], m[1], board.Empty)
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
