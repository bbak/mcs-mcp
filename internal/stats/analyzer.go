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
func ProposeSemantics(issues []jira.Issue, persistence []StatusPersistence) map[string]StatusMetadata {
	proposal := make(map[string]StatusMetadata)
	if len(persistence) == 0 {
		return proposal
	}

	// Identify first-ever entry points
	entryCounts := make(map[string]int)
	for _, issue := range issues {
		if len(issue.Transitions) > 0 {
			entryCounts[issue.Transitions[0].FromStatus]++
		}
	}
	firstStatus := ""
	maxEntry := 0
	for s, count := range entryCounts {
		if count > maxEntry {
			maxEntry = count
			firstStatus = s
		}
	}

	statusCats := make(map[string]string)
	for _, issue := range issues {
		statusCats[issue.Status] = issue.StatusCategory
	}

	queueCandidates := findQueuingColumns(persistence)

	for _, p := range persistence {
		name := p.StatusName
		tier := "Downstream" // Default
		role := "active"

		// Tier Assignment
		cat := strings.ToLower(statusCats[name])
		if name == firstStatus || cat == "to-live" || cat == "to-do" || cat == "new" {
			tier = "Demand"
		} else if cat == "done" || cat == "finished" || cat == "complete" {
			tier = "Finished"
		}

		// Role Assignment with User-Specified Constraints
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

	// Refine Commitment Point: First status in Downstream tier in discovered order
	backbone := DiscoverStatusOrder(issues)
	likelyCommitment := ""
	for _, st := range backbone {
		if m, ok := proposal[st]; ok && m.Tier == "Downstream" {
			likelyCommitment = st
			break
		}
	}

	if likelyCommitment != "" {
		// We don't return the commitment point here directly, but handlers.go uses the proposed mapping
		// to find it. We ensure the Tiers/Roles are set up so handlers can find it.
	}

	// If a high-residency status is immediately preceded by a queue candidate,
	// ensure that queue candidate is in the proposal.
	// (This logic is further refined in the handler which has the full order)

	return proposal
}

// DiscoverStatusOrder derives the Backbone workflow by analyzing transition frequencies.
// It builds a directed graph and uses frequency-based directionality to determine order.
func DiscoverStatusOrder(issues []jira.Issue) []string {
	// 1. Build transition frequency matrix
	matrix := make(map[string]map[string]int)
	allStatuses := make(map[string]bool)

	for _, issue := range issues {
		for _, t := range issue.Transitions {
			if t.FromStatus != "" {
				allStatuses[t.FromStatus] = true
			}
			if t.ToStatus != "" {
				allStatuses[t.ToStatus] = true
			}
			if t.FromStatus != "" && t.ToStatus != "" {
				if matrix[t.FromStatus] == nil {
					matrix[t.FromStatus] = make(map[string]int)
				}
				matrix[t.FromStatus][t.ToStatus]++
			}
		}
		// Also include the current status if not already there
		if issue.Status != "" {
			allStatuses[issue.Status] = true
		}
	}

	if len(allStatuses) == 0 {
		return nil
	}

	// 2. Identify the Backbone using frequency dominance
	// For any pair (A, B), if Freq(A->B) > Freq(B->A), A is a predecessor.
	// We'll build a simplified DAG by only keeping "dominant" forward edges.
	statuses := make([]string, 0, len(allStatuses))
	for s := range allStatuses {
		statuses = append(statuses, s)
	}

	// Simple topological sort based on dominance counts
	// An item is "further ahead" if it has more unique predecessors than another.
	predecessorCount := make(map[string]int)
	for i := 0; i < len(statuses); i++ {
		for j := i + 1; j < len(statuses); j++ {
			s1 := statuses[i]
			s2 := statuses[j]

			f12 := matrix[s1][s2]
			f21 := matrix[s2][s1]

			if f12 > f21 {
				predecessorCount[s2]++
			} else if f21 > f12 {
				predecessorCount[s1]++
			}
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		c1 := predecessorCount[statuses[i]]
		c2 := predecessorCount[statuses[j]]
		if c1 != c2 {
			return c1 < c2
		}
		// Deterministic fallback
		return statuses[i] < statuses[j]
	})

	return statuses
}

func findQueuingColumns(persistence []StatusPersistence) map[string]bool {
	queues := make(map[string]bool)
	lowerNames := make(map[string]string)
	for _, p := range persistence {
		lowerNames[strings.ToLower(p.StatusName)] = p.StatusName
	}

	patterns := []string{"ready for ", "awaiting ", "waiting for ", "pending ", "to be "}

	for lower, original := range lowerNames {
		for _, pat := range patterns {
			if strings.HasPrefix(lower, pat) {
				action := strings.TrimPrefix(lower, pat)
				// Check if there is an active counterpart (e.g. "Ready for Dev" -> "Development")
				found := false
				for otherLower := range lowerNames {
					if otherLower != lower && (strings.Contains(otherLower, action) || strings.HasPrefix(action, otherLower)) {
						found = true
						break
					}
				}
				if found {
					queues[original] = true
				}
			}
		}

		// Suffix patterns: "Developed" (done with dev, waiting for something else)
		if strings.HasSuffix(lower, "ed") && !strings.HasSuffix(lower, "ing") {
			queues[original] = true
		}
	}

	return queues
}
