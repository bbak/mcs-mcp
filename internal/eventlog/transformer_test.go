package eventlog_test

import (
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"testing"
)

func TestTransformIssue_DuplicateResolved(t *testing.T) {
	// Setup a DTO that has a resolution change in history AND a snapshot resolution
	dto := jira.IssueDTO{
		Key: "TEST-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Resolution: struct {
				Name string `json:"name"`
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

	events := eventlog.TransformIssue(dto)

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
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Resolution: struct {
				Name string `json:"name"`
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

	events := eventlog.TransformIssue(dto)
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
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
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

	events := eventlog.TransformIssue(dto)
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
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
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

	events := eventlog.TransformIssue(dto)
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
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
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

	events := eventlog.TransformIssue(dto)

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
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
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
							Field:    "Key",
							ToString: "NEW-2",
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

	events := eventlog.TransformIssue(dto)

	// In Case 2, we expect:
	// 1. Created (Synthetic) @ 2024-01-01, ToStatus="To Do" (derived from T=14:00 transition)
	// 2. Change (Status) @ 2024-03-01T14:00
	if len(events) != 2 {
		t.Fatalf("Expected 2 events for Case 2, got %d", len(events))
	}

	birth := events[0]
	if birth.EventType != eventlog.Created || !birth.IsHealed {
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

func TestTransformIssue_ExternalMove_NoHeal(t *testing.T) {
	// Scenario: Issue is currently in target "OURPROJ".
	// History shows a move from "EXT1" to "EXT2".
	// This move should be IGNORED for healing purposes because it doesn't enter "OURPROJ".
	dto := jira.IssueDTO{
		Key: "OURPROJ-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Created: "2024-01-01T10:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Event 1: Move between two EXTERNAL projects
					Created: "2024-02-01T12:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "Key",
							FromString: "EXT1-1",
							ToString:   "EXT2-1",
						},
						{
							Field:      "workflow",
							FromString: "WF1",
							ToString:   "WF2",
						},
					},
				},
				{
					// Event 2: Normal transition in EXT2
					Created: "2024-02-01T13:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "To Do",
							ToString:   "In Progress",
						},
					},
				},
			},
		},
	}

	events := eventlog.TransformIssue(dto)

	// Since the move was EXT1 -> EXT2, and our project is OURPROJ,
	// healing should NOT have been triggered.
	// We expect the original history to be preserved (even though we don't care about it).
	for _, e := range events {
		if e.IsHealed {
			t.Errorf("Healing should NOT have been triggered for an external move")
		}
	}
}

func TestTransformIssue_BoundaryWithLaterEvent(t *testing.T) {
	// Scenario:
	// H2 (Move): Key Change only (Boundary). Chronologically EARLIER.
	// H1 (Transition): Status: DEPLOY -> Done. Chronologically LATER.

	dto := jira.IssueDTO{
		Key: "NEW-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Created: "2024-01-01T10:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// H2: Oldest (Move @ Feb 1st)
					Created: "2024-02-01T10:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:    "Key",
							To:       "NEW-1",
							ToString: "NEW-1",
						},
						{
							Field:      "workflow",
							FromString: "OLD-WF",
							ToString:   "NEW-WF",
						},
					},
				},
				{
					// H1: Latest (Transition @ Mar 1st)
					Created: "2024-03-01T10:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "DEPLOY",
							ToString:   "Done",
						},
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

	events := eventlog.TransformIssue(dto)

	// Expecting:
	// 1. Created (Synthetic) @ 2024-01-01 (ToStatus: DEPLOY)
	// 2. Change (DEPLOY -> Done) @ 2024-03-01
	if len(events) != 2 {
		// If bug exists, user says we get ONLY ONE event (the Created one).
		t.Fatalf("Expected 2 events, got %d. Events: %+v", len(events), events)
	}

	if events[0].EventType != eventlog.Created || events[0].ToStatus != "DEPLOY" {
		t.Errorf("First event should be Created(DEPLOY), got %s(%s)", events[0].EventType, events[0].ToStatus)
	}
	if events[1].EventType != eventlog.Change || events[1].ToStatus != "Done" {
		t.Errorf("Second event should be Change(Done), got %s(%s)", events[1].EventType, events[1].ToStatus)
	}
	if events[1].Resolution != "Done" {
		t.Errorf("Second event should have resolution 'Done', got '%s'", events[1].Resolution)
	}
}

