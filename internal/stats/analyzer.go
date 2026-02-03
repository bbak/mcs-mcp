package stats

import (
	"mcs-mcp/internal/jira"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MetadataSummary provides a high-level overview of a Jira data source's scope and volume.
type MetadataSummary struct {
	TotalIngestedIssues        int            `json:"totalIngestedIssues"`
	DiscoverySampleSize        int            `json:"discoverySampleSize"`
	IssueTypes                 map[string]int `json:"issueTypes"`
	ResolutionNames            map[string]int `json:"resolutionNames"`
	ResolutionDensity          float64        `json:"resolutionDensity"` // % of issues with a resolution
	FirstResolution            *time.Time     `json:"firstResolution,omitempty"`
	LastResolution             *time.Time     `json:"lastResolution,omitempty"`
	RecommendedCommitmentPoint string         `json:"recommendedCommitmentPoint,omitempty"`
}

// StatusMetadata holds the user-confirmed semantic mapping for a status.
type StatusMetadata struct {
	Role    string `json:"role"`
	Tier    string `json:"tier"`
	Outcome string `json:"outcome,omitempty"` // delivered, abandoned_demand, abandoned_upstream, abandoned_downstream
}

// SumRangeDuration calculates the total time spent in a list of statuses for a given issue.
func SumRangeDuration(issue jira.Issue, rangeStatuses []string) float64 {
	var total float64
	for _, status := range rangeStatuses {
		if s, ok := issue.StatusResidency[status]; ok {
			total += float64(s) / 86400.0
		}
	}
	return total
}

// AnalyzeProbe performs a preliminary analysis on a sample of issues to anchor the AI on the data shape.
func AnalyzeProbe(issues []jira.Issue, totalCount int, finishedStatuses map[string]bool) MetadataSummary {
	summary := MetadataSummary{
		TotalIngestedIssues: totalCount,
		DiscoverySampleSize: len(issues),
		IssueTypes:          make(map[string]int),
		ResolutionNames:     make(map[string]int),
	}

	if len(issues) == 0 {
		return summary
	}

	var first, last *time.Time
	reachableSet := make(map[string]bool)

	for _, issue := range issues {
		summary.IssueTypes[issue.IssueType]++
		if issue.Resolution != "" {
			summary.ResolutionNames[issue.Resolution]++
		}

		// Track reachability (all statuses ever visited in this sample)
		reachableSet[issue.Status] = true
		for _, t := range issue.Transitions {
			reachableSet[t.ToStatus] = true
		}

		if issue.ResolutionDate != nil {
			if first == nil || issue.ResolutionDate.Before(*first) {
				first = issue.ResolutionDate
			}
			if last == nil || issue.ResolutionDate.After(*last) {
				last = issue.ResolutionDate
			}
		}
	}

	if summary.DiscoverySampleSize > 0 {
		resolvedCount := 0
		for _, count := range summary.ResolutionNames {
			resolvedCount += count
		}
		summary.ResolutionDensity = float64(resolvedCount) / float64(summary.DiscoverySampleSize)
	}

	summary.FirstResolution = first
	summary.LastResolution = last

	return summary
}

// ProposeSemantics applies heuristics to suggest tiers and roles for a set of statuses.
func ProposeSemantics(issues []jira.Issue, persistence []StatusPersistence) (map[string]StatusMetadata, string) {
	proposal := make(map[string]StatusMetadata)
	if len(persistence) == 0 {
		return proposal, ""
	}

	// 1. Gather facts from actual data
	entryCounts := make(map[string]int) // Number of issues that started in this status
	resolvedCounts := make(map[string]int)
	reachability := make(map[string]int) // Total unique issues that visited this status
	transitionsInto := make(map[string]int)
	transitionsOutOf := make(map[string]int)

	for _, issue := range issues {
		// Entry point detection (Demand)
		if !issue.IsMoved && len(issue.Transitions) > 0 {
			entryCounts[issue.Transitions[0].FromStatus]++
		}

		// Resolution detection (Finished)
		if issue.ResolutionDate != nil {
			resolvedCounts[issue.Status]++
		}

		visited := make(map[string]bool)
		visited[issue.Status] = true
		for _, t := range issue.Transitions {
			visited[t.ToStatus] = true
			visited[t.FromStatus] = true
			transitionsInto[t.ToStatus]++
			transitionsOutOf[t.FromStatus]++
		}
		for s := range visited {
			reachability[s]++
		}
	}

	// Identify Birth Status (The source of demand)
	birthStatus := ""
	maxEntry := 0
	for s, count := range entryCounts {
		if count > maxEntry {
			maxEntry = count
			birthStatus = s
		}
	}

	// Get the Path Order for biasing Upstream/Downstream
	pathOrder := DiscoverStatusOrder(issues)
	pathIndices := make(map[string]int)
	for i, s := range pathOrder {
		pathIndices[s] = i
	}

	queueCandidates := findQueuingColumns(persistence)

	// Keyword sets for tier biasing
	upstreamKeywords := []string{"refine", "analyze", "prioritize", "architect", "groom", "backlog", "triage", "discovery", "upstream", "ready"}
	downstreamKeywords := []string{"develop", "implement", "do", "test", "verification", "review", "deployment", "integration", "downstream", "building", "staging", "qa", "uat", "prod", "fix"}
	deliveredKeywords := []string{"done", "resolved", "fixed", "complete", "approved", "shipped", "delivered"}
	abandonedKeywords := []string{"cancel", "discard", "obsolete", "reject", "decline", "won't do", "wont do", "dropped", "abort"}

	for _, p := range persistence {
		name := p.StatusName
		lowerName := strings.ToLower(name)
		tier := "Downstream" // Default catch-all
		role := "active"

		// --- TIER HEURISTICS ---

		// A. Demand (Entry) - Only if it's the primary birth status
		// and it doesn't have a high resolution density (which would make it a sink)
		isBirth := name == birthStatus

		// B. Finished (Terminal)
		// Probabilistic Fact-Based: More than 20% of reachability is resolved here.
		resDensity := 0.0
		if reach := reachability[name]; reach > 0 {
			resDensity = float64(resolvedCounts[name]) / float64(reach)
		}
		isResolvedDensity := resDensity > 0.20

		// Terminal Asymmetry: High entry, low exit (sinking).
		isTerminalSink := transitionsInto[name] > 5 && transitionsInto[name] > (transitionsOutOf[name]*4)

		if isResolvedDensity || isTerminalSink {
			tier = "Finished"
		} else if isBirth {
			tier = "Demand"
		} else {
			// C. Upstream vs Downstream biasing
			isUpstreamKeyword := matchesAny(lowerName, upstreamKeywords)
			isDownstreamKeyword := matchesAny(lowerName, downstreamKeywords)

			idx, inPath := pathIndices[name]
			isEarlyInPath := inPath && idx < (len(pathOrder)/2)

			if isUpstreamKeyword {
				tier = "Upstream"
			} else if isDownstreamKeyword {
				tier = "Downstream"
			} else if isEarlyInPath {
				tier = "Upstream"
			}
		}

		// --- ROLE HEURISTICS ---
		outcome := ""
		if tier == "Finished" {
			role = "terminal"
			if matchesAny(lowerName, abandonedKeywords) {
				outcome = "abandoned"
			} else if matchesAny(lowerName, deliveredKeywords) {
				outcome = "delivered"
			} else {
				outcome = "delivered" // Default terminal outcome
			}
		} else if tier == "Demand" || queueCandidates[name] {
			role = "queue"
		}

		proposal[name] = StatusMetadata{
			Tier:    tier,
			Role:    role,
			Outcome: outcome,
		}
	}

	// 4. Identify Recommended Commitment Point: First Downstream status in path order
	commitmentPoint := ""
	for _, s := range pathOrder {
		if m, ok := proposal[s]; ok && m.Tier == "Downstream" {
			commitmentPoint = s
			break
		}
	}
	// Fallback to first Upstream if no Downstream
	if commitmentPoint == "" {
		for _, s := range pathOrder {
			if m, ok := proposal[s]; ok && m.Tier == "Upstream" {
				commitmentPoint = s
				break
			}
		}
	}

	return proposal, commitmentPoint
}

func matchesAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// DiscoverStatusOrder derives the workflow backbone by tracing the most frequent journeys.
func DiscoverStatusOrder(issues []jira.Issue) []string {
	// 1. Build transition frequency matrix
	matrix := make(map[string]map[string]int)
	exitsTotal := make(map[string]int)
	entryCounts := make(map[string]int)
	allStatuses := make(map[string]bool)
	resolvedAt := make(map[string]int)
	reachability := make(map[string]int)

	for _, issue := range issues {
		allStatuses[issue.Status] = true
		visited := make(map[string]bool)
		visited[issue.Status] = true

		if issue.ResolutionDate != nil {
			resolvedAt[issue.Status]++
		}

		for i, t := range issue.Transitions {
			allStatuses[t.FromStatus] = true
			allStatuses[t.ToStatus] = true
			visited[t.FromStatus] = true
			visited[t.ToStatus] = true

			if matrix[t.FromStatus] == nil {
				matrix[t.FromStatus] = make(map[string]int)
			}
			matrix[t.FromStatus][t.ToStatus]++
			exitsTotal[t.FromStatus]++

			if i == 0 && !issue.IsMoved {
				entryCounts[t.FromStatus]++
			}
		}
		for s := range visited {
			reachability[s]++
		}
	}

	if len(allStatuses) == 0 {
		return nil
	}

	// 2. Identify Birth Status
	birthStatus := ""
	maxEntry := 0
	for s, count := range entryCounts {
		if count > maxEntry {
			maxEntry = count
			birthStatus = s
		}
	}

	if birthStatus == "" {
		for s := range allStatuses {
			birthStatus = s
			break
		}
	}

	// 3. Trace the "Happy Path" using Market-Share confidence
	var order []string
	visited := make(map[string]bool)
	current := birthStatus

	for current != "" {
		order = append(order, current)
		visited[current] = true

		successors := matrix[current]
		totalExits := exitsTotal[current]

		// Find next best status
		next := ""
		maxFreq := -1

		// Candidate discovery
		type candidate struct {
			name        string
			freq        int
			marketShare float64
			isTerminal  bool
		}
		var candidates []candidate
		for name, freq := range successors {
			if visited[name] {
				continue
			}
			share := float64(freq) / float64(totalExits)
			if share < 0.15 { // Minimum market share to be considered part of the "Backbone"
				continue
			}

			resDensity := 0.0
			if reach := reachability[name]; reach > 0 {
				resDensity = float64(resolvedAt[name]) / float64(reach)
			}

			candidates = append(candidates, candidate{
				name:        name,
				freq:        freq,
				marketShare: share,
				isTerminal:  resDensity > 0.25,
			})
		}

		if len(candidates) > 0 {
			// Selection Logic:
			// 1. Prefer non-terminal candidates with high share
			// 2. If only one candidate, take it
			// 3. If multiple, take the non-terminal with highest freq

			bestActive := -1
			bestTerminal := -1

			for i, c := range candidates {
				if c.isTerminal {
					if bestTerminal == -1 || c.freq > candidates[bestTerminal].freq || (c.freq == candidates[bestTerminal].freq && c.name < candidates[bestTerminal].name) {
						bestTerminal = i
					}
				} else {
					if bestActive == -1 || c.freq > candidates[bestActive].freq || (c.freq == candidates[bestActive].freq && c.name < candidates[bestActive].name) {
						bestActive = i
					}
				}
			}

			if bestActive != -1 {
				next = candidates[bestActive].name
			} else if bestTerminal != -1 {
				next = candidates[bestTerminal].name
			}
		}

		if next == "" {
			// Fallback: try pure frequency if no candidates met the share threshold
			maxFreq = -1
			for name, freq := range successors {
				if !visited[name] {
					if next == "" || freq > maxFreq || (freq == maxFreq && name < next) {
						maxFreq = freq
						next = name
					}
				}
			}
		}

		current = next
	}

	// 4. Add any orphaned statuses using dominance-based sorting
	var orphans []string
	for s := range allStatuses {
		if !visited[s] {
			orphans = append(orphans, s)
		}
	}

	if len(orphans) > 0 {
		sort.Slice(orphans, func(i, j int) bool {
			s1, s2 := orphans[i], orphans[j]
			f12 := matrix[s1][s2]
			f21 := matrix[s2][s1]
			if f12 != f21 {
				return f12 > f21
			}
			return s1 < s2
		})
		order = append(order, orphans...)
	}

	return order
}

// findQueuingColumns applies regex-based heuristics to identify queue/buffer statuses.
//
// [STABILITY GUARDRAIL]
// This logic is central to the system's "Conceptual Integrity". The regex priority
// (isQueue > isActive) and the stemming normalization are carefully tuned to discover
// workflow pairs. CASUAL MODIFICATION MAY BREAK WIP VS LEAD-TIME MEASUREMENTS.
func findQueuingColumns(persistence []StatusPersistence) map[string]bool {
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

// SelectDiscoverySample filters a set of issues to provide a 200-item "healthy" subset for discovery.
// It prioritizes items created within 1 year, expanding to 2 or 3 years only if the sample is sparse.
func SelectDiscoverySample(issues []jira.Issue, targetSize int) []jira.Issue {
	if len(issues) <= targetSize {
		return issues
	}

	// 1. Sort by Updated DESC to ensure we get most recent activity
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Updated.After(issues[j].Updated)
	})

	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)
	twoYearsAgo := now.AddDate(-2, 0, 0)
	threeYearsAgo := now.AddDate(-3, 0, 0)

	var pool1y []jira.Issue
	for _, iss := range issues {
		if !iss.Created.Before(oneYearAgo) {
			pool1y = append(pool1y, iss)
		}
	}

	// 2. Check if we have enough 1y items
	if len(pool1y) >= targetSize {
		return pool1y[:targetSize]
	}

	// 3. Expansion Logic
	var fallbackPool []jira.Issue
	limitDate := twoYearsAgo
	if len(pool1y) < 100 {
		limitDate = threeYearsAgo
	}

	for _, iss := range issues {
		// Only consider items OLDER than 1y but NEWER than limit
		if iss.Created.Before(oneYearAgo) && iss.Created.After(limitDate) {
			fallbackPool = append(fallbackPool, iss)
		}
	}

	// Union
	result := append([]jira.Issue{}, pool1y...)
	remaining := targetSize - len(result)
	if remaining > 0 {
		if len(fallbackPool) > remaining {
			result = append(result, fallbackPool[:remaining]...)
		} else {
			result = append(result, fallbackPool...)
		}
	}

	return result
}
