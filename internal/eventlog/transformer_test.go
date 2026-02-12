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
		if e.Resolution != "" {
			resolvedCount++
		}
	}

	// With packing, it should NOW produce exactly 1 event for the history entry,
	// and the snapshot fallback should be de-duplicated.
	if resolvedCount != 1 {
		t.Errorf("Expected exactly 1 event with resolution, got %d", resolvedCount)
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
		if e.Resolution != "" {
			resolvedCount++
		}
	}

	if resolvedCount != 1 {
		t.Errorf("Expected exactly 1 resolution signal with 1s offset, got %d", resolvedCount)
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
		if e.Resolution != "" {
			resolvedCount++
		}
	}

	// Should NOT infer Resolution from status alone
	if resolvedCount != 0 {
		t.Errorf("Expected 0 resolution signals for status-only transition to Done, got %d", resolvedCount)
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
		if e.IsUnresolved {
			unresolvedCount++
		}
	}

	if unresolvedCount != 1 {
		t.Errorf("Expected exactly 1 Unresolved signal for explicit resolution clear, got %d", unresolvedCount)
	}
}

func TestTransformIssue_Case1_Preserve(t *testing.T) {
	// Scenario: Move between projects with SAME workflow.
	// We expect all history to be preserved.
	dto := jira.IssueDTO{
		Key: "NEW-1",
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
			}{Name: "Doing", ID: "4"},
			Created: "2024-03-01T10:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Event 1: Pre-move activity
					Created: "2024-03-01T11:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "Backlog",
							ToString:   "To Do",
						},
					},
				},
				{
					// Event 2: Move (No workflow field)
					Created: "2024-03-01T12:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "project",
							FromString: "OLD",
							ToString:   "NEW",
						},
					},
				},
				{
					// Event 3: Post-move activity
					Created: "2024-03-01T13:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "To Do",
							ToString:   "Doing",
						},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)

	// In Case 1, we expect all transitions (Pass 2 doesn't skip).
	// 1. Created
	// 2. Change (Backlog -> To Do)
	// 3. Change (To Do -> Doing)
	if len(events) != 3 {
		t.Fatalf("Expected 3 events for Case 1, got %d", len(events))
	}

	if events[1].FromStatus != "Backlog" || events[1].ToStatus != "To Do" {
		t.Errorf("Expected pre-move history preserved, got %s -> %s", events[1].FromStatus, events[1].ToStatus)
	}
}

func TestTransformIssue_Case2_Heal(t *testing.T) {
	// Scenario: Move with WORKFLOW change.
	// We expect pre-move history to be dropped and birth anchored at arrival.
	dto := jira.IssueDTO{
		Key: "NEW-2",
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
			}{Name: "Doing", ID: "4"},
			Created: "2024-01-01T10:00:00.000+0000", // "Biological" birth
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Event 1: Irrelevant old history (different process)
					Created: "2024-01-01T11:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "Backlog",
							ToString:   "DRAFTING",
						},
					},
				},
				{
					// Event 2: Move with Workflow change
					Created: "2024-03-01T12:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:    "project",
							ToString: "NEW",
						},
						{
							Field:      "workflow",
							FromString: "OLD-PROCESS",
							ToString:   "NEW-PROCESS",
						},
					},
				},
				{
					// Event 3: First transition in new process
					Created: "2024-03-01T14:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "To Do",
							ToString:   "Doing",
						},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)

	// In Case 2, we expect:
	// 1. Created (Synthetic) @ 2024-01-01, ToStatus="To Do" (derived from T=14:00 transition)
	// 2. Change (Status) @ 2024-03-01T14:00
	if len(events) != 2 {
		t.Fatalf("Expected 2 events for Case 2, got %d", len(events))
	}

	birth := events[0]
	if birth.EventType != Created || !birth.IsHealed {
		t.Errorf("Expected Healed birth")
	}
	if birth.ToStatus != "To Do" {
		t.Errorf("Expected arrival status 'To Do', got '%s'", birth.ToStatus)
	}

	// Preserves biological age
	if birth.Timestamp != 1704103200000000 { // 2024-01-01 10:00:00 UTC
		t.Errorf("Expected original birth timestamp, got %d", birth.Timestamp)
	}
}
