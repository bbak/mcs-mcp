package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type dcClient struct {
	cfg         Config
	httpClient  *http.Client
	lastRequest time.Time

	// Session Cache
	cache      map[string]*cacheEntry
	cacheMutex sync.RWMutex
}

type cacheEntry struct {
	Value       interface{}
	Expiration  time.Time
	AccessCount int
	OriginalTTL time.Duration
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
		cache: make(map[string]*cacheEntry),
	}
}

func (c *dcClient) getFromCache(key string) (interface{}, bool) {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	entry, ok := c.cache[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.Expiration) {
		delete(c.cache, key)
		return nil, false
	}

	// Sliding window extension
	if entry.AccessCount < 6 {
		entry.Expiration = time.Now().Add(entry.OriginalTTL)
		entry.AccessCount++
		log.Trace().Str("key", key).Int("count", entry.AccessCount).Msg("Extended cache TTL")
	}

	return entry.Value, true
}

func (c *dcClient) addToCache(key string, value interface{}, ttl time.Duration) {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	c.cache[key] = &cacheEntry{
		Value:       value,
		Expiration:  time.Now().Add(ttl),
		OriginalTTL: ttl,
		AccessCount: 1,
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

func (c *dcClient) SearchIssues(jql string, startAt int, maxResults int) (*SearchResponse, error) {
	return c.searchInternal(jql, startAt, maxResults, "")
}

func (c *dcClient) SearchIssuesWithHistory(jql string, startAt int, maxResults int) (*SearchResponse, error) {
	return c.searchInternal(jql, startAt, maxResults, "changelog")
}

func (c *dcClient) searchInternal(jql string, startAt int, maxResults int, expand string) (*SearchResponse, error) {
	cacheKey := fmt.Sprintf("search:%s:%d:%d:%s", jql, startAt, maxResults, expand)
	if val, ok := c.getFromCache(cacheKey); ok {
		return val.(*SearchResponse), nil
	}

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

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	c.addToCache(cacheKey, &result, 10*time.Minute)

	return &result, nil
}

func (c *dcClient) GetProject(key string) (interface{}, error) {
	cacheKey := "project:" + key
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

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

	c.addToCache(cacheKey, project, 5*time.Minute)
	return project, nil
}

func (c *dcClient) GetProjectStatuses(key string) (interface{}, error) {
	cacheKey := "statuses:" + key
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

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

	c.addToCache(cacheKey, statuses, 5*time.Minute)
	return statuses, nil
}

func (c *dcClient) GetBoard(id int) (interface{}, error) {
	cacheKey := fmt.Sprintf("board:%d", id)
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

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
	cacheKey := "find_projects:" + query
	if val, ok := c.getFromCache(cacheKey); ok {
		return val.([]interface{}), nil
	}

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

	c.addToCache(cacheKey, filtered, 5*time.Minute)
	return filtered, nil
}

func (c *dcClient) FindBoards(projectKey string, nameFilter string) ([]interface{}, error) {
	cacheKey := fmt.Sprintf("find_boards:%s:%s", projectKey, nameFilter)
	if val, ok := c.getFromCache(cacheKey); ok {
		return val.([]interface{}), nil
	}

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

	c.addToCache(cacheKey, filtered, 5*time.Minute)
	return filtered, nil
}
func (c *dcClient) GetBoardConfig(id int) (interface{}, error) {
	cacheKey := fmt.Sprintf("board_config:%d", id)
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

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

	c.addToCache(cacheKey, config, 5*time.Minute)
	return config, nil
}

func (c *dcClient) GetFilter(id string) (interface{}, error) {
	cacheKey := "filter:" + id
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

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

	c.addToCache(cacheKey, filter, 5*time.Minute)
	return filter, nil
}
