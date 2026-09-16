package config

const BoardSize = 15
const WinLength = 5

const DefaultSearchDepth = 6
const DefaultMaxNodes = 20000000

const LineWindowRadius = 4
const LineWindowSize = LineWindowRadius*2 + 1 // 9

const WinScore = 1_000_000_000

const (
	PosInf = 1 << 62
	NegInf = -PosInf
)
