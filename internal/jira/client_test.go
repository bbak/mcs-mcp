package jira

import "testing"

func TestNameRegistry_GetStatusName(t *testing.T) {
	nr := &NameRegistry{
		Statuses: map[string]string{
			"10001": "To Do",
			"10002": "In Progress",
			"10003": "Done",
		},
	}

	tests := []struct {
		id   string
		want string
	}{
		{"10001", "To Do"},
		{"10002", "In Progress"},
		{"10003", "Done"},
		{"99999", ""}, // Unknown ID
		{"", ""},      // Empty ID
	}

	for _, tt := range tests {
		if got := nr.GetStatusName(tt.id); got != tt.want {
			t.Errorf("GetStatusName(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestNameRegistry_GetStatusName_NilRegistry(t *testing.T) {
	var nr *NameRegistry
	if got := nr.GetStatusName("10001"); got != "" {
		t.Errorf("GetStatusName on nil registry = %q, want empty", got)
	}
}

func TestNameRegistry_GetResolutionName(t *testing.T) {
	nr := &NameRegistry{
		Resolutions: map[string]string{
			"1": "Fixed",
			"2": "Won't Fix",
			"3": "Duplicate",
		},
	}

	tests := []struct {
		id   string
		want string
	}{
		{"1", "Fixed"},
		{"2", "Won't Fix"},
		{"3", "Duplicate"},
		{"99", ""}, // Unknown ID
		{"", ""},   // Empty ID
	}

	for _, tt := range tests {
		if got := nr.GetResolutionName(tt.id); got != tt.want {
			t.Errorf("GetResolutionName(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestNameRegistry_GetResolutionName_NilRegistry(t *testing.T) {
	var nr *NameRegistry
	if got := nr.GetResolutionName("1"); got != "" {
		t.Errorf("GetResolutionName on nil registry = %q, want empty", got)
	}
}

func TestNameRegistry_GetStatusID(t *testing.T) {
	nr := &NameRegistry{
		Statuses: map[string]string{
			"10001": "To Do",
			"10002": "In Progress",
		},
	}

	tests := []struct {
		name string
		want string
	}{
		{"To Do", "10001"},
		{"to do", "10001"},       // Case-insensitive
		{"IN PROGRESS", "10002"}, // Case-insensitive
		{"Unknown", ""},
	}

	for _, tt := range tests {
		if got := nr.GetStatusID(tt.name); got != tt.want {
			t.Errorf("GetStatusID(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestNameRegistry_GetResolutionID(t *testing.T) {
	nr := &NameRegistry{
		Resolutions: map[string]string{
			"1": "Fixed",
			"2": "Won't Fix",
		},
	}

	tests := []struct {
		name string
		want string
	}{
		{"Fixed", "1"},
		{"fixed", "1"},     // Case-insensitive
		{"WON'T FIX", "2"}, // Case-insensitive
		{"Unknown", ""},
	}

	for _, tt := range tests {
		if got := nr.GetResolutionID(tt.name); got != tt.want {
			t.Errorf("GetResolutionID(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
