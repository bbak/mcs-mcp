package jira

import (
	"encoding/json"
	"time"
)

// SearchResponse is the top-level container for Jira search results.
type SearchResponse struct {
	Total  int        `json:"total"`
	Issues []IssueDTO `json:"issues"`
}

// IssueDTO represents a single issue in the Jira search response.
type IssueDTO struct {
	Key       string        `json:"key"`
	Fields    FieldsDTO     `json:"fields"`
	Changelog *ChangelogDTO `json:"changelog,omitempty"`
}

// FieldsDTO contains the specific fields we care about.
type FieldsDTO struct {
	IssueType struct {
		Name             string `json:"name"`
		UntranslatedName string `json:"untranslatedName,omitempty"`
		Subtask          bool   `json:"subtask"`
	} `json:"issuetype"`
	Status struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		UntranslatedName string `json:"untranslatedName,omitempty"`

		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"status"`
	Resolution struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		UntranslatedName string `json:"untranslatedName,omitempty"`
	} `json:"resolution"`
	ResolutionDate string `json:"resolutiondate"`
	Flagged        any    `json:"customfield_10014,omitempty"` // Standard Flagged field ID or common alias
	Created        string `json:"created"`
	Updated        string `json:"updated"`
}

// ChangelogDTO contains historical transitions.
// StartAt, MaxResults, and Total are populated from the Jira pagination envelope
// and are used solely for truncation detection; they are not passed downstream.
type ChangelogDTO struct {
	StartAt    int          `json:"startAt"`
	MaxResults int          `json:"maxResults"`
	Total      int          `json:"total"`
	Histories  []HistoryDTO `json:"histories"`
}

// UnmarshalJSON handles both the embedded-changelog format (key: "histories") used by
// the search and single-issue endpoints, and the dedicated changelog endpoint format
// (key: "values") used by the Jira Cloud standalone changelog API.
func (c *ChangelogDTO) UnmarshalJSON(data []byte) error {
	type alias struct {
		StartAt    int          `json:"startAt"`
		MaxResults int          `json:"maxResults"`
		Total      int          `json:"total"`
		Histories  []HistoryDTO `json:"histories"`
		Values     []HistoryDTO `json:"values"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	c.StartAt = a.StartAt
	c.MaxResults = a.MaxResults
	c.Total = a.Total
	if len(a.Histories) > 0 {
		c.Histories = a.Histories
	} else {
		c.Histories = a.Values
	}
	return nil
}

// HistoryDTO is a single entry in the changelog.
type HistoryDTO struct {
	Created string    `json:"created"`
	Items   []ItemDTO `json:"items"`
}

// ItemDTO is a single field change within a history entry.
type ItemDTO struct {
	Field      string `json:"field"`
	FromString string `json:"fromString"`
	ToString   string `json:"toString"`
	To         string `json:"to"`   // ID
	From       string `json:"from"` // ID
}

// FindBoardsResponse is used for the board search API.
type FindBoardsResponse struct {
	Values []any `json:"values"`
}

// ProjectStatusDTO represents the nested status structure in Jira Cloud.
type ProjectStatusDTO struct {
	Statuses []Status `json:"statuses"`
}

// ResolutionDTO represents a resolution metadata object.
type ResolutionDTO struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	UntranslatedName string `json:"untranslatedName,omitempty"`
}

// Status is an embedded status object (shared by FieldsDTO and ProjectStatusDTO).
type Status struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	UntranslatedName string `json:"untranslatedName,omitempty"`
}

// ParseTime is a helper for the strict Jira time format.
func ParseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05.000-0700", s)
}

// ExtractProjectKey extracts the project key portion from a Jira issue key (e.g., "PROJ" from "PROJ-123").
func ExtractProjectKey(issueKey string) string {
	for i := 0; i < len(issueKey); i++ {
		if issueKey[i] == '-' {
			return issueKey[:i]
		}
	}
	return issueKey
}
