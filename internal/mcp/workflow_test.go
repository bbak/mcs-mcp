package mcp

import (
	"reflect"
	"testing"
)

func TestSliceRange(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name     string
		order    []string
		start    string
		end      string
		expected []string
	}{
		{
			name:     "Empty order",
			order:    []string{},
			start:    "A",
			end:      "C",
			expected: []string{},
		},
		{
			name:     "Normal range",
			order:    []string{"A", "B", "C", "D"},
			start:    "B",
			end:      "C",
			expected: []string{"B", "C"},
		},
		{
			name:     "Invalid range (start after end)",
			order:    []string{"A", "B", "C", "D"},
			start:    "C",
			end:      "B",
			expected: []string{"C"},
		},
		{
			name:     "Full range",
			order:    []string{"A", "B"},
			start:    "",
			end:      "",
			expected: []string{"A", "B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.sliceRange(tt.order, tt.start, tt.end)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("sliceRange() = %v, want %v", got, tt.expected)
			}
		})
	}
}
