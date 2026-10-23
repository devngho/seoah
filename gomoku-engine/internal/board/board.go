package board

import (
	"fmt"
	"gomoku-engine/internal/config"
)

type Stone int

const (
	Empty Stone = iota
	Black
	White
)

type Board struct {
	cells [config.BoardSize][config.BoardSize]Stone
	// hash는 현재 돌 배치의 Zobrist 해시다. Set에서 증분 갱신하므로
	// 탐색 노드마다 보드 전체를 다시 해싱할 필요가 없다.
	hash uint64
}

var zobrist [config.BoardSize][config.BoardSize][3]uint64
var zobristSideToMove uint64

// 결정적인 난수열을 사용해 실행마다 같은 Zobrist 키를 만든다.
// 전치표는 엔진 실행 중 같은 보드 상태를 빠르게 식별하기 위한 용도다.
func init() {
	state := uint64(0x9E3779B97F4A7C15)
	for y := 0; y < config.BoardSize; y++ {
		for x := 0; x < config.BoardSize; x++ {
			for s := Black; s <= White; s++ {
				zobrist[y][x][s] = splitMix64(&state)
			}
		}
	}
	zobristSideToMove = splitMix64(&state)
}

func splitMix64(state *uint64) uint64 {
	*state += 0x9E3779B97F4A7C15
	z := *state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func NewBoard() *Board {
	return &Board{}
}

func (b *Board) Get(y, x int) Stone {
	return b.cells[y][x]
}

func (b *Board) Set(y, x int, s Stone) {
	old := b.cells[y][x]
	if old == s {
		return
	}
	if old != Empty {
		b.hash ^= zobrist[y][x][old]
	}
	if s != Empty {
		b.hash ^= zobrist[y][x][s]
	}
	b.cells[y][x] = s
}

// Hash는 현재 돌 배치의 해시를 반환한다. 차례는 포함하지 않는다.
func (b *Board) Hash() uint64 {
	return b.hash
}

// HashForTurn은 현재 돌 배치와 둘 차례를 함께 식별하는 해시를 반환한다.
// 같은 돌 배치라도 흑 차례와 백 차례는 서로 다른 탐색 상태다.
func (b *Board) HashForTurn(toMove Stone) uint64 {
	if toMove == White {
		return b.hash ^ zobristSideToMove
	}
	return b.hash
}

func InBounds(y, x int) bool {
	return y >= 0 && y < config.BoardSize && x >= 0 && x < config.BoardSize
}

func Opponent(s Stone) Stone {
	if s == Black {
		return White
	}
	return Black
}

var Directions = [4][2]int{
	{1, 0},
	{0, 1},
	{1, 1},
	{1, -1},
}

func printBoard(b *Board) {
	for y := 0; y < config.BoardSize; y++ {
		for x := 0; x < config.BoardSize; x++ {
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
