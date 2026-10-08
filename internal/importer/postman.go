// Package importer reads Postman exports (collections v2.0/v2.1,
// environments and globals) into the workspace.
package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// Result describes what an import added.
type Result struct {
	Kind        string // "collection", "environment" or "globals"
	Collection  *models.Collection
	Environment *models.Environment
	Requests    int
	Folders     int
	Variables   int
	Warnings    []string
}

// Summary is a one-line description for status bars and the CLI.
func (r *Result) Summary() string {
	switch r.Kind {
	case "collection":
		return fmt.Sprintf("Imported collection %q: %d request(s) in %d folder(s)", r.Collection.Name, r.Requests, r.Folders)
	case "environment":
		return fmt.Sprintf("Imported environment %q with %d variable(s)", r.Environment.Name, r.Variables)
	default:
		return fmt.Sprintf("Imported %d global variable(s)", r.Variables)
	}
}

func (r *Result) warn(format string, args ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// Import detects what kind of Postman export data is and adds it to ws.
func Import(data []byte, ws *models.Workspace) (*Result, error) {
	var probe struct {
		Info   *struct{ Schema string } `json:"info"`
		Item   json.RawMessage          `json:"item"`
		Values json.RawMessage          `json:"values"`
		Scope  string                   `json:"_postman_variable_scope"`
		// v1 collections have "requests" and "order" at the top level.
		Requests json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("not a JSON file: %w", err)
	}
	switch {
	case probe.Info != nil && probe.Item != nil:
		if s := probe.Info.Schema; s != "" && !strings.Contains(s, "v2.") {
			return nil, fmt.Errorf("unsupported collection schema %q (export as Collection v2.1)", s)
		}
		return importCollection(data, ws)
	case probe.Values != nil && probe.Scope == "globals":
		return importVariables(data, ws, true)
	case probe.Values != nil:
		return importVariables(data, ws, false)
	case probe.Requests != nil:
		return nil, errors.New("this is a Postman Collection v1 export; in Postman choose Export, Collection v2.1")
	}
	return nil, errors.New("not a Postman collection or environment export")
}

// ---------- JSON shapes ----------

type pmKV struct {
	Key      string `json:"key"`
	Value    any    `json:"value"`
	Disabled bool   `json:"disabled"`
	Enabled  *bool  `json:"enabled"` // environments use enabled instead of disabled
	Type     string `json:"type"`
	Src      any    `json:"src"`
}

func (kv pmKV) value() string {
	switch v := kv.Value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func (kv pmKV) off() bool {
	return kv.Disabled || (kv.Enabled != nil && !*kv.Enabled)
}

type pmItem struct {
	Name     string          `json:"name"`
	Item     []pmItem        `json:"item"`
	Request  json.RawMessage `json:"request"`
	Auth     json.RawMessage `json:"auth"`
	Event    []pmEvent       `json:"event"`
	Variable []pmKV          `json:"variable"`
}

type pmEvent struct {
	Listen string `json:"listen"`
	Script struct {
		Exec json.RawMessage `json:"exec"` // string or []string
	} `json:"script"`
	Disabled bool `json:"disabled"`
}

type pmRequest struct {
	Method string          `json:"method"`
	URL    json.RawMessage `json:"url"`
	Header json.RawMessage `json:"header"` // []pmKV or a string
	Body   *pmBody         `json:"body"`
	Auth   json.RawMessage `json:"auth"`
}

type pmURL struct {
	Raw      string          `json:"raw"`
	Protocol string          `json:"protocol"`
	Host     json.RawMessage `json:"host"` // string or []string
	Path     json.RawMessage `json:"path"` // string or []string/objects
	Port     string          `json:"port"`
	Query    []pmKV          `json:"query"`
	Variable []pmKV          `json:"variable"`
}

type pmBody struct {
	Mode       string          `json:"mode"`
	Raw        string          `json:"raw"`
	URLEncoded []pmKV          `json:"urlencoded"`
	FormData   []pmKV          `json:"formdata"`
	GraphQL    json.RawMessage `json:"graphql"`
	Disabled   bool            `json:"disabled"`
	Options    struct {
		Raw struct {
			Language string `json:"language"`
		} `json:"raw"`
	} `json:"options"`
}

// ---------- collections ----------

func importCollection(data []byte, ws *models.Workspace) (*Result, error) {
	var col struct {
		Info struct {
			Name string `json:"name"`
		} `json:"info"`
		Item     []pmItem        `json:"item"`
		Auth     json.RawMessage `json:"auth"`
		Event    []pmEvent       `json:"event"`
		Variable []pmKV          `json:"variable"`
	}
	if err := json.Unmarshal(data, &col); err != nil {
		return nil, fmt.Errorf("reading collection: %w", err)
	}
	res := &Result{Kind: "collection"}
	name := strings.TrimSpace(col.Info.Name)
	if name == "" {
		name = "Imported Collection"
	}
	root := &models.Collection{Name: uniqueCollectionName(ws, name)}
	if root.Name != name {
		res.warn("A collection named %q already exists, imported as %q", name, root.Name)
	}
	if hasScripts(col.Event) {
		res.warn("Collection-level scripts are JavaScript and were not imported")
	}

	rootAuth := parseAuth(col.Auth, nil, res, name)
	importItems(col.Item, root, rootAuth, res, name)
	res.Collection = root
	ws.Collections = append(ws.Collections, root)

	// Collection variables become globals (this app has no per-collection
	// variables). Existing globals are kept.
	for _, v := range col.Variable {
		if v.Key == "" {
			continue
		}
		val := convertDynamic(v.value())
		if cur, ok := ws.VariableMap()[v.Key]; ok && ws.VariableSource(v.Key) == "Globals" {
			if cur != val {
				res.warn("Collection variable %q not imported: a global with that name already exists (value %q)", v.Key, cur)
			}
			continue
		}
		ws.SetGlobal(v.Key, val)
		res.Variables++
	}
	if res.Variables > 0 {
		res.warn("%d collection variable(s) were added to Globals", res.Variables)
	}
	return res, nil
}

func uniqueCollectionName(ws *models.Workspace, base string) string {
	taken := map[string]bool{}
	for _, c := range ws.Collections {
		taken[strings.ToLower(c.Name)] = true
	}
	name := base
	for i := 2; taken[strings.ToLower(name)]; i++ {
		name = fmt.Sprintf("%s %d", base, i)
	}
	return name
}

func importItems(items []pmItem, into *models.Collection, inherited *models.Auth, res *Result, path string) {
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		itemPath := path + " / " + name
		if it.Request == nil && it.Item != nil {
			// Folder: has an "item" list (possibly empty) and no request.
			f := &models.Collection{Name: name}
			if f.Name == "" {
				f.Name = "Folder"
			}
			auth := parseAuth(it.Auth, inherited, res, itemPath)
			if hasScripts(it.Event) {
				res.warn("%s: folder scripts are JavaScript and were not imported", itemPath)
			}
			importItems(it.Item, f, auth, res, itemPath)
			into.Folders = append(into.Folders, f)
			res.Folders++
			continue
		}
		if it.Request == nil {
			continue
		}
		r, err := convertRequest(it, inherited, res, itemPath)
		if err != nil {
			res.warn("%s: skipped (%v)", itemPath, err)
			continue
		}
		into.Requests = append(into.Requests, r)
		res.Requests++
	}
}

func convertRequest(it pmItem, inherited *models.Auth, res *Result, path string) (*models.Request, error) {
	r := models.NewRequest(strings.TrimSpace(it.Name))
	if r.Name == "" {
		r.Name = "Request"
	}

	var pr pmRequest
	var asString string
	if json.Unmarshal(it.Request, &asString) == nil {
		pr = pmRequest{Method: "GET", URL: mustJSON(asString)}
	} else if err := json.Unmarshal(it.Request, &pr); err != nil {
		return nil, err
	}
	r.Method = strings.ToUpper(strings.TrimSpace(pr.Method))
	if r.Method == "" {
		r.Method = "GET"
	}
	if indexOf(models.Methods, r.Method) < 0 {
		res.warn("%s: method %s is not supported, imported as GET", path, r.Method)
		r.Method = "GET"
	}

	r.URL, r.Params = convertURL(pr.URL, res, path)

	var headers []pmKV
	if json.Unmarshal(pr.Header, &headers) != nil {
		// Old exports may store headers as a "Key: value\n" string.
		var s string
		if json.Unmarshal(pr.Header, &s) == nil {
			for _, kv := range models.ParseKV(s, ":") {
				headers = append(headers, pmKV{Key: kv.Key, Value: kv.Value, Disabled: kv.Disabled})
			}
		}
	}
	for _, h := range headers {
		if h.Key == "" {
			continue
		}
		r.Headers = append(r.Headers, models.KeyValue{Key: convertDynamic(h.Key), Value: convertDynamic(h.value()), Disabled: h.off()})
	}

	if a := parseAuth(pr.Auth, inherited, res, path); a != nil {
		r.Auth = *a
	}
	convertBody(pr.Body, &r, res, path)
	r.PreRequest, r.Tests = convertScripts(it.Event)
	r.Normalize()
	return &r, nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

var pathVar = regexp.MustCompile(`/:([A-Za-z_][A-Za-z0-9_]*)`)

// convertURL returns the URL without its query string and the query
// parameters as Params (so disabled ones are kept).
func convertURL(raw json.RawMessage, res *Result, path string) (string, []models.KeyValue) {
	var u pmURL
	var s string
	if json.Unmarshal(raw, &s) == nil {
		u.Raw = s
	} else {
		_ = json.Unmarshal(raw, &u)
	}

	full := u.Raw
	if full == "" {
		full = buildURL(u)
	}
	base, query, _ := strings.Cut(full, "?")
	base = convertDynamic(base)

	var params []models.KeyValue
	if u.Query != nil {
		for _, q := range u.Query {
			if q.Key == "" && q.value() == "" {
				continue
			}
			params = append(params, models.KeyValue{Key: convertDynamic(q.Key), Value: convertDynamic(q.value()), Disabled: q.off()})
		}
	} else if query != "" {
		for _, part := range strings.Split(query, "&") {
			k, v, _ := strings.Cut(part, "=")
			if uk, err := url.QueryUnescape(k); err == nil {
				k = uk
			}
			if uv, err := url.QueryUnescape(v); err == nil {
				v = uv
			}
			params = append(params, models.KeyValue{Key: convertDynamic(k), Value: convertDynamic(v)})
		}
	}

	// Path variables (/users/:id) become their value, or {{id}} when Postman
	// had no value for them.
	values := map[string]string{}
	for _, v := range u.Variable {
		values[v.Key] = convertDynamic(v.value())
	}
	base = pathVar.ReplaceAllStringFunc(base, func(m string) string {
		name := m[2:]
		if v := values[name]; v != "" {
			return "/" + v
		}
		res.warn("%s: path variable :%s became {{%s}}; define it in Variables", path, name, name)
		return "/{{" + name + "}}"
	})
	return base, params
}

func buildURL(u pmURL) string {
	join := func(raw json.RawMessage, sep string) string {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		var parts []any
		_ = json.Unmarshal(raw, &parts)
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			switch v := p.(type) {
			case string:
				out = append(out, v)
			case map[string]any: // path segment objects: {"type": "string", "value": "x"}
				if s, ok := v["value"].(string); ok {
					out = append(out, s)
				}
			}
		}
		return strings.Join(out, sep)
	}
	s := ""
	if u.Protocol != "" {
		s = u.Protocol + "://"
	}
	s += join(u.Host, ".")
	if u.Port != "" {
		s += ":" + u.Port
	}
	if p := join(u.Path, "/"); p != "" {
		s += "/" + strings.TrimPrefix(p, "/")
	}
	return s
}

// parseAuth returns the auth for an item: its own, or inherited when it has
// none or says "inherit". The result is a copy safe to modify.
func parseAuth(raw json.RawMessage, inherited *models.Auth, res *Result, path string) *models.Auth {
	if len(raw) == 0 || string(raw) == "null" {
		return inherited
	}
	var a struct {
		Type   string          `json:"type"`
		Bearer json.RawMessage `json:"bearer"`
		Basic  json.RawMessage `json:"basic"`
		APIKey json.RawMessage `json:"apikey"`
	}
	if json.Unmarshal(raw, &a) != nil {
		return inherited
	}
	params := func(raw json.RawMessage) map[string]string {
		m := map[string]string{}
		var list []pmKV // v2.1: [{key, value, type}]
		if json.Unmarshal(raw, &list) == nil {
			for _, kv := range list {
				m[kv.Key] = convertDynamic(kv.value())
			}
			return m
		}
		var obj map[string]any // v2.0: {"token": "..."}
		if json.Unmarshal(raw, &obj) == nil {
			for k, v := range obj {
				m[k] = convertDynamic(pmKV{Value: v}.value())
			}
		}
		return m
	}
	switch a.Type {
	case "", "inherit":
		return inherited
	case "noauth":
		return &models.Auth{Type: models.AuthNone, In: "header"}
	case "bearer":
		return &models.Auth{Type: models.AuthBearer, Token: params(a.Bearer)["token"], In: "header"}
	case "basic":
		p := params(a.Basic)
		return &models.Auth{Type: models.AuthBasic, Username: p["username"], Password: p["password"], In: "header"}
	case "apikey":
		p := params(a.APIKey)
		in := "header"
		if p["in"] == "query" {
			in = "query"
		}
		key := p["key"]
		if key == "" {
			key = "X-API-Key"
		}
		return &models.Auth{Type: models.AuthAPIKey, Key: key, Value: p["value"], In: in}
	}
	res.warn("%s: %s auth is not supported, imported without auth", path, a.Type)
	return &models.Auth{Type: models.AuthNone, In: "header"}
}

func convertBody(b *pmBody, r *models.Request, res *Result, path string) {
	if b == nil || b.Disabled {
		return
	}
	kvLines := func(kvs []pmKV) string {
		var out []models.KeyValue
		for _, kv := range kvs {
			if kv.Key == "" {
				continue
			}
			out = append(out, models.KeyValue{Key: convertDynamic(kv.Key), Value: convertDynamic(kv.value()), Disabled: kv.off()})
		}
		return models.FormatKV(out, "=")
	}
	switch b.Mode {
	case "raw":
		if b.Raw == "" {
			return
		}
		r.Body = convertDynamic(b.Raw)
		switch strings.ToLower(b.Options.Raw.Language) {
		case "json":
			r.BodyType = models.BodyJSON
		case "xml", "html":
			r.BodyType = models.BodyXML
		case "text", "javascript":
			r.BodyType = models.BodyText
		default:
			trimmed := strings.TrimSpace(b.Raw)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				r.BodyType = models.BodyJSON
			} else {
				r.BodyType = models.BodyText
			}
		}
	case "urlencoded":
		r.BodyType = models.BodyForm
		r.Body = kvLines(b.URLEncoded)
	case "formdata":
		var fields []pmKV
		for _, f := range b.FormData {
			if f.Type == "file" {
				res.warn("%s: form-data file field %q was skipped (file uploads are not supported)", path, f.Key)
				continue
			}
			fields = append(fields, f)
		}
		r.BodyType = models.BodyForm
		r.Body = kvLines(fields)
		res.warn("%s: multipart form-data is sent as urlencoded form fields", path)
	case "graphql":
		var g struct {
			Query     string `json:"query"`
			Variables string `json:"variables"`
		}
		_ = json.Unmarshal(b.GraphQL, &g)
		payload := map[string]any{"query": g.Query}
		if v := strings.TrimSpace(g.Variables); v != "" {
			var parsed any
			if json.Unmarshal([]byte(v), &parsed) == nil {
				payload["variables"] = parsed
			}
		}
		out, _ := json.MarshalIndent(payload, "", "  ")
		r.BodyType = models.BodyJSON
		r.Body = convertDynamic(string(out))
	case "file":
		res.warn("%s: binary file body was skipped (file uploads are not supported)", path)
	}
}