func TestTransformIssue_LostTerminalEvent_UserReproduction(t *testing.T) {
	// Scenario:
	// Entry 2 (Oldest): Key change + Workflow change (Boundary). Feb 1st.
	// Entry 1 (Latest): Status Change (DEPLOY -> Done) + Resolution (Fixed). Mar 1st.
	dto := jira.IssueDTO{
		Key: "NEW-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Created: "2024-01-01T10:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// H2: Move (Oldest)
					Created: "2024-02-01T10:00:00.000+0000",
					Items: []jira.ItemDTO{
						{Field: "Key", To: "NEW-1", ToString: "NEW-1"},
						{Field: "workflow", FromString: "old", ToString: "new"},
					},
				},
				{
					// H1: Transition (Latest)
					Created: "2024-03-01T10:00:00.000+0000",
					Items: []jira.ItemDTO{
						{Field: "status", FromString: "DEPLOY", ToString: "Done"},
						{Field: "resolution", FromString: "", ToString: "Fixed"},
					},
				},
			},
		},
	}

	events := eventlog.TransformIssue(dto)

	// User says they get ONE event (the Created one).
	// We expect 2: Created + Change.
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d. Events: %+v", len(events), events)
	}

	// Double check the terminal resolution
	found := false
	for _, e := range events {
		if e.Resolution == "Fixed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Lost the terminal resolution event from history entry 1")
	}
}

func TestTransformIssue_LostTerminalEvent_SameTimestamp(t *testing.T) {
	// Scenario:
	// Histories[0] = Transition (Latest in intent)
	// Histories[1] = Move (Oldest in intent)
	// Both have same timestamp.
	// Backward loop starts at index 1 (Move) and BREAKS.
	// Transition at index 0 is LOST.
	ts := "2024-02-01T10:00:00.000+0000"
	dto := jira.IssueDTO{
		Key: "NEW-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Created: "2024-01-01T10:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Index 0: Transition
					Created: ts,
					Items: []jira.ItemDTO{
						{Field: "status", FromString: "DEPLOY", ToString: "Done"},
						{Field: "resolution", FromString: "", ToString: "Fixed"},
					},
				},
				{
					// Index 1: Move
					Created: ts,
					Items: []jira.ItemDTO{
						{Field: "Key", To: "NEW-1", ToString: "NEW-1"},
						{Field: "workflow", FromString: "old", ToString: "new"},
					},
				},
			},
		},
	}

	// NOTE: We don't sort in this test setup because we want to force the order
	// Or even if we did sort, t1.Before(t2) is false, so swap won't happen.

	events := eventlog.TransformIssue(dto)

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d. Events: %+v", len(events), events)
	}
}

func TestTransformIssue_GlitchReproduction(t *testing.T) {
	// Case 2 from docs/ideas.md:
	// Simultaneous Key/Workflow change (boundary) and Status change.
	// We want to ensure that ONLY the Created event reflects this status,
	// and NO duplicate Change event is emitted.
	// We ALSO check the user's mention of resolution in the same change-set.

	dto := jira.IssueDTO{
		Key: "GENPROJ-87",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Feature"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Done", ID: "10003"},
			Created: "2023-08-17T12:00:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Move + Status Change + Resolution (Boundary)
					Created: "2023-08-18T18:50:00.000+0000",
					Items: []jira.ItemDTO{
						{Field: "Key", FromString: "EXTPROJ-3120", ToString: "GENPROJ-87"},
						{Field: "status", FromString: "FUNCTIONAL", From: "10061", ToString: "Analysis", To: "19772"},
						{Field: "resolution", FromString: "", ToString: "Fixed", To: "1"},
					},
				},
				{
					// Subsequent Status Change
					Created: "2023-08-28T12:47:00.000+0000",
					Items: []jira.ItemDTO{
						{Field: "status", FromString: "Analysis", From: "19772", ToString: "Ready for Development", To: "10175"},
					},
				},
			},
		},
	}

	events := eventlog.TransformIssue(dto)

	// Expected Events:
	// 1. Created (Synthetic @ 2023-08-17) - Status: Analysis (ToString of boundary)
	// 2. Change (@ 2023-08-18) - Status suppressed (arrival status is in Created), Resolution: Fixed
	// 3. Change (@ 2023-08-28) - From: Analysis, To: Ready for Development

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d. Events: %+v", len(events), events)
	}

	created := events[0]
	if created.EventType != eventlog.Created {
		t.Errorf("First event should be Created, got %v", created.EventType)
	}
	if created.ToStatus != "Analysis" {
		t.Errorf("Created event status should be 'Analysis' (arrival), got '%s'", created.ToStatus)
	}

	boundary := events[1]
	if boundary.EventType != eventlog.Change {
		t.Errorf("Second event should be Change (Boundary), got %v", boundary.EventType)
	}
	// Status should be suppressed
	if boundary.ToStatus != "" {
		t.Errorf("Boundary change status should be suppressed, got ->%s", boundary.ToStatus)
	}
	if boundary.Resolution != "Fixed" {
		t.Errorf("Boundary change resolution should be 'Fixed', got '%s'", boundary.Resolution)
	}

	change := events[2]
	if change.EventType != eventlog.Change {
		t.Errorf("Third event should be Change, got %v", change.EventType)
	}
	if change.FromStatus != "Analysis" || change.ToStatus != "Ready for Development" {
		t.Errorf("Change event status mismatch: expected Analysis->Ready for Development, got %s->%s", change.FromStatus, change.ToStatus)
	}
}

