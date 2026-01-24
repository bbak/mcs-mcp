package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type dcClient struct {
	cfg         Config
	httpClient  *http.Client
	lastRequest time.Time
}

func NewDataCenterClient(cfg Config) Client {
	if cfg.RequestDelay == 0 {
		cfg.RequestDelay = 10 * time.Second
	}
	return &dcClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (c *dcClient) throttle() {
	elapsed := time.Since(c.lastRequest)
	if elapsed < c.cfg.RequestDelay {
		wait := c.cfg.RequestDelay - elapsed
		log.Debug().Dur("wait", wait).Msg("Throttling Jira request")
		time.Sleep(wait)
	}
	c.lastRequest = time.Now()
}

func (c *dcClient) addCookies(req *http.Request) {
	cookies := []struct {
		name  string
		value string
	}{
		{"atlassian.xsrf.token", c.cfg.XsrfToken},
		{"JSESSIONID", c.cfg.SessionID},
		{"seraph.rememberme.cookie", c.cfg.RememberMe},
		{"GCILB", c.cfg.GCILB},
		{"GCLB", c.cfg.GCLB},
	}

	var cookiePairs []string
	for _, cookie := range cookies {
		if cookie.value != "" {
			// We build the string manually to avoid net/http's strict RFC 6265 validation
			// which would drop valid Jira/GCLB cookies containing double quotes.
			cookiePairs = append(cookiePairs, fmt.Sprintf("%s=%s", cookie.name, cookie.value))
		}
	}

	if len(cookiePairs) > 0 {
		req.Header.Set("Cookie", strings.Join(cookiePairs, "; "))
	}
}

func (c *dcClient) SearchIssues(jql string, startAt int, maxResults int) ([]Issue, int, error) {
	return c.searchInternal(jql, startAt, maxResults, "")
}

func (c *dcClient) SearchIssuesWithHistory(jql string, startAt int, maxResults int) ([]Issue, int, error) {
	return c.searchInternal(jql, startAt, maxResults, "changelog")
}

func (c *dcClient) searchInternal(jql string, startAt int, maxResults int, expand string) ([]Issue, int, error) {
	c.throttle()

	// Use url.Values for better query param handling
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("startAt", fmt.Sprintf("%d", startAt))
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	params.Set("fields", "issuetype,status,resolution,resolutiondate,created")
	if expand != "" {
		params.Set("expand", expand)
	}

	searchURL := fmt.Sprintf("%s/rest/api/2/search?%s", c.cfg.BaseURL, params.Encode())
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, 0, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var result struct {
		Total  int `json:"total"`
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				IssueType struct {
					Name string `json:"name"`
				} `json:"issuetype"`
				Status struct {
					Name string `json:"name"`
				} `json:"status"`
				Resolution struct {
					Name string `json:"name"`
				} `json:"resolution"`
				ResolutionDate string `json:"resolutiondate"`
				Created        string `json:"created"`
			} `json:"fields"`
			Changelog *struct {
				Histories []struct {
					Created string `json:"created"`
					Items   []struct {
						Field    string `json:"field"`
						ToString string `json:"toString"`
					} `json:"items"`
				} `json:"histories"`
			} `json:"changelog"`
		} `json:"issues"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	issues := make([]Issue, len(result.Issues))
	for i, item := range result.Issues {
		issues[i] = Issue{
			Key:        item.Key,
			IssueType:  item.Fields.IssueType.Name,
			Status:     item.Fields.Status.Name,
			Resolution: item.Fields.Resolution.Name,
		}
		if t, err := time.Parse("2006-01-02T15:04:05.000-0700", item.Fields.Created); err == nil {
			issues[i].Created = t
		}
		if item.Fields.ResolutionDate != "" {
			if t, err := time.Parse("2006-01-02T15:04:05.000-0700", item.Fields.ResolutionDate); err == nil {
				issues[i].ResolutionDate = &t
			}
		}

		// Parse changelog for Transitions and StartedDate
		if item.Changelog != nil {
			var earliest *time.Time
			for _, h := range item.Changelog.Histories {
				for _, itm := range h.Items {
					if itm.Field == "status" {
						if t, err := time.Parse("2006-01-02T15:04:05.000-0700", h.Created); err == nil {
							issues[i].Transitions = append(issues[i].Transitions, StatusTransition{
								ToStatus: itm.ToString,
								Date:     t,
							})
							if earliest == nil || t.Before(*earliest) {
								st := t
								earliest = &st
							}
						}
					}
				}
			}
			issues[i].StartedDate = earliest
		}
	}

	return issues, result.Total, nil
}

func (c *dcClient) GetProject(key string) (interface{}, error) {
	c.throttle()

	url := fmt.Sprintf("%s/rest/api/2/project/%s", c.cfg.BaseURL, key)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("project %s not found", key)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var project map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, err
	}

	return project, nil
}

func (c *dcClient) GetProjectStatuses(key string) (interface{}, error) {
	c.throttle()

	url := fmt.Sprintf("%s/rest/api/2/project/%s/statuses", c.cfg.BaseURL, key)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("project %s statuses not found", key)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var statuses []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, err
	}

	return statuses, nil
}

func (c *dcClient) GetBoard(id int) (interface{}, error) {
	c.throttle()

	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d", c.cfg.BaseURL, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("board %d not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var board map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		return nil, err
	}

	return board, nil
}
func (c *dcClient) FindProjects(query string) ([]interface{}, error) {
	c.throttle()

	// Jira API for fetching projects
	url := fmt.Sprintf("%s/rest/api/2/project", c.cfg.BaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var projects []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}

	// Filter projects by query (case-insensitive)
	var filtered []interface{}
	q := strings.ToLower(query)
	for _, p := range projects {
		proj := p.(map[string]interface{})
		name := strings.ToLower(proj["name"].(string))
		key := strings.ToLower(proj["key"].(string))
		if strings.Contains(name, q) || strings.Contains(key, q) {
			filtered = append(filtered, proj)
		}
		if len(filtered) >= 20 { // Cap results
			break
		}
	}

	return filtered, nil
}

func (c *dcClient) FindBoards(projectKey string, nameFilter string) ([]interface{}, error) {
	c.throttle()

	url := fmt.Sprintf("%s/rest/agile/1.0/board", c.cfg.BaseURL)
	if projectKey != "" {
		url = fmt.Sprintf("%s?projectKeyOrId=%s", url, projectKey)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var result struct {
		Values []interface{} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	filtered := result.Values
	if nameFilter != "" {
		var f []interface{}
		nf := strings.ToLower(nameFilter)
		for _, b := range result.Values {
			board := b.(map[string]interface{})
			name := strings.ToLower(board["name"].(string))
			if strings.Contains(name, nf) {
				f = append(f, board)
			}
		}
		filtered = f
	}

	if len(filtered) > 20 {
		filtered = filtered[:20]
	}

	return filtered, nil
}
func (c *dcClient) GetBoardConfig(id int) (interface{}, error) {
	c.throttle()

	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d/configuration", c.cfg.BaseURL, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("board configuration %d not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var config map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *dcClient) GetFilter(id string) (interface{}, error) {
	c.throttle()

	url := fmt.Sprintf("%s/rest/api/2/filter/%s", c.cfg.BaseURL, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.addCookies(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("filter %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned status %d", resp.StatusCode)
	}

	var filter map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&filter); err != nil {
		return nil, err
	}

	return filter, nil
}
