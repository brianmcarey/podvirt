package config

import (
	"fmt"
	"strconv"
	"strings"
)

var quantitySuffixes = []struct {
	suffix     string
	multiplier uint64
}{
	{"Ti", 1 << 40},
	{"Gi", 1 << 30},
	{"Mi", 1 << 20},
	{"Ki", 1 << 10},
	{"T", 1000 * 1000 * 1000 * 1000},
	{"G", 1000 * 1000 * 1000},
	{"M", 1000 * 1000},
	{"K", 1000},
}

func ParseQuantityBytes(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty quantity")
	}

	for _, item := range quantitySuffixes {
		if !strings.HasSuffix(s, item.suffix) {
			continue
		}
		numStr := strings.TrimSuffix(s, item.suffix)
		val, err := strconv.ParseUint(numStr, 10, 64)
		if err != nil || val == 0 {
			return 0, fmt.Errorf("invalid quantity %q", s)
		}
		return val * item.multiplier, nil
	}

	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil || val == 0 {
		return 0, fmt.Errorf("invalid quantity %q", s)
	}
	return val, nil
}
