package discovery

import (
	"math"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"slices"
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
	proposal, cp, refinedOrder, _ := ProposeSemantics(sample, persistence, resolutions)
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