// ---------- scripts ----------

func hasScripts(events []pmEvent) bool {
	for _, e := range events {
		if len(scriptLines(e)) > 0 {
			return true
		}
	}
	return false
}

func scriptLines(e pmEvent) []string {
	if e.Disabled {
		return nil
	}
	var lines []string
	if json.Unmarshal(e.Script.Exec, &lines) != nil {
		var s string
		if json.Unmarshal(e.Script.Exec, &s) == nil {
			lines = strings.Split(s, "\n")
		}
	}
	var out []string
	for _, l := range lines {
		for _, part := range strings.Split(l, "\n") {
			if strings.TrimSpace(part) != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

var (
	reStatus      = regexp.MustCompile(`pm\.response\.to\.have\.status\(\s*(\d{3})\s*\)`)
	reStatusEql   = regexp.MustCompile(`pm\.expect\(\s*pm\.response\.(?:code|status)\s*\)\.to\.(?:eql|equal|be\.equal)\(\s*(\d{3})\s*\)`)
	reTimeBelow   = regexp.MustCompile(`pm\.expect\(\s*pm\.response\.responseTime\s*\)\.to\.be\.below\(\s*(\d+)\s*\)`)
	reHeaderCheck = regexp.MustCompile(`pm\.response\.to\.have\.header\(\s*["']([^"']+)["']\s*\)`)
	reSetFromJSON = regexp.MustCompile(`pm\.(?:environment|globals|collectionVariables|variables)\.set\(\s*["']([^"']+)["']\s*,\s*(?:pm\.response\.json\(\)|jsonData|json|data|body|response)((?:\.[A-Za-z_$][\w$]*|\[\d+\])+)\s*\)`)
	reSetLiteral  = regexp.MustCompile(`pm\.(?:environment|globals|collectionVariables|variables)\.set\(\s*["']([^"']+)["']\s*,\s*["']([^"']*)["']\s*\)`)
)

// convertScripts turns Postman's JavaScript into this app's script lines
// where a common pattern is recognised; everything else is kept as comments
// so nothing is lost.
func convertScripts(events []pmEvent) (pre, tests string) {
	for _, e := range events {
		lines := scriptLines(e)
		if len(lines) == 0 {
			continue
		}
		var converted []string
		for _, l := range lines {
			if e.Listen == "prerequest" {
				if m := reSetLiteral.FindStringSubmatch(l); m != nil {
					converted = append(converted, fmt.Sprintf("set %s = %s", m[1], convertDynamic(m[2])))
				}
				continue
			}
			for _, m := range reStatus.FindAllStringSubmatch(l, -1) {
				converted = append(converted, "status == "+m[1])
			}
			for _, m := range reStatusEql.FindAllStringSubmatch(l, -1) {
				converted = append(converted, "status == "+m[1])
			}
			for _, m := range reTimeBelow.FindAllStringSubmatch(l, -1) {
				converted = append(converted, "time < "+m[1])
			}
			for _, m := range reHeaderCheck.FindAllStringSubmatch(l, -1) {
				converted = append(converted, "header "+m[1]+" exists")
			}
			for _, m := range reSetFromJSON.FindAllStringSubmatch(l, -1) {
				converted = append(converted, "set "+m[1]+" = json"+m[2])
			}
		}
		var sb strings.Builder
		if len(converted) > 0 {
			sb.WriteString("# Converted from the Postman script below:\n")
			sb.WriteString(strings.Join(converted, "\n"))
			sb.WriteString("\n\n")
		}
		sb.WriteString("# Original Postman script (JavaScript, not run here):\n")
		for _, l := range lines {
			sb.WriteString("# " + l + "\n")
		}
		text := strings.TrimRight(sb.String(), "\n")
		switch e.Listen {
		case "prerequest":
			pre = text
		case "test":
			tests = text
		}
	}
	return pre, tests
}

// ---------- environments and globals ----------

func importVariables(data []byte, ws *models.Workspace, globals bool) (*Result, error) {
	var env struct {
		Name   string `json:"name"`
		Values []pmKV `json:"values"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("reading variables: %w", err)
	}
	var kvs []models.KeyValue
	for _, v := range env.Values {
		if v.Key == "" {
			continue
		}
		kvs = append(kvs, models.KeyValue{Key: v.Key, Value: convertDynamic(v.value()), Disabled: v.off()})
	}

	if globals {
		res := &Result{Kind: "globals"}
		existing := map[string]string{}
		for _, kv := range ws.Variables {
			existing[kv.Key] = kv.Value
		}
		for _, kv := range kvs {
			if cur, ok := existing[kv.Key]; ok {
				if cur != kv.Value {
					res.warn("Global %q kept its current value (the import had %q)", kv.Key, kv.Value)
				}
				continue
			}
			ws.Variables = append(ws.Variables, kv)
			res.Variables++
		}
		return res, nil
	}

	name := strings.TrimSpace(env.Name)
	if name == "" {
		name = "Imported Environment"
	}
	res := &Result{Kind: "environment", Variables: len(kvs)}
	e := &models.Environment{Name: ws.UniqueEnvName(name), Variables: kvs}
	if e.Name != name {
		res.warn("An environment named %q already exists, imported as %q", name, e.Name)
	}
	ws.Environments = append(ws.Environments, e)
	res.Environment = e
	return res, nil
}

// ---------- dynamic variables ----------

// Postman's dynamic variables that have a direct equivalent here.
var dynamicMap = map[string]string{
	"$guid":         "$uuid",
	"$randomUUID":   "$uuid",
	"$timestamp":    "$timestamp",
	"$isoTimestamp": "$isoTimestamp",
	"$randomInt":    "$randomInt",
}

var reDynamic = regexp.MustCompile(`\{\{\s*(\$[A-Za-z]+)\s*\}\}`)

func convertDynamic(s string) string {
	return reDynamic.ReplaceAllStringFunc(s, func(m string) string {
		name := reDynamic.FindStringSubmatch(m)[1]
		if to, ok := dynamicMap[name]; ok {
			return "{{" + to + "}}"
		}
		return m
	})
}
