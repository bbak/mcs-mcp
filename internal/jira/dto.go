package jira

import "time"

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
		StatusCategory   struct {
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
type ChangelogDTO struct {
	Histories []HistoryDTO `json:"histories"`
}

// HistoryDTO is a single entry in the changelog.
type HistoryDTO struct {
	Created string    `json:"created"`
	Items   []ItemDTO `json:"items"`
}

// ItemDTO is a single field change within a history entry.
type ItemDTO struct {
	Field                  string `json:"field"`
	ToString               string `json:"toString"`
	UntranslatedToString   string `json:"untranslatedToString,omitempty"`
	FromString             string `json:"fromString"`
	UntranslatedFromString string `json:"untranslatedFromString,omitempty"`
	To                     string `json:"to"`   // ID
	From                   string `json:"from"` // ID
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
