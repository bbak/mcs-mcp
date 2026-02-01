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

	// Internal Inventory (Sliding Window)
	projectInventory []interface{}
	boardInventory   []interface{}
	inventoryMutex   sync.RWMutex
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
		log.Debug().Str("key", key).Msg("Cache miss")
		return nil, false
	}
	log.Debug().Str("key", key).Msg("Cache hit")

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
	log.Debug().Str("key", key).Dur("ttl", ttl).Msg("Added to cache")
}

func (c *dcClient) throttle(isMetadata bool) {
	// Metadata requests (Board, Config, Project) are allowed to "burst" sequentially
	// to avoid artificial delay during the setup phase.
	if isMetadata {
		c.lastRequest = time.Now()
		return
	}

	elapsed := time.Since(c.lastRequest)
	if elapsed < c.cfg.RequestDelay {
		wait := c.cfg.RequestDelay - elapsed
		log.Debug().Dur("wait", wait).Msg("Throttling Jira request")
		time.Sleep(wait)
	}
	c.lastRequest = time.Now()
}

func (c *dcClient) authenticateRequest(req *http.Request) {
	// 1. Prioritize Personal Access Token (PAT)
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.cfg.Token))
		return
	}

	// 2. Fallback to session cookies
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

	c.throttle(false)

	// Use url.Values for better query param handling
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("startAt", fmt.Sprintf("%d", startAt))
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	params.Set("fields", "issuetype,status,resolution,resolutiondate,created,updated")
	if expand != "" {
		params.Set("expand", expand)
	}

	searchURL := fmt.Sprintf("%s/rest/api/2/search?%s", c.cfg.BaseURL, params.Encode())
	log.Info().Msg("Requesting issues from Jira")
	log.Debug().Str("url", searchURL).Str("jql", jql).Msg("Jira search details")
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		case http.StatusTooManyRequests:
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter != "" {
				return nil, fmt.Errorf("Jira rate limit exceeded (429). Retry after %s seconds.", retryAfter)
			}
			return nil, fmt.Errorf("Jira rate limit exceeded (429).")
		default:
			return nil, fmt.Errorf("Jira API returned status %d. Please check Jira availability.", resp.StatusCode)
		}
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode Jira response: %w", err)
	}

	c.addToCache(cacheKey, &result, 10*time.Minute)

	return &result, nil
}

func (c *dcClient) GetProject(key string) (interface{}, error) {
	cacheKey := "project:" + key
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

	c.throttle(true)

	url := fmt.Sprintf("%s/rest/api/2/project/%s", c.cfg.BaseURL, key)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("project %s not found", key)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		default:
			return nil, fmt.Errorf("Jira API returned status %d for project %s", resp.StatusCode, key)
		}
	}

	var project map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, fmt.Errorf("failed to decode project response: %w", err)
	}

	c.addToCache(cacheKey, project, 5*time.Minute)
	// Add to inventory
	c.updateInventory(&c.projectInventory, []interface{}{project}, 1000, "key")
	return project, nil
}

func (c *dcClient) GetProjectStatuses(key string) (interface{}, error) {
	cacheKey := "statuses:" + key
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

	c.throttle(true)

	url := fmt.Sprintf("%s/rest/api/2/project/%s/statuses", c.cfg.BaseURL, key)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("project %s statuses not found", key)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		default:
			return nil, fmt.Errorf("Jira API returned status %d for project %s statuses", resp.StatusCode, key)
		}
	}

	var statuses []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("failed to decode project statuses response: %w", err)
	}

	c.addToCache(cacheKey, statuses, 5*time.Minute)
	return statuses, nil
}

func (c *dcClient) GetBoard(id int) (interface{}, error) {
	cacheKey := fmt.Sprintf("board:%d", id)
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

	c.throttle(true)

	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d", c.cfg.BaseURL, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("board %d not found", id)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		default:
			return nil, fmt.Errorf("Jira API returned status %d for board %d", resp.StatusCode, id)
		}
	}

	var board map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		return nil, fmt.Errorf("failed to decode board response: %w", err)
	}

	// Add to inventory
	c.updateInventory(&c.boardInventory, []interface{}{board}, 1000, "id")
	return board, nil
}
func (c *dcClient) FindProjects(query string) ([]interface{}, error) {
	cacheKey := "find_projects:" + query
	if val, ok := c.getFromCache(cacheKey); ok {
		return val.([]interface{}), nil
	}

	c.throttle(false)

	// Use /projects/picker for efficient server-side search
	params := url.Values{}
	params.Set("query", query)
	params.Set("maxResults", "30")

	// Note: /projects/picker is technically rest/api/2/projects/picker
	searchURL := fmt.Sprintf("%s/rest/api/2/projects/picker?%s", c.cfg.BaseURL, params.Encode())
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		default:
			return nil, fmt.Errorf("Jira API returned status %d for project search", resp.StatusCode)
		}
	}

	var pickerResponse struct {
		Projects []interface{} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pickerResponse); err != nil {
		return nil, fmt.Errorf("failed to decode project picker response: %w", err)
	}

	// Normalizing picker project structure to standard project structure
	var result []interface{}
	for _, p := range pickerResponse.Projects {
		pMap, ok := p.(map[string]interface{})
		if !ok {
			log.Warn().Msg("Failed to type-assert project from picker response")
			continue
		}
		result = append(result, map[string]interface{}{
			"id":   pMap["id"],
			"key":  pMap["key"],
			"name": pMap["name"],
		})
	}

	c.updateInventory(&c.projectInventory, result, 1000, "key")
	c.addToCache(cacheKey, result, 5*time.Minute)

	// Recall from inventory (merged perspective)
	return c.filterInventory(c.projectInventory, query, 30, "key", "name"), nil
}

