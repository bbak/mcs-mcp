package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"regexp"
	"sort"
	"strings"
	"time"
)

type MetadataSummary struct {
	Whole                      WholeDatasetStats  `json:"whole"`
	Sample                     SampleDatasetStats `json:"sample"`
	RecommendedCommitmentPoint string             `json:"recommendedCommitmentPoint,omitempty"`
}

type WholeDatasetStats struct {
	TotalItems   int       `json:"total_items"`
	FirstEventAt time.Time `json:"first_event_at"`
	LastEventAt  time.Time `json:"last_event_at"`
}

type SampleDatasetStats struct {
	SampleSize        int                `json:"sample_size"`
	PercentageOfWhole float64            `json:"percentage_of_whole"`
	WorkItemWeights   map[string]float64 `json:"work_item_distribution"`
	ResolutionNames   []string           `json:"resolutions"`
	ResolutionDensity float64            `json:"resolution_density"`
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
		if s, ok := GetResidencyCaseInsensitive(issue.StatusResidency, status); ok {
			total += float64(s) / 86400.0
		}
	}
	return total
}

// AnalyzeProbe performs a characterization analysis on a sample of issues.
func AnalyzeProbe(sample []jira.Issue, totalCount int, finishedStatuses map[string]bool) MetadataSummary {
	summary := MetadataSummary{
		Whole: WholeDatasetStats{
			TotalItems: totalCount,
		},
		Sample: SampleDatasetStats{
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
	sort.Strings(summary.Sample.ResolutionNames)

	summary.Sample.ResolutionDensity = math.Round((float64(resolvedCount)/float64(len(sample)))*100) / 100

	return summary
}

// CalculateDiscoveryCutoff identifies the steady-state cutoff by finding the 5th delivery date.
func CalculateDiscoveryCutoff(issues []jira.Issue, isFinished map[string]bool) *time.Time {
	var deliveryDates []time.Time

	for _, issue := range issues {
		if issue.ResolutionDate != nil && isFinished[issue.Status] {
			deliveryDates = append(deliveryDates, *issue.ResolutionDate)
		}
	}

	if len(deliveryDates) < 5 {
		return nil
	}

	// Sort deliveries chronologically
	sort.Slice(deliveryDates, func(i, j int) bool {
		return deliveryDates[i].Before(deliveryDates[j])
	})

	// The cutoff is the timestamp of the 5th delivery.
	// This ensures we only start analyzing once the system has demonstrated delivery capacity.
	cutoff := deliveryDates[4]
	return &cutoff
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
		// Entry point detection (Demand) using birth status
		if issue.BirthStatus != "" {
			entryCounts[issue.BirthStatus]++
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

// DiscoverStatusOrder derives the workflow backbone by analyzing temporal precedence across all issues.
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

	// Canonical casing map: lower -> original
	canonical := make(map[string]string)
	getCanonical := func(s string) string {
		lower := strings.ToLower(s)
		if existing, ok := canonical[lower]; ok {
			return existing
		}
		canonical[lower] = s
		return s
	}

	// 2. Build the Precedence Matrix
	for _, issue := range issues {
		issueBirth := getCanonical(issue.BirthStatus)
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
			from := getCanonical(t.FromStatus)
			to := getCanonical(t.ToStatus)
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
		curr := getCanonical(issue.Status)
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
		name       string
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
			name:       s,
			score:      score,
			birthCount: entryCounts[s],
		})
	}

	// 4. Sort statuses by Global Precedence
	sort.Slice(infos, func(i, j int) bool {
		// Primary: Higher precedence score (more statuses follow it)
		if infos[i].score != infos[j].score {
			return infos[i].score > infos[j].score
		}
		// Secondary: Higher birth frequency (entry points)
		if infos[i].birthCount != infos[j].birthCount {
			return infos[i].birthCount > infos[j].birthCount
		}
		// Tertiary: Alphabetical for determinism
		return infos[i].name < infos[j].name
	})

	order := make([]string, len(infos))
	for i, info := range infos {
		order[i] = info.name
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
