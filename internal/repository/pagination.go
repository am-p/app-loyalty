package repository

import (
	"math"
)

func totalPages(total int64, size int) int64 {
	if total == 0 {
		return 0
	}
	return int64(math.Ceil(float64(total) / float64(size)))
}

func itoa(v int) string {
	if v < 10 {
		return string(rune('0' + v))
	}
	return fmtInt(v)
}

func fmtInt(v int) string {
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}
