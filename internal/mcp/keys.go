package mcp

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Jira keys reach file names (cache files) and JQL, so they are checked once,
// where tool input enters the server. Jira stores keys in upper case only, so
// lower-case input is converted rather than rejected.
var (
	projectKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)
	issueKeyPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[0-9]+$`)
)

// normalizeProjectKey returns the upper-case project key, or an error if it is
// not a valid Jira project key. An empty key is returned unchanged: tools treat
// it as "not given".
func normalizeProjectKey(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	key := strings.ToUpper(raw)
	if !projectKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid project_key %q: use the Jira project key, e.g. PROJ (a letter followed by letters, digits or underscores)", raw)
	}
	return key, nil
}

// normalizeIssueKey returns the upper-case issue key, or an error if it is not
// a valid Jira issue key. An empty key is returned unchanged.
func normalizeIssueKey(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	key := strings.ToUpper(raw)
	if !issueKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid issue_key %q: use the Jira issue key, e.g. PROJ-123", raw)
	}
	return key, nil
}

// keyNormalizers maps the JSON name of a tool input field to its normalizer.
var keyNormalizers = map[string]func(string) (string, error){
	"project_key": normalizeProjectKey,
	"issue_key":   normalizeIssueKey,
}

// normalizeKeys normalizes, in place, every string field of the tool input
// struct that in points to whose JSON name is in keyNormalizers. It works on
// every tool input, so a new tool is covered without further code.
func normalizeKeys(in any) error {
	v := reflect.ValueOf(in).Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := range v.NumField() {
		field := v.Type().Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		normalize, ok := keyNormalizers[name]
		if !ok || field.Type.Kind() != reflect.String {
			continue
		}
		key, err := normalize(v.Field(i).String())
		if err != nil {
			return err
		}
		v.Field(i).SetString(key)
	}
	return nil
}
