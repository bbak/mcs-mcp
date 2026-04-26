package mcp

// Default analysis window sizes used by handlers.
const (
	// DefaultForecastSampleDays is the default rolling window (in days) for forecast sampling.
	// Used by forecast_monte_carlo and the BBak engine; not the diagnostic session window.
	DefaultForecastSampleDays = 90

	// BaselineWindowWeeks is the default baseline window (in weeks) for forecast-internal stationarity checks.
	BaselineWindowWeeks = 26

	// DataProbeSampleSize is the number of issues sampled during tier-neutral data probes.
	DataProbeSampleSize = 200
)
