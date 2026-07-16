package collector

import (
	"regexp"
	"strconv"
)

var rusageMemPattern = regexp.MustCompile(`(?i)\brusage\s*\[[^\]]*\bmem\s*=\s*([0-9]+(?:\.[0-9]+)?)`)

func requestedMemoryKB(resourceReq string) int64 {
	match := rusageMemPattern.FindStringSubmatch(resourceReq)
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value <= 0 {
		return 0
	}
	return int64(value * 1024)
}
