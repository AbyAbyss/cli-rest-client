package models

// RequestTemplate represents a saved HTTP request template
type RequestTemplate struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Body   string `json:"body"`
	Name   string `json:"name"`
}

// AppState represents the current application state
type AppState struct {
	Method    string `json:"method"`
	URL       string `json:"url"`
	Body      string `json:"body"`
	ActiveTab int    `json:"active_tab"`
	FocusIdx  int    `json:"focus_idx"`
}

// TreeNodeData represents data stored in tree nodes
type TreeNodeData struct {
	IsCollection bool             `json:"is_collection"`
	Template     *RequestTemplate `json:"template,omitempty"`
	Name         string           `json:"name"`
}

