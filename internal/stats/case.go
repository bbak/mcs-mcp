package stats

import "strings"

// PreferID returns the ID if non-empty, otherwise falls back to the name.
// This is the canonical ID-first resolution pattern for the migration.
func PreferID(id, name string) string {
	if id != "" {
		return id
	}
	return name
}

// EqualFold returns true if s1 and s2 are equal under Unicode case-folding.
func EqualFold(s1, s2 string) bool {
	return strings.EqualFold(s1, s2)
}

// ExtractProjectKey extracts the project key portion from a Jira issue key (e.g., "PROJ" from "PROJ-123").
func ExtractProjectKey(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == '-' {
			return key[:i]
		}
	}
	return key
}
