package mcp

import "testing"

func TestNormalizeProjectKey(t *testing.T) {
	valid := map[string]string{
		"PROJ":    "PROJ",
		"proj":    "PROJ",
		"MCSTEST": "MCSTEST",
		"AB_12":   "AB_12",
		"":        "",
	}
	for in, want := range valid {
		got, err := normalizeProjectKey(in)
		if err != nil || got != want {
			t.Errorf("normalizeProjectKey(%q) = %q, %v; want %q, nil", in, got, err, want)
		}
	}
	for _, in := range []string{"../../proj/notes", `..\x`, "PROJ-1", `a"b`, `C:\x`, "P", "1PROJ", "PROJ OR 1=1"} {
		if got, err := normalizeProjectKey(in); err == nil {
			t.Errorf("normalizeProjectKey(%q) = %q, nil; want error", in, got)
		}
	}
}

func TestNormalizeIssueKey(t *testing.T) {
	valid := map[string]string{
		"PROJ-12": "PROJ-12",
		"proj-12": "PROJ-12",
		"":        "",
	}
	for in, want := range valid {
		got, err := normalizeIssueKey(in)
		if err != nil || got != want {
			t.Errorf("normalizeIssueKey(%q) = %q, %v; want %q, nil", in, got, err, want)
		}
	}
	for _, in := range []string{"X OR 1=1", "PROJ-12 OR key = X-1", "PROJ", "PROJ-", "../PROJ-1"} {
		if got, err := normalizeIssueKey(in); err == nil {
			t.Errorf("normalizeIssueKey(%q) = %q, nil; want error", in, got)
		}
	}
}

func TestNormalizeKeys_RewritesToolInput(t *testing.T) {
	in := AnalyzeItemJourneyInput{ProjectKey: "proj", BoardID: 7, IssueKey: "proj-1"}
	if err := normalizeKeys(&in); err != nil {
		t.Fatalf("normalizeKeys: %v", err)
	}
	if in.ProjectKey != "PROJ" || in.IssueKey != "PROJ-1" || in.BoardID != 7 {
		t.Errorf("got %+v", in)
	}

	bad := ImportProjectContextInput{ProjectKey: "../../proj/notes"}
	if err := normalizeKeys(&bad); err == nil {
		t.Error("normalizeKeys accepted a traversal project_key")
	}

	// Inputs without key fields pass through untouched.
	other := ImportProjectsInput{Query: "../anything"}
	if err := normalizeKeys(&other); err != nil || other.Query != "../anything" {
		t.Errorf("normalizeKeys changed an unrelated field: %+v, %v", other, err)
	}
}
