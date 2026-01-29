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
		Name    string `json:"name"`
		Subtask bool   `json:"subtask"`
	} `json:"issuetype"`
	Status struct {
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"status"`
	Resolution struct {
		Name string `json:"name"`
	} `json:"resolution"`
	ResolutionDate string `json:"resolutiondate"`
	Created        string `json:"created"`
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
	Field      string `json:"field"`
	ToString   string `json:"toString"`
	FromString string `json:"fromString"`
}

// FindBoardsResponse is used for the board search API.
type FindBoardsResponse struct {
	Values []interface{} `json:"values"`
}

// ParseTime is a helper for the strict Jira time format.
func ParseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05.000-0700", s)
}
