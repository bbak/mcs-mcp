package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"sort"
	"time"
)

// MetadataSummary provides a high-level overview of a Jira data source.
type MetadataSummary struct {
	TotalIssues            int            `json:"totalIssues"`
	SampleSize             int            `json:"sampleSize"`
	IssueTypes             map[string]int `json:"issueTypes"`
	Statuses               map[string]int `json:"statuses"`
	ResolutionNames        map[string]int `json:"resolutionNames"`
	ResolutionRate         float64        `json:"resolutionRate"`
	FirstResolution        *time.Time     `json:"firstResolution,omitempty"`
	LastResolution         *time.Time     `json:"lastResolution,omitempty"`
	AverageCycleTime       float64        `json:"averageCycleTime,omitempty"` // Days
	AvailableStatuses      interface{}    `json:"availableStatuses,omitempty"`
	HistoricalReachability map[string]int `json:"historicalReachability,omitempty"` // How many issues visited each status
	CommitmentPointHints   []string       `json:"commitmentPointHints,omitempty"`
	BacklogSize            int            `json:"backlogSize,omitempty"`
}

// StatusPersistence provides historical residency analysis for a single status.
type StatusPersistence struct {
	StatusName     string  `json:"statusName"`
	Count          int     `json:"count"`
	Category       string  `json:"category,omitempty"` // Jira Category (To Do, In Progress, Done)
	Role           string  `json:"role,omitempty"`     // Functional Role (active, queue, ignore)
	Tier           string  `json:"tier,omitempty"`     // Meta-Workflow Tier (Demand, Upstream, Downstream, Finished)
	P50            float64 `json:"coin_toss"`          // P50
	P70            float64 `json:"probable"`           // P70
	P85            float64 `json:"likely"`             // P85
	P95            float64 `json:"safe_bet"`           // P95
	Interpretation string  `json:"interpretation,omitempty"`
}

// ProcessYield represents the delivery efficiency across tiers.
type ProcessYield struct {
	TotalIngested    int                  `json:"totalIngested"`
	DeliveredCount   int                  `json:"deliveredCount"`
	AbandonedCount   int                  `json:"abandonedCount"`
	OverallYieldRate float64              `json:"overallYieldRate"`
	LossPoints       []AbandonmentInsight `json:"lossPoints"`
}

// AbandonmentInsight quantifies waste at a specific stage.
type AbandonmentInsight struct {
	Tier       string  `json:"tier"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"` // % of total items abandoned at this tier
	AvgAge     float64 `json:"avgAge"`     // Avg residency in that tier before abandonment
	Severity   string  `json:"severity"`   // Indicator of waste (High for Downstream)
}

// WIPAgeAnalysis represents the risk of a single active item.
type WIPAgeAnalysis struct {
	Key          string  `json:"key"`
	Type         string  `json:"type"`
	Summary      string  `json:"summary"`
	Status       string  `json:"status"`
	DaysInStatus float64 `json:"daysInStatus"`
	Percentile   int     `json:"percentile"` // e.g., 85 if it's at the P85 level
	IsStale      bool    `json:"isStale"`    // true if DaysInStatus > P85
}

// DeliveryCadence represents a weekly snapshot of throughput.
type DeliveryCadence struct {
	WeekStarting   time.Time `json:"weekStarting"`
	ItemsDelivered int       `json:"itemsDelivered"`
}

// CalculateStatusPersistence analyzes how long items spend in each status.
func CalculateStatusPersistence(issues []jira.Issue) []StatusPersistence {
	statusDurations := make(map[string][]float64)

	for _, issue := range issues {
		for status, duration := range issue.StatusResidency {
			if duration > 0 {
				statusDurations[status] = append(statusDurations[status], duration)
			}
		}
	}

	var results []StatusPersistence
	for status, durations := range statusDurations {
		if len(durations) == 0 {
			continue
		}
		sort.Float64s(durations)
		n := len(durations)
		results = append(results, StatusPersistence{
			StatusName: status,
			Count:      n,
			P50:        math.Round(durations[int(float64(n)*0.50)]*10) / 10,
			P70:        math.Round(durations[int(float64(n)*0.70)]*10) / 10,
			P85:        math.Round(durations[int(float64(n)*0.85)]*10) / 10,
			P95:        math.Round(durations[int(float64(n)*0.95)]*10) / 10,
		})
	}

	// Sort results by status name for stability
	sort.Slice(results, func(i, j int) bool {
		return results[i].StatusName < results[j].StatusName
	})

	return results
}

