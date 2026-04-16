package eventlog

import "time"

// ResolutionGracePeriod is the time window within which duplicate resolution-change events
// are collapsed into a single event. This handles Jira changelog entries where both a
// "status change" and a "resolution set" are recorded within the same history item.
const ResolutionGracePeriod = 2 * time.Second

// DateTimeFormat is the canonical minute-precision date-time layout used when
// rendering timestamps into JQL boundaries and user-facing status messages.
const DateTimeFormat = "2006-01-02 15:04"
