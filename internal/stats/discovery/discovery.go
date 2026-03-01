package discovery

import (
	"cmp"
	"math"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"regexp"
	"slices"
	"strings"
	"time"
)

// DiscoveryResult encapsulates the output of the workflow discovery process.
type DiscoveryResult struct {
	Summary         stats.MetadataSummary
	Proposal        map[string]stats.StatusMetadata
	CommitmentPoint string
	StatusOrder     []string
	Sample          []jira.Issue
}

// DiscoverWorkflow orchestrates the discovery process.
func DiscoverWorkflow(events []eventlog.IssueEvent, sample []jira.Issue, resolutions map[string]string) DiscoveryResult {
	persistence := stats.CalculateStatusPersistence(sample)
	proposal, cp, refinedOrder := ProposeSemantics(sample, persistence)
	summary := AnalyzeProbe(sample, len(sample)) // simplified for now

	return DiscoveryResult{
		Summary:         summary,
		Proposal:        proposal,
		CommitmentPoint: cp,
		StatusOrder:     refinedOrder,
		Sample:          sample,
	}
}

// AnalyzeProbe performs a characterization analysis on a sample of issues.
func AnalyzeProbe(sample []jira.Issue, totalCount int) stats.MetadataSummary {
	summary := stats.MetadataSummary{
		Whole: stats.WholeDatasetStats{
			TotalItems: totalCount,
		},
		Sample: stats.SampleDatasetStats{
			SampleSize:      len(sample),
			WorkItemWeights: make(map[string]float64),
		},
	}

	if totalCount > 0 {
		summary.Sample.PercentageOfWhole = math.Round((float64(len(sample))/float64(totalCount))*1000) / 10
	}

	if len(sample) == 0 {
		return summary
	}

	typeCounts := make(map[string]int)
	resNames := make(map[string]bool)
	resolvedCount := 0

	for _, issue := range sample {
		typeCounts[issue.IssueType]++
		if issue.Resolution != "" {
			resNames[issue.Resolution] = true
			resolvedCount++
		}
	}

	// Calculate distributions
	for t, count := range typeCounts {
		summary.Sample.WorkItemWeights[t] = math.Round((float64(count)/float64(len(sample)))*100) / 100
	}

	for name := range resNames {
		summary.Sample.ResolutionNames = append(summary.Sample.ResolutionNames, name)
	}
	slices.Sort(summary.Sample.ResolutionNames)

	summary.Sample.ResolutionDensity = math.Round((float64(resolvedCount)/float64(len(sample)))*100) / 100

	return summary
}

// CalculateDiscoveryCutoff identifies the steady-state cutoff by finding the 5th delivery date.
func CalculateDiscoveryCutoff(issues []jira.Issue, isFinished map[string]bool) *time.Time {
	var deliveryDates []time.Time

	for _, issue := range issues {
		isFin := isFinished[issue.Status] || (issue.StatusID != "" && isFinished[issue.StatusID])
		if issue.ResolutionDate != nil && isFin {
			deliveryDates = append(deliveryDates, *issue.ResolutionDate)
		}
	}

	if len(deliveryDates) < 5 {
		return nil
	}

	// Sort deliveries chronologically
	slices.SortFunc(deliveryDates, func(a, b time.Time) int {
		return a.Compare(b)
	})

	// The cutoff is the timestamp of the 5th delivery.
	// This ensures we only start analyzing once the system has demonstrated delivery capacity.
	cutoff := deliveryDates[4]
	return &cutoff
}