// SumRangeDuration calculates the total time spent in a list of statuses for a given issue.
func SumRangeDuration(issue jira.Issue, rangeStatuses []string) float64 {
	var total float64
	for _, status := range rangeStatuses {
		if d, ok := issue.StatusResidency[status]; ok {
			total += d
		}
	}
	return total
}

// EnrichStatusPersistence adds semantic context to the persistence results.
func EnrichStatusPersistence(results []StatusPersistence, categories map[string]string, mappings map[string]StatusMetadata) []StatusPersistence {
	for i := range results {
		s := &results[i]
		if cat, ok := categories[s.StatusName]; ok {
			s.Category = cat
		}

		if m, ok := mappings[s.StatusName]; ok {
			s.Role = m.Role
			s.Tier = m.Tier
		} else {
			// Inferred defaults
			switch s.Category {
			case "to-do", "new":
				s.Tier = "Demand"
				s.Role = "active"
			case "indeterminate":
				s.Tier = "Downstream" // Conservative default
				s.Role = "active"
			case "done":
				s.Tier = "Finished"
				s.Role = "active"
			}
		}

		// Interpretation Hint
		switch s.Role {
		case "queue":
			s.Interpretation = "This is a queue/waiting stage. Persistence here is 'Flow Debt'."
		case "active":
			if s.Tier == "Demand" {
				s.Interpretation = "This is item storage; high persistence is expected."
			} else {
				s.Interpretation = "This is a working stage. High persistence indicates a bottleneck."
			}
		case "ignore":
			s.Interpretation = "This status is ignored in most process diagnostics."
		}
	}
	return results
}

// StatusMetadata holds the user-confirmed semantic mapping for a status.
type StatusMetadata struct {
	Role string `json:"role"`
	Tier string `json:"tier"`
}

// CalculateProcessYield analyzes abandonment points across tiers.
func CalculateProcessYield(issues []jira.Issue, mappings map[string]StatusMetadata, resolutions map[string]string) ProcessYield {
	yield := ProcessYield{
		TotalIngested: len(issues),
	}

	lossMap := make(map[string][]float64) // Tier -> Ages before abandonment

	for _, issue := range issues {
		isAbandoned := resolutions[issue.Resolution] == "abandoned"
		if isAbandoned {
			yield.AbandonedCount++
			// Determine which tier it was abandoned from
			// It's the Tier of the status BEFORE it reached Finished
			lastTier := "Demand"
			if len(issue.Transitions) > 0 {
				lastStatus := issue.Transitions[len(issue.Transitions)-1].ToStatus
				if m, ok := mappings[lastStatus]; ok {
					lastTier = m.Tier
				}
			}

			// Total age in the process
			age := 0.0
			for _, d := range issue.StatusResidency {
				age += d
			}
			lossMap[lastTier] = append(lossMap[lastTier], age)
		} else if issue.ResolutionDate != nil {
			yield.DeliveredCount++
		}
	}

	if yield.TotalIngested > 0 {
		yield.OverallYieldRate = float64(yield.DeliveredCount) / float64(yield.TotalIngested)
	}

	for _, tier := range []string{"Demand", "Upstream", "Downstream"} {
		ages := lossMap[tier]
		if len(ages) == 0 {
			continue
		}

		sum := 0.0
		for _, a := range ages {
			sum += a
		}

		severity := "Low"
		if tier == "Downstream" {
			severity = "High"
		} else if tier == "Upstream" {
			severity = "Medium"
		}

		yield.LossPoints = append(yield.LossPoints, AbandonmentInsight{
			Tier:       tier,
			Count:      len(ages),
			Percentage: float64(len(ages)) / float64(yield.TotalIngested),
			AvgAge:     math.Round((sum/float64(len(ages)))*10) / 10,
			Severity:   severity,
		})
	}

	return yield
}

