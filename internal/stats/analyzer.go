package stats

import (
	"mcs-mcp/internal/jira"
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

	// 1. Identify "Demand" (the first status ever visited)
	firstStatus := ""
	if len(issues) > 0 {
		// Heuristic: The status appearing earliest in transitions is the entry point
		entryCounts := make(map[string]int)
		for _, issue := range issues {
			if len(issue.Transitions) > 0 {
				entryCounts[issue.Transitions[0].FromStatus]++
			} else {
				entryCounts[issue.Status]++
			}
		}
		maxCount := -1
		for st, count := range entryCounts {
			if count > maxCount {
				maxCount = count
				firstStatus = st
			}
		}
	}

	// 2. Map Statuses to Categories (Jira fallback)
	statusCats := make(map[string]string)
	for _, issue := range issues {
		statusCats[issue.Status] = issue.StatusCategory
	}

	// 3. Pattern-based Queuing Detection
	queueCandidates := findQueuingColumns(persistence)

	// 4. Assemble Proposal
	for _, p := range persistence {
		name := p.StatusName
		tier := "Downstream" // Default
		role := "active"

		if name == firstStatus {
			tier = "Demand"
		} else if statusCats[name] == "done" || statusCats[name] == "finished" {
			tier = "Finished"
		} else if statusCats[name] == "to-do" || statusCats[name] == "new" {
			tier = "Demand"
		}

		if queueCandidates[name] {
			role = "queue"
		}

		proposal[name] = StatusMetadata{
			Tier: tier,
			Role: role,
		}
	}

	// 5. Commitment Point Logic: Highest residency crossing
	// If the user hasn't confirmed, we look for the highest residency status
	// that isn't 'Demand' or 'Finished'.
	maxRes := -1.0
	for _, p := range persistence {
		if proposal[p.StatusName].Tier == "Downstream" {
			if p.P50 > maxRes {
				maxRes = p.P50
			}
		}
	}

	// If a high-residency status is immediately preceded by a queue candidate,
	// ensure that queue candidate is in the proposal.
	// (This logic is further refined in the handler which has the full order)

	return proposal
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
