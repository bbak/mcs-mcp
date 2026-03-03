package simulation

// Stratification thresholds — controls when item types are simulated independently.
const (
	// StratificationMinVolume is the minimum number of delivered items a type must have to be stratified.
	StratificationMinVolume = 15
	// StratificationMinVariance is the minimum relative P85 distance from the pooled average
	// required to justify modeling a type as a separate stream (15% divergence).
	StratificationMinVariance = 0.15
)

// Dependency detection — identifies types that share capacity and cannot be modelled independently.
const (
	// DependencyCorrelationThreshold is the Pearson correlation below which two types are considered
	// capacity-constrained (one "taxes" the other). Negative values indicate an inverse relationship.
	DependencyCorrelationThreshold = -0.6
	// CapacityTaxRate is the fraction of the taxer type's throughput subtracted from the taxed type
	// when modelling capacity clashes in stratified simulations.
	CapacityTaxRate = 0.5
)

// Fat-tail / predictability classification — based on Kanban University heuristics.
const (
	// FatTailThreshold is the P98/P50 throughput ratio above which the process is classified as "Unstable".
	FatTailThreshold = 5.6
	// HeavyTailThreshold is the P85/P50 throughput ratio above which the process is classified as "Highly Volatile".
	HeavyTailThreshold = 3.0
	// SparseFatTailSymbol is returned by CalculateFatTail when the throughput median is zero
	// (sparse process), acting as a symbolic high-risk indicator.
	SparseFatTailSymbol = 10.0
)

// Forecast safeguards — prevent infinite loops and degenerate results.
const (
	// MaxForecastDays is the maximum simulated duration (10 years). Exceeding this is treated as
	// Throughput Collapse and triggers a guardrail warning.
	MaxForecastDays = 3650
	// StallCheckInterval is the number of simulated days between stall-detection checks.
	// If no items complete within this interval, the simulation is considered collapsed.
	StallCheckInterval = 1000
	// DroppedWindowWarnThreshold is the fraction of issues that must be excluded from the analysis
	// window before a guardrail warning is emitted to the caller.
	DroppedWindowWarnThreshold = 0.5
)

// Throughput trend detection.
const (
	// ThroughputChangeThreshold is the relative throughput change (positive or negative) required
	// to report a directional trend. Expressed as a positive fraction (10% = notable change).
	ThroughputChangeThreshold = 0.10
	// ThroughputSevereDeclineThreshold is the relative decline beyond which a severe throughput
	// collapse warning is emitted (30% decline).
	ThroughputSevereDeclineThreshold = 0.30
)
