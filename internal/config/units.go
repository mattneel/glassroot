package config

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var errInvalidUnit = errors.New("invalid unit")

func ParseSizeBytes(s string) (int64, error) {
	units := []struct {
		suffix string
		mult   int64
	}{
		{"TiB", 1024 * 1024 * 1024 * 1024},
		{"GiB", 1024 * 1024 * 1024},
		{"MiB", 1024 * 1024},
		{"KiB", 1024},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			return parsePositiveUnitNumber(strings.TrimSuffix(s, u.suffix), u.mult)
		}
	}
	return 0, errInvalidUnit
}

func ParseDurationMillis(s string) (int64, error) {
	units := []struct {
		suffix string
		mult   int64
	}{
		{"ms", 1},
		{"s", 1000},
		{"m", 60 * 1000},
		{"h", 60 * 60 * 1000},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			return parsePositiveUnitNumber(strings.TrimSuffix(s, u.suffix), u.mult)
		}
	}
	return 0, errInvalidUnit
}

func parsePositiveUnitNumber(num string, multiplier int64) (int64, error) {
	if num == "" {
		return 0, errInvalidUnit
	}
	for _, r := range num {
		if r < '0' || r > '9' {
			return 0, errInvalidUnit
		}
	}
	value, err := strconv.ParseUint(num, 10, 63)
	if err != nil || value == 0 {
		return 0, errInvalidUnit
	}
	if value > uint64(math.MaxInt64/int64(multiplier)) {
		return 0, errInvalidUnit
	}
	return int64(value) * multiplier, nil
}
