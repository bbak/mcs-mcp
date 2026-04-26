package mcp

import (
	"testing"
)

func TestSchemaForAllToolInputs(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{"AnalyzeCycleTimeInput", func() error { _, err := schemaFor[AnalyzeCycleTimeInput](); return err }},
		{"AnalyzeThroughputInput", func() error { _, err := schemaFor[AnalyzeThroughputInput](); return err }},
		{"AnalyzeProcessStabilityInput", func() error { _, err := schemaFor[AnalyzeProcessStabilityInput](); return err }},
		{"AnalyzeFlowDebtInput", func() error { _, err := schemaFor[AnalyzeFlowDebtInput](); return err }},
		{"GenerateCFDDataInput", func() error { _, err := schemaFor[GenerateCFDDataInput](); return err }},
		{"AnalyzeWIPStabilityInput", func() error { _, err := schemaFor[AnalyzeWIPStabilityInput](); return err }},
		{"AnalyzeWIPAgeStabilityInput", func() error { _, err := schemaFor[AnalyzeWIPAgeStabilityInput](); return err }},
		{"AnalyzeResidenceTimeInput", func() error { _, err := schemaFor[AnalyzeResidenceTimeInput](); return err }},
		{"AnalyzeStatusPersistenceInput", func() error { _, err := schemaFor[AnalyzeStatusPersistenceInput](); return err }},
		{"AnalyzeWorkItemAgeInput", func() error { _, err := schemaFor[AnalyzeWorkItemAgeInput](); return err }},
		{"AnalyzeProcessEvolutionInput", func() error { _, err := schemaFor[AnalyzeProcessEvolutionInput](); return err }},
		{"AnalyzeYieldInput", func() error { _, err := schemaFor[AnalyzeYieldInput](); return err }},
		{"ForecastMonteCarloInput", func() error { _, err := schemaFor[ForecastMonteCarloInput](); return err }},
		{"ForecastBacktestInput", func() error { _, err := schemaFor[ForecastBacktestInput](); return err }},
		{"SetAnalysisWindowInput", func() error { _, err := schemaFor[SetAnalysisWindowInput](); return err }},
		{"GetAnalysisWindowInput", func() error { _, err := schemaFor[GetAnalysisWindowInput](); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err != nil {
				t.Fatalf("schemaFor[%s] failed: %v", tc.name, err)
			}
		})
	}
}