// CalculateDeliveryCadence aggregates items resolved per week.
func CalculateDeliveryCadence(issues []jira.Issue, windowWeeks int) []DeliveryCadence {
	weeks := make(map[time.Time]int)

	// Start date for the window
	cutoff := time.Now().AddDate(0, 0, -windowWeeks*7)

	// Normalize time to the start of the week (Monday)
	normalize := func(t time.Time) time.Time {
		// Go's Weekday starts at Sunday=0. We want Monday to be the anchor.
		offset := int(t.Weekday()) - 1
		if offset < 0 {
			offset = 6 // Sunday
		}
		return time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, t.Location())
	}

	for _, issue := range issues {
		if issue.ResolutionDate != nil && issue.ResolutionDate.After(cutoff) {
			week := normalize(*issue.ResolutionDate)
			weeks[week]++
		}
	}

	var results []DeliveryCadence
	for week, count := range weeks {
		results = append(results, DeliveryCadence{
			WeekStarting:   week,
			ItemsDelivered: count,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].WeekStarting.Before(results[j].WeekStarting)
	})

	return results
}

// CalculateWIPAging identifies active items and compares their age to historical status persistence.
func CalculateWIPAging(wipIssues []jira.Issue, persistence []StatusPersistence) []WIPAgeAnalysis {
	var results []WIPAgeAnalysis

	// Build a map for quick lookup of historical percentiles
	pMap := make(map[string]StatusPersistence)
	for _, p := range persistence {
		pMap[p.StatusName] = p
	}

	for _, issue := range wipIssues {
		// Calculate time in current status
		clockStart := issue.Created
		if len(issue.Transitions) > 0 {
			clockStart = issue.Transitions[len(issue.Transitions)-1].Date
		}

		daysInStatus := time.Since(clockStart).Hours() / 24.0

		analysis := WIPAgeAnalysis{
			Key:          issue.Key,
			Type:         issue.IssueType,
			Summary:      issue.Summary,
			Status:       issue.Status,
			DaysInStatus: math.Round(daysInStatus*10) / 10,
		}

		// Calculate percentile relative to history
		if p, ok := pMap[issue.Status]; ok {
			if daysInStatus > p.P95 {
				analysis.Percentile = 95
				analysis.IsStale = true
			} else if daysInStatus > p.P85 {
				analysis.Percentile = 85
				analysis.IsStale = true
			} else if daysInStatus > p.P70 {
				analysis.Percentile = 70
			} else if daysInStatus > p.P50 {
				analysis.Percentile = 50
			} else {
				analysis.Percentile = 10
			}
		}

		results = append(results, analysis)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].DaysInStatus > results[j].DaysInStatus
	})

	return results
}

// AnalyzeProbe performs a preliminary analysis on a sample of issues.
func AnalyzeProbe(issues []jira.Issue, totalCount int) MetadataSummary {
	summary := MetadataSummary{
		TotalIssues:            totalCount,
		SampleSize:             len(issues),
		IssueTypes:             make(map[string]int),
		Statuses:               make(map[string]int),
		ResolutionNames:        make(map[string]int),
		HistoricalReachability: make(map[string]int),
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
			if first == nil || issue.ResolutionDate.Before(*first) {
				first = issue.ResolutionDate
			}
			if last == nil || issue.ResolutionDate.After(*last) {
				last = issue.ResolutionDate
			}
		}
	}

	summary.ResolutionRate = float64(resolvedCount) / float64(len(issues))
	summary.FirstResolution = first
	summary.LastResolution = last

	return summary
}
