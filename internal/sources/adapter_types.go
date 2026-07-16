package sources

import "time"

// sourceEvent is the smallest normalized unit an adapter may expose. Adapters
// must apply their source-specific role/schema allowlist before constructing an
// event and must provide a trustworthy event timestamp.
type sourceEvent struct {
	Text           string
	Timestamp      time.Time
	TimestampBasis string
	Sequence       int
}

// artifactResult keeps failure modes distinct so absence is never confused
// with a storage or schema we do not understand.
type artifactResult struct {
	Events             []sourceEvent
	ParseError         bool
	Truncated          bool
	Unreadable         bool
	UnsupportedSchema  bool
	UnsupportedStorage bool
}

// jsonLine is a decoded complete JSONL record. Sequence preserves its order in
// the bounded read window and is used only as a stable timestamp tie-breaker.
type jsonLine struct {
	Value    any
	Sequence int
}
