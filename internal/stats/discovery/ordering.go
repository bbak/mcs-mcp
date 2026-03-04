package discovery

import (
	"cmp"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"regexp"
	"slices"
	"strings"
)

// DiscoverStatusOrder derives the workflow backbone by analyzing temporal precedence across all issues.
// The returned order uses status IDs when available, falling back to names for legacy data.
func DiscoverStatusOrder(issues []jira.Issue) []string {
	if len(issues) == 0 {
		return nil
	}

	// 1. Initialize data structures
	entryCounts := make(map[string]int)
	allStatuses := make(map[string]bool)
	resolvedAt := make(map[string]int)
	reachability := make(map[string]int)

	// precedes[A][B] = count of items where A appeared before B
	precedes := make(map[string]map[string]int)

	// 2. Build the Precedence Matrix
	for _, issue := range issues {
		issueBirth := stats.PreferID(issue.BirthStatusID, issue.BirthStatus)
		if issueBirth != "" {
			entryCounts[issueBirth]++
		}

		// Extract unique status sequence in order of appearance
		var sequence []string
		seenInIssue := make(map[string]bool)

		// Start with birth
		if issueBirth != "" {
			sequence = append(sequence, issueBirth)
			seenInIssue[issueBirth] = true
			allStatuses[issueBirth] = true
		}

		// Append transitions
		for _, t := range issue.Transitions {
			from := stats.PreferID(t.FromStatusID, t.FromStatus)
			to := stats.PreferID(t.ToStatusID, t.ToStatus)
			allStatuses[from] = true
			allStatuses[to] = true

			if !seenInIssue[from] {
				sequence = append(sequence, from)
				seenInIssue[from] = true
			}
			if !seenInIssue[to] {
				sequence = append(sequence, to)
				seenInIssue[to] = true
			}
		}

		// Track current status if not already in sequence (WIP items)
		curr := stats.PreferID(issue.StatusID, issue.Status)
		if curr != "" {
			allStatuses[curr] = true
			if !seenInIssue[curr] {
				sequence = append(sequence, curr)
				seenInIssue[curr] = true
			}
			if issue.ResolutionDate != nil {
				resolvedAt[curr]++
			}
		}

		// Update reachability for all discovered statuses in this issue
		for s := range seenInIssue {
			reachability[s]++
		}

		// Update the global precedence matrix for all unique pairs (A, B) where A precedes B
		for i := 0; i < len(sequence); i++ {
			for j := i + 1; j < len(sequence); j++ {
				A, B := sequence[i], sequence[j]
				if precedes[A] == nil {
					precedes[A] = make(map[string]int)
				}
				precedes[A][B]++
			}
		}
	}

	// 3. Calculate Precedence Scores
	// A status gets a point for every other status it "globally precedes"
	// (i.e., appears before it more often than after it).
	type statusInfo struct {
		id         string
		score      int
		birthCount int
	}
	var infos []statusInfo
	for s := range allStatuses {
		score := 0
		for other := range allStatuses {
			if s == other {
				continue
			}
			// Does s generally precede other?
			forward := precedes[s][other]
			backward := precedes[other][s]
			if forward > backward {
				score++
			}
		}
		infos = append(infos, statusInfo{
			id:         s,
			score:      score,
			birthCount: entryCounts[s],
		})
	}

	// 4. Sort statuses by Global Precedence
	slices.SortFunc(infos, func(a, b statusInfo) int {
		// Primary: Higher precedence score (more statuses follow it)
		if a.score != b.score {
			return cmp.Compare(b.score, a.score)
		}
		// Secondary: Higher birth frequency (entry points)
		if a.birthCount != b.birthCount {
			return cmp.Compare(b.birthCount, a.birthCount)
		}
		// Tertiary: Alphabetical/numeric for determinism
		return cmp.Compare(a.id, b.id)
	})

	order := make([]string, len(infos))
	for i, info := range infos {
		order[i] = info.id
	}

	return order
}

// findQueuingColumns applies regex-based heuristics to identify queue/buffer statuses.
func findQueuingColumns(persistence []stats.StatusPersistence) map[string]bool {
	queues := make(map[string]bool)
	lowerConfigs := make(map[string]string)
	for _, p := range persistence {
		lowerConfigs[strings.ToLower(p.StatusName)] = p.StatusName
	}

	queueRegex := regexp.MustCompile(`(?i)^(?:ready for|awaiting|waiting\s+for|pending|to be|next)\s+([\w\s-]+?)(?:\s+ed|ment|ion)?$`)
	suffixOnlyQueueRegex := regexp.MustCompile(`(?i)^([\w\s-]+?)ed$`)
	activeRegex := regexp.MustCompile(`(?i)^(?:in\s+[\w\s-]+|[\w\s-]+ing)$`)

	isQueue := func(s string) bool {
		return queueRegex.MatchString(s) || suffixOnlyQueueRegex.MatchString(s)
	}
	isActive := func(s string) bool {
		// Priority: If it looks like a queue, it's NOT active.
		return activeRegex.MatchString(s) && !isQueue(s)
	}

	// 1. Direct Role Matching
	for lower, original := range lowerConfigs {
		if isQueue(lower) {
			queues[original] = true
		}
	}

	// 2. Pair Discovery (Stem matching)
	stems := make(map[string]string) // stem -> original status name
	for lower, original := range lowerConfigs {
		stem := extractStatusStem(lower)
		if stem == "" {
			continue
		}

		if existing, ok := stems[stem]; ok {
			lowerExisting := strings.ToLower(existing)
			// Confirm it's a valid semantic pair (one queue, one active)
			if isQueue(lower) && isActive(lowerExisting) {
				queues[original] = true
			} else if isQueue(lowerExisting) && isActive(lower) {
				queues[existing] = true
			}
		} else {
			stems[stem] = original
		}
	}

	return queues
}

func extractStatusStem(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Unify: Strip prefixes and suffixes to get the 'core'
	re := regexp.MustCompile(`(?i)^(?:ready for|awaiting|waiting\s+for|pending|to be|in|at)\s+|(?:\s+ing|ed|ment|ion|stage)$`)
	stem := re.ReplaceAllString(s, "")
	return strings.TrimSpace(stem)
}
