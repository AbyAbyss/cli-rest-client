// Package models holds the data types that are persisted to the workspace file.
package models

// Supported HTTP methods, in the order they appear in the method picker.
var Methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// Auth types.
const (
	AuthNone   = "none"
	AuthBasic  = "basic"
	AuthBearer = "bearer"
	AuthAPIKey = "apikey"
)

// AuthTypes lists the auth types in the order shown in the Auth tab.
var AuthTypes = []string{AuthNone, AuthBasic, AuthBearer, AuthAPIKey}

// Body types.
const (
	BodyNone = "none"
	BodyJSON = "json"
	BodyText = "text"
	BodyXML  = "xml"
	BodyForm = "form"
	// BodyGraphQL sends Body as a GraphQL query, with GraphQLVariables.
	BodyGraphQL = "graphql"
)

// BodyTypes lists the body types in the order shown in the Body tab.
var BodyTypes = []string{BodyNone, BodyJSON, BodyText, BodyXML, BodyForm, BodyGraphQL}

// KeyValue is a single header, query parameter, form field or variable.
// Disabled entries are kept but not sent.
type KeyValue struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

// Auth describes how a request authenticates.
type Auth struct {
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	Key      string `json:"key,omitempty"`
	Value    string `json:"value,omitempty"`
	// In is "header" or "query" for API key auth.
	In string `json:"in,omitempty"`
}

// Request is a saved (or in-progress) HTTP request.
type Request struct {
	Name     string     `json:"name"`
	Method   string     `json:"method"`
	URL      string     `json:"url"`
	Params   []KeyValue `json:"params,omitempty"`
	Headers  []KeyValue `json:"headers,omitempty"`
	Auth     Auth       `json:"auth"`
	BodyType string     `json:"body_type"`
	Body     string     `json:"body,omitempty"`
	// GraphQLVariables is the JSON object sent as "variables" with a
	// GraphQL query (BodyGraphQL).
	GraphQLVariables string `json:"graphql_variables,omitempty"`
	PreRequest       string `json:"pre_request,omitempty"`
	Tests            string `json:"tests,omitempty"`
}

// NewRequest returns an empty GET request with defaults filled in.
func NewRequest(name string) Request {
	return Request{
		Name:     name,
		Method:   "GET",
		Auth:     Auth{Type: AuthNone, In: "header"},
		BodyType: BodyNone,
	}
}

// Normalize fills in defaults for fields that may be missing in older files.
func (r *Request) Normalize() {
	if r.Method == "" {
		r.Method = "GET"
	}
	if r.Auth.Type == "" {
		r.Auth.Type = AuthNone
	}
	if r.Auth.In == "" {
		r.Auth.In = "header"
	}
	if r.BodyType == "" {
		if r.Body != "" {
			r.BodyType = BodyJSON
		} else {
			r.BodyType = BodyNone
		}
	}
}

// Clone returns a deep copy of the request.
func (r Request) Clone() Request {
	c := r
	c.Params = append([]KeyValue(nil), r.Params...)
	c.Headers = append([]KeyValue(nil), r.Headers...)
	return c
}

// Equal reports whether two requests have the same content.
func (r Request) Equal(o Request) bool {
	if r.Name != o.Name || r.Method != o.Method || r.URL != o.URL || r.Auth != o.Auth ||
		r.BodyType != o.BodyType || r.Body != o.Body || r.GraphQLVariables != o.GraphQLVariables ||
		r.PreRequest != o.PreRequest || r.Tests != o.Tests {
		return false
	}
	return kvEqual(r.Params, o.Params) && kvEqual(r.Headers, o.Headers)
}

