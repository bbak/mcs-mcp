package stats

import (
	"mcs-mcp/internal/jira"
	"sort"
	"strings"
	"time"
)

// MetadataSummary provides a high-level overview of a Jira data source.
type MetadataSummary struct {
	TotalIssues            int                       `json:"totalIssues"`
	SampleSize             int                       `json:"sampleSize"`
	IssueTypes             map[string]int            `json:"issueTypes"`
	Statuses               map[string]int            `json:"statuses"`
	ResolutionNames        map[string]int            `json:"resolutionNames"`
	SampleResolvedRatio    float64                   `json:"sampleResolvedRatio"` // Diagnostic: % of sample with resolution
	CurrentWIPCount        int                       `json:"currentWIPCount"`
	CurrentBacklogCount    int                       `json:"currentBacklogCount"`
	FirstResolution        *time.Time                `json:"firstResolution,omitempty"`
	LastResolution         *time.Time                `json:"lastResolution,omitempty"`
	AverageCycleTime       float64                   `json:"averageCycleTime,omitempty"` // Days
	AvailableStatuses      interface{}               `json:"availableStatuses,omitempty"`
	HistoricalReachability map[string]int            `json:"historicalReachability,omitempty"` // How many issues visited each status
	StatusAtResolution     map[string]int            `json:"statusAtResolution"`               // Frequency of Status when ResolutionDate is set
	ResolutionToStatus     map[string]map[string]int `json:"resolutionToStatus"`               // Resolution -> Status -> Count correlation
	CommitmentPointHints   []string                  `json:"commitmentPointHints,omitempty"`
	BacklogSize            int                       `json:"backlogSize,omitempty"`
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

// AnalyzeProbe performs a preliminary analysis on a sample of issues.
func AnalyzeProbe(issues []jira.Issue, totalCount int, finishedStatuses map[string]bool) MetadataSummary {
	summary := MetadataSummary{
		TotalIssues:            totalCount,
		SampleSize:             len(issues),
		IssueTypes:             make(map[string]int),
		Statuses:               make(map[string]int),
		ResolutionNames:        make(map[string]int),
		HistoricalReachability: make(map[string]int),
		StatusAtResolution:     make(map[string]int),
		ResolutionToStatus:     make(map[string]map[string]int),
	}

	if len(issues) == 0 {
		return summary
	}

	resolvedCount := 0
	var first, last *time.Time

	for _, issue := range issues {
		summary.IssueTypes[issue.IssueType]++
		summary.Statuses[issue.Status]++
		if issue.Resolution != "" {
			summary.ResolutionNames[issue.Resolution]++
		}

		// Track reachability from transitions
		visited := make(map[string]bool)
		visited[issue.Status] = true
		for _, t := range issue.Transitions {
			visited[t.ToStatus] = true
		}
		for status := range visited {
			summary.HistoricalReachability[status]++
		}

		if issue.ResolutionDate != nil {
			resolvedCount++
			summary.StatusAtResolution[issue.Status]++
			if issue.Resolution != "" {
				if _, ok := summary.ResolutionToStatus[issue.Resolution]; !ok {
					summary.ResolutionToStatus[issue.Resolution] = make(map[string]int)
				}
				summary.ResolutionToStatus[issue.Resolution][issue.Status]++
			}

			if first == nil || issue.ResolutionDate.Before(*first) {
				first = issue.ResolutionDate
			}
			if last == nil || issue.ResolutionDate.After(*last) {
				last = issue.ResolutionDate
			}
		}

		// Inventory Heuristic (Category-Aware)
		// We only count items that have NO resolution AND are not in a mapped Finished status
		if issue.ResolutionDate == nil && !finishedStatuses[issue.Status] {
			switch issue.StatusCategory {
			case "indeterminate":
				summary.CurrentWIPCount++
			case "to-do", "new":
				summary.CurrentBacklogCount++
			default:
				// Fallback: if category is unknown or missing, try common names
				if issue.Status == "In Progress" || issue.Status == "Development" || issue.Status == "QA" || issue.Status == "Testing" {
					summary.CurrentWIPCount++
				} else if issue.StatusCategory != "done" {
					// Only count as backlog if it's NOT in the 'done' category
					summary.CurrentBacklogCount++
				}
			}
		}
	}

	summary.SampleResolvedRatio = float64(resolvedCount) / float64(len(issues))
	summary.FirstResolution = first
	summary.LastResolution = last

	return summary
}

// ProposeSemantics applies heuristics to suggest tiers and roles for a set of statuses.
func ProposeSemantics(issues []jira.Issue, persistence []StatusPersistence, statusCats map[string]string) map[string]StatusMetadata {
	proposal := make(map[string]StatusMetadata)
	if len(persistence) == 0 {
		return proposal
	}

	// 1. Identify the Birth Status (Entry point)
	entryCounts := make(map[string]int)
	for _, issue := range issues {
		if !issue.IsMoved && len(issue.Transitions) > 0 {
			entryCounts[issue.Transitions[0].FromStatus]++
		}
	}
	birthStatus := ""
	maxEntry := 0
	for s, count := range entryCounts {
		if count > maxEntry {
			maxEntry = count
			birthStatus = s
		}
	}

	queueCandidates := findQueuingColumns(persistence)

	for _, p := range persistence {
		name := p.StatusName
		cat := statusCats[name]
		tier := "Downstream" // Default catch-all
		role := "active"

		// Tier Assignment Logic (Refined)
		if name == birthStatus {
			tier = "Demand"
		} else if cat == "done" || cat == "finished" || cat == "complete" {
			tier = "Finished"
		} else if cat == "to-live" || cat == "to-do" || cat == "new" {
			tier = "Upstream"
		} else if cat == "indeterminate" {
			// Historically, Jira "Indeterminate" covers everything In Progress.
			// However, if we've already identified Upstream vs Downstream based on path,
			// we can refine this. For the proposal we stick to the category signal.
			tier = "Downstream"
		}

		// Role Assignment
		if tier == "Demand" {
			role = "queue"
		} else if tier == "Finished" {
			role = "active"
		} else if queueCandidates[name] {
			role = "queue"
		}

		proposal[name] = StatusMetadata{
			Tier: tier,
			Role: role,
		}
	}

	return proposal
}

// DiscoverStatusOrder derives the workflow backbone by tracing the most frequent journeys.
func DiscoverStatusOrder(issues []jira.Issue) []string {
	// 1. Build transition frequency matrix
	matrix := make(map[string]map[string]int)
	entryCounts := make(map[string]int)
	allStatuses := make(map[string]bool)

	for _, issue := range issues {
		allStatuses[issue.Status] = true
		for i, t := range issue.Transitions {
			allStatuses[t.FromStatus] = true
			allStatuses[t.ToStatus] = true
			if matrix[t.FromStatus] == nil {
				matrix[t.FromStatus] = make(map[string]int)
			}
			matrix[t.FromStatus][t.ToStatus]++
			if i == 0 && !issue.IsMoved {
				entryCounts[t.FromStatus]++
			}
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

	// Fallback if no explicit birth status found
	if birthStatus == "" {
		for s := range allStatuses {
			birthStatus = s
			break
		}
	}

	// 3. Trace the "Happy Path" using greedy most-frequent successors
	var order []string
	visited := make(map[string]bool)
	current := birthStatus

	for current != "" {
		order = append(order, current)
		visited[current] = true

		// Find next best status
		next := ""
		maxFreq := -1
		successors := matrix[current]

		// Sort keys for deterministic tie-breaking
		var keys []string
		for k := range successors {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, s := range keys {
			freq := successors[s]
			if !visited[s] && freq > maxFreq {
				maxFreq = freq
				next = s
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
		// Topological-ish sort for orphans
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

func findQueuingColumns(persistence []StatusPersistence) map[string]bool {
	queues := make(map[string]bool)
	lowerToOriginal := make(map[string]string)
	for _, p := range persistence {
		lowerToOriginal[strings.ToLower(p.StatusName)] = p.StatusName
	}

	patterns := []string{"ready for ", "awaiting ", "waiting for ", "pending ", "to be "}

	// Helper to strip common activity suffixes for fuzzy matching
	stripSuffixes := func(s string) string {
		s = strings.TrimSpace(s)
		suffixes := []string{"ing", "ment", "ed", "ion"}
		for _, suff := range suffixes {
			if strings.HasSuffix(s, suff) {
				return s[:len(s)-len(suff)]
			}
		}
		return s
	}

	for lower, original := range lowerToOriginal {
		// 1. Explicit Prefix Patterns
		for _, pat := range patterns {
			if strings.HasPrefix(lower, pat) {
				action := strings.TrimPrefix(lower, pat)
				stem := stripSuffixes(action)

				// Check if there is an active counterpart
				found := false
				for otherLower := range lowerToOriginal {
					if otherLower == lower {
						continue
					}
					otherStem := stripSuffixes(otherLower)
					if otherStem != "" && (strings.Contains(otherStem, stem) || strings.Contains(stem, otherStem)) {
						found = true
						break
					}
				}
				if found {
					queues[original] = true
				}
			}
		}

		// 2. Suffix patterns: "Developed" (done with dev, waiting for something else)
		// We avoid "ing" which usually implies active work.
		if strings.HasSuffix(lower, "ed") && !strings.HasSuffix(lower, "ing") {
			queues[original] = true
		}
	}

	return queues
}
