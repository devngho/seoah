package engine

import (
	"sync"

	"gomoku-engine/internal/config"
)

const defaultTranspositionTableSize = 1 << 18 // 262,144 entries

// ttBound는 저장된 점수가 정확값인지, 하한인지, 상한인지를 나타낸다.
type ttBound uint8

const (
	ttExact ttBound = iota
	ttLowerBound
	ttUpperBound
)

type ttEntry struct {
	key   uint64
	score int32
	depth int16
	move  uint16 // 0은 수 없음, 그 외에는 y*BoardSize+x+1
	bound ttBound
	valid bool
}

// TranspositionTable은 직접 사상(direct-mapped) 전치표다.
// 여러 루트 탐색 goroutine이 공유하므로, 락을 분산해 데이터 레이스를 막는다.
type TranspositionTable struct {
	entries []ttEntry
	mask    uint64
	locks   [256]sync.Mutex
}

func NewTranspositionTable(size int) *TranspositionTable {
	if size <= 0 {
		size = defaultTranspositionTableSize
	}

	// 인덱스 계산에 비트 마스크를 쓸 수 있도록 크기를 2의 거듭제곱으로 맞춘다.
	capacity := 1
	for capacity < size {
		capacity <<= 1
	}

	return &TranspositionTable{
		entries: make([]ttEntry, capacity),
		mask:    uint64(capacity - 1),
	}
}

func (tt *TranspositionTable) index(key uint64) int {
	return int(key & tt.mask)
}

func (tt *TranspositionTable) Probe(key uint64) (ttEntry, bool) {
	if tt == nil || len(tt.entries) == 0 {
		return ttEntry{}, false
	}

	idx := tt.index(key)
	lock := &tt.locks[idx%len(tt.locks)]
	lock.Lock()
	entry := tt.entries[idx]
	lock.Unlock()

	if !entry.valid || entry.key != key {
		return ttEntry{}, false
	}
	return entry, true
}

func (tt *TranspositionTable) Store(key uint64, depth, score int, bound ttBound, move uint16) {
	if tt == nil || len(tt.entries) == 0 {
		return
	}

	idx := tt.index(key)
	lock := &tt.locks[idx%len(tt.locks)]
	lock.Lock()
	old := tt.entries[idx]

	// 기존 항목이 없거나 새 항목을 같거나 더 깊게 탐색했다면 교체한다.
	// 따라서 같은 상태의 깊은 결과는 얕은 결과에 의해 덮어써지지 않는다.
	if !old.valid || depth >= int(old.depth) {
		if depth > int(^uint16(0)>>1) {
			depth = int(^uint16(0) >> 1)
		}
		if depth < 0 {
			depth = 0
		}
		tt.entries[idx] = ttEntry{
			key:   key,
			score: int32(score),
			depth: int16(depth),
			move:  move,
			bound: bound,
			valid: true,
		}
	}
	lock.Unlock()
}

func encodeMove(y, x int) uint16 {
	if y < 0 || x < 0 || y >= config.BoardSize || x >= config.BoardSize {
		return 0
	}
	return uint16(y*config.BoardSize + x + 1)
}

func decodeMove(move uint16) ([2]int, bool) {
	if move == 0 {
		return [2]int{}, false
	}
	v := int(move - 1)
	return [2]int{v / config.BoardSize, v % config.BoardSize}, true
}
