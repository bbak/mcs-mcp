package stats

import (
	"mcs-mcp/internal/jira"
	"reflect"
	"testing"
	"time"
)

func TestCalculateInventoryAge_SideEffectProtection(t *testing.T) {
	// 1. Setup mock data
	issues := []jira.Issue{
		{Key: "WIP-1", Status: "Doing"},
	}

	// Input cycle times: explicitly NOT sorted
	originalPersistence := []float64{50.0, 10.0, 30.0, 5.0, 100.0}
	persistenceCopy := make([]float64, len(originalPersistence))
	copy(persistenceCopy, originalPersistence)

	mappings := map[string]StatusMetadata{
		"Doing": {Tier: "Downstream"},
	}

	// 2. Execute calculation
	_ = CalculateInventoryAge(issues, "", nil, mappings, persistenceCopy, "wip", time.Now())

	// 3. Verify side-effect protection
	if !reflect.DeepEqual(persistenceCopy, originalPersistence) {
		t.Errorf("REGRESSION: CalculateInventoryAge mutated the input slice!\nExpected: %v\nGot:      %v", originalPersistence, persistenceCopy)
	}
}
