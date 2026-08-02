package logging

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestVerboseLevel(t *testing.T) {
	tests := []struct {
		raw  string
		want zerolog.Level
	}{
		{"true", zerolog.DebugLevel},
		{"TRUE", zerolog.DebugLevel},
		{"True", zerolog.DebugLevel},
		{"1", zerolog.DebugLevel},
		{"t", zerolog.DebugLevel},
		{"  true  ", zerolog.DebugLevel},
		{"false", zerolog.InfoLevel},
		{"0", zerolog.InfoLevel},
		{"", zerolog.InfoLevel},
		{"garbage", zerolog.InfoLevel},
		{"yes", zerolog.InfoLevel}, // ParseBool rejects it, so it stays off
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			if got := verboseLevel(tc.raw); got != tc.want {
				t.Errorf("verboseLevel(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
