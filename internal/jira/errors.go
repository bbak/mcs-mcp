package jira

import (
	"fmt"
	"net/http"
)

// FailureKind classifies why a Jira call failed. The distinction matters to
// the calling agent: a rejected credential and an empty search result call for
// opposite responses, and collapsing both into "it didn't work" invites the
// agent to retry its way around a problem only the operator can fix.
type FailureKind string

const (
	// KindAuth means Jira rejected the credentials (401/403).
	KindAuth FailureKind = "auth"
	// KindNotFound means the endpoint or resource does not exist (404). On a
	// resource this is usually a wrong key; on a search endpoint it usually
	// means the wrong API dialect for this deployment.
	KindNotFound FailureKind = "not_found"
	// KindUnavailable means Jira is reachable but not serving (5xx, 429).
	KindUnavailable FailureKind = "unavailable"
	// KindTransport means Jira could not be reached at all.
	KindTransport FailureKind = "transport"
	// KindOther covers any other non-success status.
	KindOther FailureKind = "other"
)

// APIError is a Jira call that failed for an operational reason, as opposed to
// a query that legitimately returned nothing.
type APIError struct {
	Kind      FailureKind
	Status    int
	URL       string
	Operation string
	Err       error
}

func (e *APIError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("jira %s failed: status %d at %s", e.Operation, e.Status, e.URL)
	}
	return fmt.Sprintf("jira %s failed: %v", e.Operation, e.Err)
}

func (e *APIError) Unwrap() error { return e.Err }

// statusError builds an APIError from a response status, classifying it so the
// caller does not have to re-derive the meaning from the number.
func statusError(operation, url string, status int) *APIError {
	kind := KindOther
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		kind = KindAuth
	case status == http.StatusNotFound:
		kind = KindNotFound
	case status == http.StatusTooManyRequests, status >= 500:
		kind = KindUnavailable
	}
	return &APIError{Kind: kind, Status: status, URL: url, Operation: operation}
}

// transportError builds an APIError for a request that never got a response.
func transportError(operation, url string, err error) *APIError {
	return &APIError{Kind: KindTransport, URL: url, Operation: operation, Err: err}
}
