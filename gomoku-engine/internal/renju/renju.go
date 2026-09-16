package renju

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
