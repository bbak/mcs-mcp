package stats

import (
	"time"
)

// StatusMetadata holds the user-confirmed semantic mapping for a status.
type StatusMetadata struct {
	Role    string `json:"role"`
	Tier    string `json:"tier"`
	Outcome string `json:"outcome,omitempty"` // delivered, abandoned_demand, abandoned_upstream, abandoned_downstream
}

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

// StatusPersistence provides historical residency analysis for a single status.
type StatusPersistence struct {
	StatusName     string  `json:"statusName"`
	Share          float64 `json:"share"`          // % of sample that visited this status
	Role           string  `json:"role,omitempty"` // Functional Role (active, queue, ignore)
	Tier           string  `json:"tier,omitempty"` // Meta-Workflow Tier (Demand, Upstream, Downstream, Finished)
	P50            float64 `json:"coin_toss"`      // P50
	P70            float64 `json:"probable"`       // P70
	P85            float64 `json:"likely"`         // P85
	P95            float64 `json:"safe_bet"`       // P95
	IQR            float64 `json:"iqr"`            // P75-P25
	Inner80        float64 `json:"inner_80"`       // P90-P10
	BlockedP50     float64 `json:"blocked_p50,omitempty"`
	BlockedP85     float64 `json:"blocked_p85,omitempty"`
	BlockedCount   int     `json:"blocked_count,omitempty"`
	Interpretation string  `json:"interpretation,omitempty"`
}

// StratifiedThroughput represents delivery volume across different work item types.
type StratifiedThroughput struct {
	Pooled []int            `json:"pooled"`
	ByType map[string][]int `json:"by_type"`       // Stratified by type
	XmR    *XmRResult       `json:"xmr,omitempty"` // Stability limits calculated against pooled throughput
}
