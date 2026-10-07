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
)

// BodyTypes lists the body types in the order shown in the Body tab.
var BodyTypes = []string{BodyNone, BodyJSON, BodyText, BodyXML, BodyForm}

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
	Name       string     `json:"name"`
	Method     string     `json:"method"`
	URL        string     `json:"url"`
	Params     []KeyValue `json:"params,omitempty"`
	Headers    []KeyValue `json:"headers,omitempty"`
	Auth       Auth       `json:"auth"`
	BodyType   string     `json:"body_type"`
	Body       string     `json:"body,omitempty"`
	PreRequest string     `json:"pre_request,omitempty"`
	Tests      string     `json:"tests,omitempty"`
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
		r.BodyType != o.BodyType || r.Body != o.Body || r.PreRequest != o.PreRequest || r.Tests != o.Tests {
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

// Collection is a named group of requests.
type Collection struct {
	Name     string     `json:"name"`
	Requests []*Request `json:"requests"`
}

// Settings are user preferences.
type Settings struct {
	Theme              string `json:"theme"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	DisableRedirects   bool   `json:"disable_redirects,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"`
}

// Draft is the unsaved builder state, restored on the next start.
type Draft struct {
	Request Request `json:"request"`
	// Collection and Index point at the saved request the draft was loaded
	// from, or are -1 when the draft is not linked to a saved request.
	Collection int `json:"collection"`
	Index      int `json:"index"`
}

// Workspace is everything persisted between runs.
type Workspace struct {
	Version     int           `json:"version"`
	Collections []*Collection `json:"collections"`
	Variables   []KeyValue    `json:"variables,omitempty"`
	Settings    Settings      `json:"settings"`
	Draft       *Draft        `json:"draft,omitempty"`
}

// CurrentVersion is the workspace file format version.
const CurrentVersion = 1

// Normalize fills in defaults after loading.
func (w *Workspace) Normalize() {
	if w.Version == 0 {
		w.Version = CurrentVersion
	}
	if w.Settings.TimeoutSeconds <= 0 {
		w.Settings.TimeoutSeconds = 30
	}
	cols := w.Collections[:0]
	for _, c := range w.Collections {
		if c == nil {
			continue
		}
		reqs := c.Requests[:0]
		for _, r := range c.Requests {
			if r == nil {
				continue
			}
			r.Normalize()
			reqs = append(reqs, r)
		}
		c.Requests = reqs
		cols = append(cols, c)
	}
	w.Collections = cols
	if w.Draft != nil {
		w.Draft.Request.Normalize()
	}
}

// VariableMap returns the enabled variables as a map.
func (w *Workspace) VariableMap() map[string]string {
	m := make(map[string]string, len(w.Variables))
	for _, kv := range w.Variables {
		if !kv.Disabled && kv.Key != "" {
			m[kv.Key] = kv.Value
		}
	}
	return m
}

// SetVariable sets (or adds) an enabled variable.
func (w *Workspace) SetVariable(key, value string) {
	for i := range w.Variables {
		if w.Variables[i].Key == key {
			w.Variables[i].Value = value
			w.Variables[i].Disabled = false
			return
		}
	}
	w.Variables = append(w.Variables, KeyValue{Key: key, Value: value})
}

// UnsetVariable removes a variable.
func (w *Workspace) UnsetVariable(key string) {
	out := w.Variables[:0]
	for _, kv := range w.Variables {
		if kv.Key != key {
			out = append(out, kv)
		}
	}
	w.Variables = out
}

// Locate returns the collection and request index of r, or -1, -1.
func (w *Workspace) Locate(r *Request) (int, int) {
	for ci, c := range w.Collections {
		for ri, x := range c.Requests {
			if x == r {
				return ci, ri
			}
		}
	}
	return -1, -1
}

// CollectionOf returns the collection holding r, or nil.
func (w *Workspace) CollectionOf(r *Request) *Collection {
	ci, _ := w.Locate(r)
	if ci < 0 {
		return nil
	}
	return w.Collections[ci]
}
