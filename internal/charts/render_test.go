package charts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHasTemplate(t *testing.T) {
	if !HasTemplate("analyze_throughput") {
		t.Error("expected HasTemplate to return true for analyze_throughput")
	}
	if HasTemplate("import_boards") {
		t.Error("expected HasTemplate to return false for import_boards")
	}
}

func TestRenderChart_UnknownTool(t *testing.T) {
	_, err := RenderChart("unknown_tool", Payload{})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRenderChart_Throughput(t *testing.T) {
	// Minimal payload that satisfies the throughput template's data access.
	data := json.RawMessage(`{
		"@metadata": [{"label": "W1", "start_date": "2024-01-01", "end_date": "2024-01-07", "is_partial": "false"}],
		"total_throughput": [5],
		"stratified_throughput": {"Story": [5]},
		"stability": {
			"average": 5.0,
			"upper_natural_process_limit": 15.0,
			"lower_natural_process_limit": 0.0,
			"average_moving_range": 2.0,
			"moving_ranges": [],
			"signals": []
		}
	}`)
	workflow := json.RawMessage(`{
		"board_id": 123,
		"project_key": "TEST",
		"board_name": "Test Board"
	}`)

	payload := Payload{
		Data:     data,
		Workflow: workflow,
	}

	html, err := RenderChart("analyze_throughput", payload)
	if err != nil {
		t.Fatalf("RenderChart failed: %v", err)
	}

	// Verify the HTML contains expected structural elements.
	checks := []string{
		"<!DOCTYPE html>",
		"__MCS_PAYLOAD__",
		"__MCS_VENDOR__",
		`<div id="root">`,
		"board_id",
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("HTML missing expected content: %q", check)
		}
	}

	// Sanity check: HTML should be substantial (vendor.js alone is ~660KB).
	if len(html) < 100000 {
		t.Errorf("HTML seems too small: %d bytes", len(html))
	}
}
