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
