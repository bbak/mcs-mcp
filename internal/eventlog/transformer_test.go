package eventlog

import (
	"mcs-mcp/internal/jira"
	"testing"
)

func TestTransformIssue_DuplicateResolved(t *testing.T) {
	// Setup a DTO that has a resolution change in history AND a snapshot resolution
	dto := jira.IssueDTO{
		Key: "TEST-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string "json:\"name\""
				Subtask bool   "json:\"subtask\""
			}{Name: "Story"},
			Status: struct {
				ID             string "json:\"id\""
				Name           string "json:\"name\""
				StatusCategory struct {
					Key string "json:\"key\""
				} "json:\"statusCategory\""
			}{Name: "Done", ID: "10003"},
			Resolution: struct {
				Name string "json:\"name\""
			}{Name: "Done"},
			ResolutionDate: "2024-03-20T14:30:00.000+0000",
			Created:        "2024-03-20T10:00:00.000+0000",
			Updated:        "2024-03-20T14:30:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					Created: "2024-03-20T14:30:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "In Progress",
							From:       "3",
							ToString:   "Done",
							To:         "10003",
						},
						{
							Field:      "resolution",
							FromString: "",
							From:       "",
							ToString:   "Done",
							To:         "1",
						},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)

	resolvedCount := 0
	for _, e := range events {
		if e.EventType == Resolved {
			resolvedCount++
		}
	}

	// This is the glitch: it currently produces 2 Resolved events
	if resolvedCount != 1 {
		t.Errorf("Expected exactly 1 Resolved event, got %d", resolvedCount)
	}
}

func TestTransformIssue_ResolutionGracePeriod(t *testing.T) {
	// Case: Resolution in history is 1s away from snapshot resolutiondate
	dto := jira.IssueDTO{
		Key: "TEST-2",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string "json:\"name\""
				Subtask bool   "json:\"subtask\""
			}{Name: "Story"},
			Status: struct {
				ID             string "json:\"id\""
				Name           string "json:\"name\""
				StatusCategory struct {
					Key string "json:\"key\""
				} "json:\"statusCategory\""
			}{Name: "Done", ID: "10003"},
			Resolution: struct {
				Name string "json:\"name\""
			}{Name: "Done"},
			ResolutionDate: "2024-03-20T14:30:01.000+0000", // 1s offset
			Created:        "2024-03-20T10:00:00.000+0000",
			Updated:        "2024-03-20T14:30:01.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					Created: "2024-03-20T14:30:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "resolution",
							FromString: "",
							ToString:   "Done",
						},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)
	resolvedCount := 0
	for _, e := range events {
		if e.EventType == Resolved {
			resolvedCount++
		}
	}

	if resolvedCount != 1 {
		t.Errorf("Expected exactly 1 Resolved event with 1s offset, got %d", resolvedCount)
	}
}

func TestTransformIssue_MisconfiguredWorkflow(t *testing.T) {
	// Case: Transition to "Done" but NO resolution set (misconfigured)
	dto := jira.IssueDTO{
		Key: "TEST-3",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string "json:\"name\""
				Subtask bool   "json:\"subtask\""
			}{Name: "Story"},
			Status: struct {
				ID             string "json:\"id\""
				Name           string "json:\"name\""
				StatusCategory struct {
					Key string "json:\"key\""
				} "json:\"statusCategory\""
			}{Name: "Done", ID: "10003"},
			// Resolution is EMPTY
			Created: "2024-03-20T10:00:00.000+0000",
			Updated: "2024-03-20T14:30:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					Created: "2024-03-20T14:30:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "In Progress",
							ToString:   "Done",
						},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)
	resolvedCount := 0
	for _, e := range events {
		if e.EventType == Resolved {
			resolvedCount++
		}
	}

	// Should NOT infer Resolved from status alone (Conceptual Integrity)
	if resolvedCount != 0 {
		t.Errorf("Expected 0 Resolved events for status-only transition to Done, got %d", resolvedCount)
	}
}

func TestTransformIssue_ExplicitUnresolved(t *testing.T) {
	// Case 1: Resolution explicitly cleared in Jira history
	dto := jira.IssueDTO{
		Key: "TEST-4",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string "json:\"name\""
				Subtask bool   "json:\"subtask\""
			}{Name: "Story"},
			Status: struct {
				ID             string "json:\"id\""
				Name           string "json:\"name\""
				StatusCategory struct {
					Key string "json:\"key\""
				} "json:\"statusCategory\""
			}{Name: "In Progress", ID: "3"},
			Created: "2024-03-20T10:00:00.000+0000",
			Updated: "2024-03-20T15:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					Created: "2024-03-20T15:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "resolution",
							FromString: "Done",
							ToString:   "", // Cleared
						},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)
	unresolvedCount := 0
	for _, e := range events {
		if e.EventType == Unresolved {
			unresolvedCount++
		}
	}

	if unresolvedCount != 1 {
		t.Errorf("Expected exactly 1 Unresolved event for explicit resolution clear, got %d", unresolvedCount)
	}
}
