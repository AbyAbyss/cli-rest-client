package models

import "time"

// HistoryLimit is how many entries are kept; older ones are dropped.
const HistoryLimit = 200

// HistoryBodyLimit caps how much of each response body is kept in history.
const HistoryBodyLimit = 64 << 10

// TestOutcome is a stored test result.
type TestOutcome struct {
	Source  string `json:"source"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
	Capture bool   `json:"capture,omitempty"`
}

// HistoryEntry records one sent request and what came back.
type HistoryEntry struct {
	Time time.Time `json:"time"`
	// Request is the request as edited, with {{variables}} unresolved, so
	// it can be opened and sent again.
	Request Request `json:"request"`
	// Source is the path of the saved request it was sent from, if any.
	Source string `json:"source,omitempty"`
	// Environment is the environment that was active ("" = none).
	Environment string `json:"environment,omitempty"`

	// What was sent, with variables resolved.
	Method         string              `json:"method"`
	URL            string              `json:"url"`
	RequestHeaders map[string][]string `json:"request_headers,omitempty"`

	// What came back. Error is set instead when no response arrived.
	Status          int                 `json:"status,omitempty"`
	StatusText      string              `json:"status_text,omitempty"`
	Proto           string              `json:"proto,omitempty"`
	DurationMs      int64               `json:"duration_ms"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
	Body            []byte              `json:"body,omitempty"`
	BodySize        int                 `json:"body_size,omitempty"`
	BodyTruncated   bool                `json:"body_truncated,omitempty"`
	FinalURL        string              `json:"final_url,omitempty"`
	Error           string              `json:"error,omitempty"`
	Tests           []TestOutcome       `json:"tests,omitempty"`
}

// History is the list of sent requests, newest first.
type History struct {
	Entries []*HistoryEntry `json:"entries"`
}

// Add puts e at the front and drops entries beyond HistoryLimit. Bodies
// larger than HistoryBodyLimit are cut.
func (h *History) Add(e *HistoryEntry) {
	e.Request = e.Request.Clone()
	if e.BodySize == 0 {
		e.BodySize = len(e.Body)
	}
	if len(e.Body) > HistoryBodyLimit {
		e.Body = e.Body[:HistoryBodyLimit]
		e.BodyTruncated = true
	}
	h.Entries = append([]*HistoryEntry{e}, h.Entries...)
	if len(h.Entries) > HistoryLimit {
		h.Entries = h.Entries[:HistoryLimit]
	}
}

// Remove deletes e from the history.
func (h *History) Remove(e *HistoryEntry) {
	for i, x := range h.Entries {
		if x == e {
			h.Entries = append(h.Entries[:i], h.Entries[i+1:]...)
			return
		}
	}
}