func TestTransformIssue_FlaggedHistory(t *testing.T) {
	dto := jira.IssueDTO{
		Key: "TEST-1",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "InProgress", ID: "3"},
			Created: "2024-03-20T10:00:00.000+0000",
			Flagged: []interface{}{map[string]interface{}{"value": "Impediment"}}, // Current snapshot state: Blocked
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Flagged as Blocked
					Created: "2024-03-20T11:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "Flagged",
							FromString: "",
							ToString:   "Impediment",
						},
					},
				},
				{
					// Flagged cleared
					Created: "2024-03-20T12:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "Flagged",
							FromString: "Impediment",
							ToString:   "",
						},
					},
				},
				{
					// Flagged as Blocked again
					Created: "2024-03-20T13:00:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "Flagged",
							FromString: "",
							ToString:   "Impediment",
						},
					},
				},
			},
		},
	}

	events := eventlog.TransformIssue(dto)

	// Expected:
	// 1. Created (Flagged: "") @ 10:00
	// 2. Flagged (Blocked) @ 11:00
	// 3. Flagged ("") @ 12:00
	// 4. Flagged (Blocked) @ 13:00

	if len(events) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(events))
	}

	if events[0].EventType != eventlog.Created || events[0].Flagged != "" {
		t.Errorf("Expected Created event with empty flagged, got %v with '%s'", events[0].EventType, events[0].Flagged)
	}

	if events[1].EventType != eventlog.Flagged || events[1].Flagged != "Impediment" {
		t.Errorf("Expected first Flagged event to be Impediment, got %v with '%s'", events[1].EventType, events[1].Flagged)
	}

	if events[2].EventType != eventlog.Flagged || events[2].Flagged != "" {
		t.Errorf("Expected second Flagged event to be empty, got %v with '%s'", events[2].EventType, events[2].Flagged)
	}

	if events[3].EventType != eventlog.Flagged || events[3].Flagged != "Impediment" {
		t.Errorf("Expected third Flagged event to be Impediment, got %v with '%s'", events[3].EventType, events[3].Flagged)
	}

	// Verify Reconstruction (Sanity check of event counts and types above is sufficient for transformer)
}

func TestTransformIssue_MoveBoundaryWithFlagged(t *testing.T) {
	dto := jira.IssueDTO{
		Key: "IESFSCPL-153",
		Fields: jira.FieldsDTO{
			IssueType: struct {
				Name    string `json:"name"`
				Subtask bool   `json:"subtask"`
			}{Name: "Story"},
			Status: struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			}{Name: "Closed", ID: "6"},
			Created: "2023-02-01T22:41:00.000+0000",
		},
		Changelog: &jira.ChangelogDTO{
			Histories: []jira.HistoryDTO{
				{
					// Arrival in IESFSCPL (Move)
					Created: "2023-02-01T22:41:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "Key",
							FromString: "ITSCPL-3074",
							ToString:   "IESFSCPL-153",
						},
						{
							Field:      "Project",
							FromString: "Compliance",
							To:         "115133",
							ToString:   "IES FS Compliance",
						},
					},
				},
				{
					// Flagged set
					Created: "2023-05-26T16:16:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "Flagged",
							FromString: "",
							ToString:   "Impediment",
						},
					},
				},
				{
					// Status Change (Open -> Closed) - THIS should be the arrival status
					Created: "2023-06-15T17:51:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "status",
							FromString: "Open",
							From:       "1",
							ToString:   "Closed",
							To:         "6",
						},
						{
							Field:      "resolution",
							FromString: "",
							ToString:   "Won't Do",
						},
					},
				},
				{
					// Flagged cleared
					Created: "2023-06-19T16:07:00.000+0000",
					Items: []jira.ItemDTO{
						{
							Field:      "Flagged",
							FromString: "Impediment",
							ToString:   "",
						},
					},
				},
			},
		},
	}

	events := eventlog.TransformIssue(dto)

	// Expected 'Created' event to have ToStatus: "Open"
	var createdEvent *eventlog.IssueEvent
	for i := range events {
		if events[i].EventType == eventlog.Created {
			createdEvent = &events[i]
		}
	}

	if createdEvent == nil {
		t.Fatal("Created event not found")
	}

	if createdEvent.ToStatus != "Open" {
		t.Errorf("Expected arrival status 'Open', got '%s'", createdEvent.ToStatus)
	}
}
