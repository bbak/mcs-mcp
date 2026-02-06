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

func TestTransformIssue_MoveArrival(t *testing.T) {
	// Scenario: Item moved (Key/Project change) and then had a status change.
	// We explicitly provide histories OUT OF ORDER (descending) to verify Pass 0 sorting.
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
			Created: "2017-03-02T10:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Event 2: Status change (Next Status Event)
					Created: "2017-03-02T12:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "In Progress",
							ToString:   "Doing",
						},
					},
				},
				{
					// Event 1: Move (Context Entry)
					Created: "2017-03-02T11:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "project",
							FromString: "OLD",
							ToString:   "NEW",
						},
					},
				},
			},
		},
	}

	events := TransformIssue(dto)

	// We expect:
	// 1. Created Event (Healed) @ T=10:00:00, ToStatus="In Progress" (derived from T=12:00:00 fromStatus)
	// 2. Change Event (Move) @ T=11:00:00
	// 3. Change Event (Status) @ T=12:00:00

	if len(events) < 3 {
		t.Fatalf("Expected at least 3 events, got %d", len(events))
	}

	created := events[0]
	if created.EventType != Created {
		t.Errorf("First event should be Created, got %s", created.EventType)
	}
	if !created.IsHealed {
		t.Errorf("Created event should be marked as IsHealed")
	}
	if created.ToStatus != "In Progress" {
		t.Errorf("Expected Healed arrival status 'In Progress', got '%s'", created.ToStatus)
	}
}
