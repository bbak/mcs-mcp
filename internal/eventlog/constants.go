package eventlog

import "time"

// ResolutionGracePeriod is the time window within which duplicate resolution-change events
// are collapsed into a single event. This handles Jira changelog entries where both a
// "status change" and a "resolution set" are recorded within the same history item.
const ResolutionGracePeriod = 2 * time.Second
