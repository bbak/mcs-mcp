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

// GetWeightRobust retrieves the weight from the map using ID priority then case-insensitive name.
func GetWeightRobust(weights map[string]int, statusID, statusName string) (int, bool) {
	if statusID != "" {
		if val, ok := weights[statusID]; ok {
			return val, true
		}
	}

	if statusName != "" {
		if val, ok := weights[statusName]; ok {
			return val, true
		}
		lower := strings.ToLower(statusName)
		for k, v := range weights {
			if strings.ToLower(k) == lower {
				return v, true
			}
		}
	}
	return 0, false
}

// GetMetadataRobust retrieves status metadata using ID priority then case-insensitive name.
func GetMetadataRobust(mappings map[string]StatusMetadata, statusID, statusName string) (StatusMetadata, bool) {
	if statusID != "" {
		if val, ok := mappings[statusID]; ok {
			return val, true
		}
	}

	if statusName != "" {
		if val, ok := mappings[statusName]; ok {
			return val, true
		}
		lower := strings.ToLower(statusName)
		for k, v := range mappings {
			if strings.ToLower(k) == lower {
				return v, true
			}
		}
	}
	return StatusMetadata{}, false
}

// EqualFold returns true if s1 and s2 are equal under Unicode case-folding.
func EqualFold(s1, s2 string) bool {
	return strings.EqualFold(s1, s2)
}
