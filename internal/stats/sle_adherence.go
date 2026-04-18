package stats

import (
	"mcs-mcp/internal/jira"
	"slices"
	"time"
)

// AdherenceBucket holds per-period SLE attainment + tail-severity stats.
type AdherenceBucket struct {
	BucketStart      time.Time `json:"bucket_start"`
	BucketLabel      string    `json:"bucket_label"`
	DeliveredCount   int       `json:"delivered_count"`
	BreachCount      int       `json:"breach_count"`
	AttainmentRate   float64   `json:"attainment_rate"`
	MaxCycleTimeDays float64   `json:"max_cycle_time_days"`
	P95BreachMagDays float64   `json:"p95_breach_magnitude_days"`
	IsPartial        bool      `json:"is_partial,omitempty"`
}

// SLEAdherenceResult holds the trended adherence series + summary metrics.
type SLEAdherenceResult struct {
	SLEDurationDays float64           `json:"sle_duration_days"`
	SLEPercentile   int               `json:"sle_percentile"`
	SLESource       string            `json:"sle_source"`
	ExpectedRate    float64           `json:"expected_attainment_rate"`
	OverallRate     float64           `json:"overall_attainment_rate"`
	FatTailRatio    float64           `json:"fat_tail_ratio_p95_p85"`
	Buckets         []AdherenceBucket `json:"buckets"`
}

// ComputeSLEAdherence builds a per-bucket attainment + breach-magnitude series.
//
// issues and cycleTimes must be aligned by index (as returned by Server.getCycleTimes).
// sleDays is the breach threshold; items with cycleTime > sleDays are breaches.
// slePercentile is informational (used to derive ExpectedRate when > 0).
// sleSource is one of "user", "derived_p85", "derived_pXX".
// window provides the bucket boundaries via Subdivide().
func ComputeSLEAdherence(
	issues []jira.Issue,
	cycleTimes []float64,
	sleDays float64,
	slePercentile int,
	sleSource string,
	window AnalysisWindow,
) SLEAdherenceResult {
	res := SLEAdherenceResult{
		SLEDurationDays: Round2(sleDays),
		SLEPercentile:   slePercentile,
		SLESource:       sleSource,
	}
	if slePercentile > 0 && slePercentile < 100 {
		// A P85 SLE expects 85% of items inside the threshold by definition.
		res.ExpectedRate = Round2(float64(slePercentile) / 100.0)
	}

	if len(issues) == 0 || len(cycleTimes) == 0 || sleDays <= 0 {
		return res
	}

	bucketStarts := window.Subdivide()
	if len(bucketStarts) == 0 {
		return res
	}

	// Aggregate per-bucket counts + breach magnitudes.
	type acc struct {
		delivered     int
		breaches      int
		maxCT         float64
		breachExcess  []float64 // (cycleTime - sleDays) per breach
		representDate time.Time
	}
	buckets := make([]acc, len(bucketStarts))

	totalDelivered := 0
	totalBreaches := 0

	for i, iss := range issues {
		if i >= len(cycleTimes) {
			break
		}
		ct := cycleTimes[i]
		dt := iss.OutcomeDate
		if dt == nil {
			dt = iss.ResolutionDate
		}
		if dt == nil {
			continue
		}
		idx := window.FindBucketIndex(*dt)
		if idx < 0 || idx >= len(buckets) {
			continue
		}
		b := &buckets[idx]
		b.delivered++
		totalDelivered++
		if ct > b.maxCT {
			b.maxCT = ct
		}
		if ct > sleDays {
			b.breaches++
			totalBreaches++
			b.breachExcess = append(b.breachExcess, ct-sleDays)
		}
		if b.representDate.IsZero() {
			b.representDate = bucketStarts[idx]
		}
	}

	out := make([]AdherenceBucket, 0, len(bucketStarts))
	allCT := append([]float64(nil), cycleTimes...)
	slices.Sort(allCT)
	p85 := PercentileOfSorted(allCT, 0.85)
	p95 := PercentileOfSorted(allCT, 0.95)

	for i, start := range bucketStarts {
		b := buckets[i]
		bucket := AdherenceBucket{
			BucketStart:      start,
			BucketLabel:      window.GenerateLabel(start),
			DeliveredCount:   b.delivered,
			BreachCount:      b.breaches,
			MaxCycleTimeDays: Round2(b.maxCT),
			IsPartial:        window.IsPartial(start),
		}
		if b.delivered > 0 {
			bucket.AttainmentRate = Round2(1.0 - float64(b.breaches)/float64(b.delivered))
		}
		if len(b.breachExcess) > 0 {
			slices.Sort(b.breachExcess)
			bucket.P95BreachMagDays = Round2(PercentileOfSorted(b.breachExcess, 0.95))
		}
		out = append(out, bucket)
	}

	res.Buckets = out
	if totalDelivered > 0 {
		res.OverallRate = Round2(1.0 - float64(totalBreaches)/float64(totalDelivered))
	}
	if p85 > 0 {
		res.FatTailRatio = Round2(p95 / p85)
	}

	return res
}
