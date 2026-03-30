package mcp

// Default analysis window sizes used by handlers.
const (
	// DefaultPersistenceWindowDays is the historical window (in days) for status persistence analysis.
	DefaultPersistenceWindowDays = 180

	// DefaultForecastSampleDays is the default rolling window (in days) for forecast sampling.
	DefaultForecastSampleDays = 90

	// BaselineWindowWeeks is the default baseline window (in weeks) for throughput and stability analysis.
	BaselineWindowWeeks = 26

	// DataProbeSampleSize is the number of issues sampled during tier-neutral data probes.
	DataProbeSampleSize = 200
)