func kvEqual(a, b []KeyValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Collection is a named group of requests and sub-folders. A top-level
// Collection is shown as a collection; nested ones are folders. Folders can
// be nested to any depth.
type Collection struct {
	Name     string        `json:"name"`
	Folders  []*Collection `json:"folders,omitempty"`
	Requests []*Request    `json:"requests"`
}

// Settings are user preferences.
type Settings struct {
	Theme              string `json:"theme"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	DisableRedirects   bool   `json:"disable_redirects,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"`
	// CollapsedSections remembers which response pane sections are folded
	// ("tests", "request_headers", "response_headers", "body").
	CollapsedSections map[string]bool `json:"collapsed_sections,omitempty"`
	// DisableHistory stops recording sent requests.
	DisableHistory bool `json:"disable_history,omitempty"`
	// CodeLanguage is the language last picked in the code window (Ctrl+G),
	// which y also copies in.
	CodeLanguage string `json:"code_language,omitempty"`
}

// Draft is the unsaved builder state, restored on the next start.
type Draft struct {
	Request Request `json:"request"`
	// Folders and Index locate the saved request the draft was loaded from
	// (see Workspace.Location). Index is -1 when the draft is not linked.
	Folders []int `json:"folders,omitempty"`
	Index   int   `json:"index"`
	// Collection is the version 1 location (top-level collection index),
	// read for migration only.
	Collection int `json:"collection,omitempty"`
}

// Workspace is everything persisted between runs.
type Workspace struct {
	Version     int           `json:"version"`
	Collections []*Collection `json:"collections"`
	// Requests are saved requests that are not in any collection.
	Requests []*Request `json:"requests,omitempty"`
	// Variables are the global variables, available in every environment.
	Variables []KeyValue `json:"variables,omitempty"`
	// Environments are named variable sets (e.g. Local, Staging); the
	// active one's values override globals with the same name.
	Environments      []*Environment `json:"environments,omitempty"`
	ActiveEnvironment string         `json:"active_environment,omitempty"`
	Settings          Settings       `json:"settings"`
	Draft             *Draft         `json:"draft,omitempty"`
}

// CurrentVersion is the workspace file format version.
// Version 2 added folders, top-level requests and Draft.Folders.
const CurrentVersion = 2

// Normalize fills in defaults after loading and migrates older files.
func (w *Workspace) Normalize() {
	if w.Settings.TimeoutSeconds <= 0 {
		w.Settings.TimeoutSeconds = 30
	}
	w.Collections = normalizeCollections(w.Collections)
	envs := w.Environments[:0]
	for _, e := range w.Environments {
		if e != nil {
			envs = append(envs, e)
		}
	}
	w.Environments = envs
	if w.Active() == nil {
		w.ActiveEnvironment = ""
	}
	w.Requests = normalizeRequests(w.Requests)
	if d := w.Draft; d != nil {
		d.Request.Normalize()
		if w.Version < 2 {
			// v1: Collection/Index; -1/-1 meant "not linked".
			if d.Collection >= 0 && d.Index >= 0 {
				d.Folders = []int{d.Collection}
			} else {
				d.Folders, d.Index = nil, -1
			}
			d.Collection = 0
		}
	}
	w.Version = CurrentVersion
}

func normalizeCollections(cols []*Collection) []*Collection {
	out := cols[:0]
	for _, c := range cols {
		if c == nil {
			continue
		}
		c.Folders = normalizeCollections(c.Folders)
		c.Requests = normalizeRequests(c.Requests)
		out = append(out, c)
	}
	return out
}

func normalizeRequests(reqs []*Request) []*Request {
	out := reqs[:0]
	for _, r := range reqs {
		if r == nil {
			continue
		}
		r.Normalize()
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// VariableMap returns the enabled globals overridden by the enabled
// variables of the active environment.
func (w *Workspace) VariableMap() map[string]string {
	m := make(map[string]string, len(w.Variables))
	add := func(kvs []KeyValue) {
		for _, kv := range kvs {
			if !kv.Disabled && kv.Key != "" {
				m[kv.Key] = kv.Value
			}
		}
	}
	add(w.Variables)
	if env := w.Active(); env != nil {
		add(env.Variables)
	}
	return m
}

// SetVariable sets a variable where it lives: in the active environment if
// it is defined there, else in globals if defined there. New variables go
// into the active environment, or into globals when none is active.
func (w *Workspace) SetVariable(key, value string) {
	env := w.Active()
	switch {
	case env != nil && hasKey(env.Variables, key):
		env.Variables = setKV(env.Variables, key, value)
	case hasKey(w.Variables, key) || env == nil:
		w.Variables = setKV(w.Variables, key, value)
	default:
		env.Variables = setKV(env.Variables, key, value)
	}
}

// SetGlobal sets a global variable, ignoring environments.
func (w *Workspace) SetGlobal(key, value string) {
	w.Variables = setKV(w.Variables, key, value)
}

// UnsetVariable removes a variable from the active environment, or from
// globals when the environment doesn't define it.
func (w *Workspace) UnsetVariable(key string) {
	if env := w.Active(); env != nil && hasKey(env.Variables, key) {
		env.Variables = removeKV(env.Variables, key)
		return
	}
	w.Variables = removeKV(w.Variables, key)
}

// VariableSource names where the value of key comes from: the active
// environment's name, "Globals", or "" when it is not defined.
func (w *Workspace) VariableSource(key string) string {
	if env := w.Active(); env != nil {
		for _, kv := range env.Variables {
			if kv.Key == key && !kv.Disabled {
				return env.Name
			}
		}
	}
	for _, kv := range w.Variables {
		if kv.Key == key && !kv.Disabled {
			return "Globals"
		}
	}
	return ""
}

func hasKey(kvs []KeyValue, key string) bool {
	for _, kv := range kvs {
		if kv.Key == key {
			return true
		}
	}
	return false
}

func setKV(kvs []KeyValue, key, value string) []KeyValue {
	for i := range kvs {
		if kvs[i].Key == key {
			kvs[i].Value = value
			kvs[i].Disabled = false
			return kvs
		}
	}
	return append(kvs, KeyValue{Key: key, Value: value})
}

func removeKV(kvs []KeyValue, key string) []KeyValue {
	out := kvs[:0]
	for _, kv := range kvs {
		if kv.Key != key {
			out = append(out, kv)
		}
	}
	return out
}
