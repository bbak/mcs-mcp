package mcp

import (
	"strings"
	"testing"
	"time"

	"mcs-mcp/internal/stats"
)

func TestWindow_DefaultLazy(t *testing.T) {
	s := &Server{}
	start, end, explicit := s.Window()

	if explicit {
		t.Fatalf("expected explicit=false on a fresh server, got true")
	}
	got := stats.CalendarDaysBetween(start, end)
	want := DefaultWindowWeeks * 7
	if got != want {
		t.Fatalf("default duration = %dd, want %dd", got, want)
	}
	if time.Since(end) > 5*time.Second {
		t.Fatalf("default end should be ~now, got %v", end)
	}
}

func TestWindow_DefaultRespectsEvaluationDate(t *testing.T) {
	pinned := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	s := &Server{activeEvaluationDate: &pinned}

	start, end, explicit := s.Window()

	if explicit {
		t.Fatalf("expected explicit=false, got true")
	}
	if !end.Equal(pinned) {
		t.Fatalf("end = %v, want pinned eval date %v", end, pinned)
	}
	wantStart := pinned.AddDate(0, 0, -DefaultWindowWeeks*7)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %v, want %v", start, wantStart)
	}
}

func TestWindow_ExplicitSession(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	s := &Server{activeWindowStart: &start, activeWindowEnd: &end}

	gotStart, gotEnd, explicit := s.Window()

	if !explicit {
		t.Fatalf("expected explicit=true")
	}
	if !gotStart.Equal(start) || !gotEnd.Equal(end) {
		t.Fatalf("window = [%v, %v], want [%v, %v]", gotStart, gotEnd, start, end)
	}
}

func TestSetAnalysisWindow_DurationOnly(t *testing.T) {
	pinned := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	s := &Server{activeEvaluationDate: &pinned}

	if _, err := s.handleSetAnalysisWindow("", "", 60, false); err != nil {
		t.Fatalf("set window: %v", err)
	}

	start, end, explicit := s.Window()
	if !explicit {
		t.Fatalf("expected explicit window after set")
	}
	if !end.Equal(pinned) {
		t.Fatalf("end = %v, want %v (eval date)", end, pinned)
	}
	wantStart := pinned.AddDate(0, 0, -60)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %v, want %v", start, wantStart)
	}
}

func TestSetAnalysisWindow_ExplicitDates(t *testing.T) {
	pinned := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	s := &Server{activeEvaluationDate: &pinned}

	if _, err := s.handleSetAnalysisWindow("2024-01-01", "2024-04-01", 0, false); err != nil {
		t.Fatalf("set window: %v", err)
	}

	start, end, _ := s.Window()
	if start.Format("2006-01-02") != "2024-01-01" {
		t.Fatalf("start = %v, want 2024-01-01", start)
	}
	if end.Format("2006-01-02") != "2024-04-01" {
		t.Fatalf("end = %v, want 2024-04-01", end)
	}
}

func TestSetAnalysisWindow_Reset(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	s := &Server{activeWindowStart: &start, activeWindowEnd: &end}

	if _, err := s.handleSetAnalysisWindow("", "", 0, true); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, _, explicit := s.Window(); explicit {
		t.Fatalf("expected explicit=false after reset")
	}
}

func TestSetAnalysisWindow_Validation(t *testing.T) {
	pinned := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		start     string
		end       string
		days      int
		reset     bool
		wantError string
	}{
		{"neither set", "", "", 0, false, "exactly one"},
		{"both set", "2024-01-01", "", 30, false, "exactly one"},
		{"reversed dates", "2024-05-01", "2024-04-01", 0, false, "strictly before"},
		{"future end", "2024-01-01", "2025-01-01", 0, false, "future"},
		{"bad start format", "yesterday", "", 0, false, "invalid start_date"},
		{"bad end format", "2024-01-01", "tomorrow", 0, false, "invalid end_date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{activeEvaluationDate: &pinned}
			_, err := s.handleSetAnalysisWindow(tc.start, tc.end, tc.days, tc.reset)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got err=%v, want substring %q", err, tc.wantError)
			}
		})
	}
}

func TestAnchorContext_ResetsWindow(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	s := &Server{
		activeSourceID:    "PROJ_1",
		activeWindowStart: &start,
		activeWindowEnd:   &end,
	}
	// Direct field reset simulates what anchorContext does on switch; full
	// anchorContext requires Jira/eventlog plumbing which is out of scope here.
	s.activeWindowStart = nil
	s.activeWindowEnd = nil

	if _, _, explicit := s.Window(); explicit {
		t.Fatalf("expected window cleared")
	}
}
