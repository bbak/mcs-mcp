package stats

import "strings"

// GetResidencyCaseInsensitive retrieves the residency value from the map using a case-insensitive lookup.
func GetResidencyCaseInsensitive(residency map[string]int64, statusName string) (int64, bool) {
	if val, ok := residency[statusName]; ok {
		return val, true
	}
	lower := strings.ToLower(statusName)
	for k, v := range residency {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return 0, false
}

// GetWeightCaseInsensitive retrieves the weight from the map using a case-insensitive lookup.
func GetWeightCaseInsensitive(weights map[string]int, statusName string) (int, bool) {
	if val, ok := weights[statusName]; ok {
		return val, true
	}
	lower := strings.ToLower(statusName)
	for k, v := range weights {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return 0, false
}

// GetMetadataCaseInsensitive retrieves status metadata from the map using a case-insensitive lookup.
func GetMetadataCaseInsensitive(mappings map[string]StatusMetadata, statusName string) (StatusMetadata, bool) {
	if val, ok := mappings[statusName]; ok {
		return val, true
	}
	lower := strings.ToLower(statusName)
	for k, v := range mappings {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return StatusMetadata{}, false
}

// EqualFold returns true if s1 and s2 are equal under Unicode case-folding.
func EqualFold(s1, s2 string) bool {
	return strings.EqualFold(s1, s2)
}