func (c *dcClient) FindBoards(projectKey string, nameFilter string) ([]interface{}, error) {
	cacheKey := fmt.Sprintf("find_boards:%s:%s", projectKey, nameFilter)
	if val, ok := c.getFromCache(cacheKey); ok {
		return val.([]interface{}), nil
	}

	c.throttle(false)

	params := url.Values{}
	if projectKey != "" {
		params.Set("projectKeyOrId", projectKey)
	}
	if nameFilter != "" {
		params.Set("name", nameFilter)
	}
	params.Set("maxResults", "30")

	searchURL := fmt.Sprintf("%s/rest/agile/1.0/board?%s", c.cfg.BaseURL, params.Encode())
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		default:
			return nil, fmt.Errorf("Jira API returned status %d for board search", resp.StatusCode)
		}
	}

	var resultObj struct {
		Values []interface{} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resultObj); err != nil {
		return nil, fmt.Errorf("failed to decode board search response: %w", err)
	}

	c.updateInventory(&c.boardInventory, resultObj.Values, 1000, "id")
	c.addToCache(cacheKey, resultObj.Values, 5*time.Minute)

	// Recall from inventory (merged perspective)
	return c.filterInventory(c.boardInventory, nameFilter, 30, "name", "id"), nil
}

func (c *dcClient) updateInventory(inventory *[]interface{}, newItems []interface{}, limit int, idField string) {
	c.inventoryMutex.Lock()
	defer c.inventoryMutex.Unlock()

	for _, newItem := range newItems {
		newMap, ok := newItem.(map[string]interface{})
		if !ok {
			continue
		}

		newID := fmt.Sprintf("%v", newMap[idField])
		foundIdx := -1

		// Find if it already exists to move it to the end
		for i, existingItem := range *inventory {
			existingMap, ok := existingItem.(map[string]interface{})
			if !ok {
				continue
			}
			if fmt.Sprintf("%v", existingMap[idField]) == newID {
				foundIdx = i
				break
			}
		}

		if foundIdx != -1 {
			// Remove from current position
			*inventory = append((*inventory)[:foundIdx], (*inventory)[foundIdx+1:]...)
		}

		// Push to end
		*inventory = append(*inventory, newItem)
	}

	// Enforce cap
	if len(*inventory) > limit {
		*inventory = (*inventory)[len(*inventory)-limit:]
	}

	log.Debug().Int("size", len(*inventory)).Str("field", idField).Msg("Inventory updated")
}

func (c *dcClient) filterInventory(inventory []interface{}, query string, limit int, fields ...string) []interface{} {
	c.inventoryMutex.RLock()
	defer c.inventoryMutex.RUnlock()

	var matches []interface{}
	q := strings.ToLower(query)

	// Iterate backwards to prioritize most recent discoveries
	for i := len(inventory) - 1; i >= 0; i-- {
		item, ok := inventory[i].(map[string]interface{})
		if !ok {
			continue
		}
		match := false

		if q == "" {
			match = true
		} else {
			for _, field := range fields {
				if val, ok := item[field]; ok {
					sVal := strings.ToLower(fmt.Sprintf("%v", val))
					if strings.Contains(sVal, q) {
						match = true
						break
					}
				}
			}
		}

		if match {
			matches = append(matches, item)
		}
		if len(matches) >= limit {
			break
		}
	}

	return matches
}
func (c *dcClient) GetBoardConfig(id int) (interface{}, error) {
	cacheKey := fmt.Sprintf("board_config:%d", id)
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

	c.throttle(true)

	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d/configuration", c.cfg.BaseURL, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("board configuration %d not found", id)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		default:
			return nil, fmt.Errorf("Jira API returned status %d for board configuration %d", resp.StatusCode, id)
		}
	}

	var config map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode board configuration response: %w", err)
	}

	c.addToCache(cacheKey, config, 5*time.Minute)
	// Side-effect: we know about the board now. Re-fetch board metadata to ensure inventory is high-quality if needed.
	return config, nil
}

func (c *dcClient) GetFilter(id string) (interface{}, error) {
	cacheKey := "filter:" + id
	if val, ok := c.getFromCache(cacheKey); ok {
		return val, nil
	}

	c.throttle(true)

	url := fmt.Sprintf("%s/rest/api/2/filter/%s", c.cfg.BaseURL, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.authenticateRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("filter %s not found", id)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("Jira authentication failed (401/403). Please check your session cookies.")
		default:
			return nil, fmt.Errorf("Jira API returned status %d for filter %s", resp.StatusCode, id)
		}
	}

	var filter map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&filter); err != nil {
		return nil, fmt.Errorf("failed to decode filter response: %w", err)
	}

	c.addToCache(cacheKey, filter, 5*time.Minute)
	// Filters aren't currently in a dedicated sliding window, but we could add one if they become a primary anchor.
	return filter, nil
}