// ProposeSemantics infers missing workflow semantics based on heuristics.
// It returns a mapping of status IDs to semantics, the recommended commitment point, and the refined sequential order.
func ProposeSemantics(issues []jira.Issue, persistence []stats.StatusPersistence) (map[string]stats.StatusMetadata, string, []string) {
	proposal := make(map[string]stats.StatusMetadata)
	if len(persistence) == 0 {
		return proposal, "", nil
	}

	// 1. Gather facts from actual data (keyed by ID)
	entryCounts := make(map[string]int) // Number of issues that started in this status
	resolvedCounts := make(map[string]int)
	reachability := make(map[string]int) // Total unique issues that visited this status
	transitionsInto := make(map[string]int)
	transitionsOutOf := make(map[string]int)

	for _, issue := range issues {
		// Entry point detection (Demand) using birth status
		birth := stats.PreferID(issue.BirthStatusID, issue.BirthStatus)
		if birth != "" {
			entryCounts[birth]++
		}

		// Resolution detection (Finished)
		curr := stats.PreferID(issue.StatusID, issue.Status)
		if issue.ResolutionDate != nil && curr != "" {
			resolvedCounts[curr]++
		}

		visited := make(map[string]bool)
		if curr != "" {
			visited[curr] = true
		}
		for _, t := range issue.Transitions {
			to := stats.PreferID(t.ToStatusID, t.ToStatus)
			from := stats.PreferID(t.FromStatusID, t.FromStatus)
			visited[to] = true
			visited[from] = true
			transitionsInto[to]++
			transitionsOutOf[from]++
		}
		for s := range visited {
			reachability[s]++
		}
	}

	// Get the Path Order for biasing Upstream/Downstream (already ID-based)
	pathOrder := DiscoverStatusOrder(issues)
	pathIndices := make(map[string]int)
	for i, s := range pathOrder {
		pathIndices[s] = i
	}

	// queueCandidates is keyed by name (regex-based heuristic)
	queueCandidates := findQueuingColumns(persistence)

	// Keyword sets for tier biasing (applied to human-readable names)
	upstreamKeywords := []string{"refine", "analyze", "prioritize", "architect", "groom", "backlog", "triage", "discovery", "upstream", "ready"}
	downstreamKeywords := []string{"develop", "implement", "do", "test", "verification", "review", "deployment", "integration", "downstream", "building", "staging", "qa", "uat", "prod", "fix"}
	abandonedKeywords := []string{"cancel", "discard", "obsolete", "reject", "decline", "won't do", "wont do", "dropped", "abort"}
	deliveredKeywords := []string{"done", "resolved", "fixed", "complete", "approved", "shipped", "delivered"}

	// Regex for detecting delivered vs abandoned resolutions
	deliveredResolutionRegex := regexp.MustCompile(`(?i)(?:(?:fix|deliver|resolve|release|complete|shipp?|approve|deploy)(?:e?d)?)|done`)

	// Track workflow state to prevent Upstream from appearing after Downstream
	seenDownstream := false

	// Iterate over the discovered path order to ensure sequential logic applies
	for i, statusID := range pathOrder {
		var p *stats.StatusPersistence
		for j := range persistence {
			pid := stats.PreferID(persistence[j].StatusID, persistence[j].StatusName)
			if pid == statusID {
				p = &persistence[j]
				break
			}
		}

		if p == nil {
			continue // Should not happen, but defensive
		}

		name := p.StatusName
		lowerName := strings.ToLower(name)
		tier := "Downstream" // Default catch-all
		role := "active"

		// --- TIER HEURISTICS ---

		// 1. Demand (Entry) - Strictly the first status in the path order
		if i == 0 {
			tier = "Demand"
		} else {
			// 2. Finished (Terminal)

			// Probabilistic Fact-Based: More than 20% of reachability is resolved here.
			resDensity := 0.0
			if reach := reachability[statusID]; reach > 0 {
				resDensity = float64(resolvedCounts[statusID]) / float64(reach)
			}
			isResolvedDensity := resDensity > 0.20

			// Terminal Asymmetry: High entry, low exit (sinking).
			isTerminalSink := transitionsInto[statusID] > 5 && transitionsInto[statusID] > (transitionsOutOf[statusID]*4)

			// Keyword Fallbacks
			isAbandonedKeyword := matchesAny(lowerName, abandonedKeywords)
			isDeliveredKeyword := matchesAny(lowerName, deliveredKeywords)

			if isResolvedDensity || isTerminalSink || isAbandonedKeyword || isDeliveredKeyword {
				tier = "Finished"
			} else {
				// 3. Upstream vs Downstream biasing
				isUpstreamKeyword := matchesAny(lowerName, upstreamKeywords)
				isDownstreamKeyword := matchesAny(lowerName, downstreamKeywords)

				if isDownstreamKeyword {
					tier = "Downstream"
					seenDownstream = true
				} else if isUpstreamKeyword && !seenDownstream {
					// Only allow Upstream if we haven't crossed into Downstream territory
					tier = "Upstream"
				} else if !seenDownstream && i < (len(pathOrder)/2) {
					// Fallback for early unknown statuses before the midpoint
					tier = "Upstream"
				} else {
					// Default to Downstream or forced Downstream because we already passed the point of no return
					tier = "Downstream"
					seenDownstream = true
				}
			}
		}

		// --- ROLE & OUTCOME HEURISTICS ---
		outcome := ""
		if tier == "Finished" {
			role = "terminal"

			// Final fallback logic if resolution checking down below doesn't trigger
			if matchesAny(lowerName, abandonedKeywords) {
				outcome = "abandoned"
			} else {
				outcome = "delivered"
			}
		} else if tier == "Demand" || queueCandidates[name] {
			role = "queue"
		}

		proposal[statusID] = stats.StatusMetadata{
			Name:    name,
			Tier:    tier,
			Role:    role,
			Outcome: outcome,
		}
	}

	// Refine Outcomes based on actual Resolutions
	for _, issue := range issues {
		curr := stats.PreferID(issue.StatusID, issue.Status)
		if issue.ResolutionDate != nil && curr != "" && issue.Resolution != "" {
			if m, ok := proposal[curr]; ok && m.Tier == "Finished" {
				if deliveredResolutionRegex.MatchString(issue.Resolution) {
					m.Outcome = "delivered"
				} else {
					// If a state has BOTH delivered and abandoned resolutions, we bias towards delivered.
					// So only set to abandoned if it hasn't already been confirmed delivered.
					if m.Outcome != "delivered" {
						m.Outcome = "abandoned"
					}
				}
				proposal[curr] = m
			}
		}
	}

	// Post-processing rule: Terminal States are ALWAYS last!
	var activeOrder []string
	var terminalOrder []string
	for _, statusID := range pathOrder {
		if m, ok := proposal[statusID]; ok && m.Tier == "Finished" {
			terminalOrder = append(terminalOrder, statusID)
		} else {
			activeOrder = append(activeOrder, statusID)
		}
	}
	refinedOrder := append(activeOrder, terminalOrder...)

	// 5. Deduce the Primary Commitment Point
	commitmentPoint := ""
	for _, statusID := range refinedOrder {
		if m, ok := proposal[statusID]; ok && m.Tier == "Downstream" {
			commitmentPoint = stats.PreferID(statusID, proposal[statusID].Name) // Fallback to name if ID is missing
			break
		}
	}
	// Fallback to first Upstream if no Downstream
	if commitmentPoint == "" {
		for _, statusID := range refinedOrder {
			if m, ok := proposal[statusID]; ok && m.Tier == "Upstream" {
				commitmentPoint = stats.PreferID(statusID, proposal[statusID].Name)
				break
			}
		}
	}

	return proposal, commitmentPoint, refinedOrder
}

func matchesAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

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

// SelectDiscoverySample filters a set of issues to provide a 200-item "healthy" subset for discovery.
func SelectDiscoverySample(issues []jira.Issue, targetSize int) []jira.Issue {
	if len(issues) <= targetSize {
		return issues
	}

	// 1. Sort by Updated DESC to ensure we get most recent activity
	slices.SortStableFunc(issues, func(a, b jira.Issue) int {
		return b.Updated.Compare(a.Updated)
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
