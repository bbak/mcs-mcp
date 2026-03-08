package discovery

import (
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"regexp"
	"strings"
)

// ProposeSemantics infers missing workflow semantics based on heuristics.
// It returns a mapping of status IDs to semantics, the recommended commitment point,
// the refined sequential order, and a proposed resolution → outcome mapping.
// The resolutions parameter (name-keyed, may be nil) is used for direct outcome lookups
// before falling back to regex matching.
func ProposeSemantics(issues []jira.Issue, persistence []stats.StatusPersistence, resolutions map[string]string) (map[string]stats.StatusMetadata, string, []string, map[string]string) {
	proposal := make(map[string]stats.StatusMetadata)
	if len(persistence) == 0 {
		return proposal, "", nil, nil
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
	abandonedKeywords := []string{"cancel", "discard", "obsolete", "reject", "decline", "won't do", "wont do", "dropped", "abort", "closed"}
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
		var tier string
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
			// Role intentionally left empty — Tier:"Finished" carries the terminal semantics.

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

	// Refine Outcomes based on actual Resolutions.
	// Direct map lookup takes precedence over regex; delivered-bias is preserved throughout.
	for _, issue := range issues {
		curr := stats.PreferID(issue.StatusID, issue.Status)
		if issue.ResolutionDate != nil && curr != "" && issue.Resolution != "" {
			if m, ok := proposal[curr]; ok && m.Tier == "Finished" {
				var outcome string
				if direct, found := resolutions[issue.Resolution]; found {
					outcome = direct
				} else if deliveredResolutionRegex.MatchString(issue.Resolution) {
					outcome = "delivered"
				} else {
					outcome = "abandoned"
				}
				// Bias towards delivered: once confirmed delivered, do not downgrade.
				if outcome == "delivered" {
					m.Outcome = "delivered"
				} else if m.Outcome != "delivered" {
					m.Outcome = outcome
				}
				proposal[curr] = m
			}
		}
	}

	// Build the proposed resolution → outcome mapping from all resolutions seen in the sample.
	proposedResolutions := make(map[string]string)
	for _, issue := range issues {
		if issue.Resolution == "" {
			continue
		}
		if _, exists := proposedResolutions[issue.Resolution]; exists {
			continue
		}
		if direct, found := resolutions[issue.Resolution]; found {
			proposedResolutions[issue.Resolution] = direct
		} else if deliveredResolutionRegex.MatchString(issue.Resolution) {
			proposedResolutions[issue.Resolution] = "delivered"
		} else {
			proposedResolutions[issue.Resolution] = "abandoned"
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

	return proposal, commitmentPoint, refinedOrder, proposedResolutions
}

func matchesAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
