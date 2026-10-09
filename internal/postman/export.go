package postman

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// SchemaV21 is the Postman collection format written by ExportCollection.
const SchemaV21 = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"

// ExportResult describes an export.
type ExportResult struct {
	Data     []byte
	Requests int
	Folders  int
	// Notes lists things that changed on the way (untranslated script lines).
	Notes []string
}

// ExportCollection writes c (a collection or folder) as a Postman v2.1
// collection. Every request carries its own auth, so nothing depends on
// Postman's inheritance.
func ExportCollection(c *models.Collection) (*ExportResult, error) {
	res := &ExportResult{}
	out := map[string]any{
		"info": map[string]any{
			"_postman_id": newID(),
			"name":        c.Name,
			"schema":      SchemaV21,
		},
		"item": exportItems(c, res, c.Name),
	}
	data, err := json.MarshalIndent(out, "", "\t")
	if err != nil {
		return nil, err
	}
	res.Data = append(data, '\n')
	return res, nil
}

// ExportRequests writes loose requests (the top level of the workspace) as
// a collection called name.
func ExportRequests(name string, reqs []*models.Request) (*ExportResult, error) {
	return ExportCollection(&models.Collection{Name: name, Requests: reqs})
}

func exportItems(c *models.Collection, res *ExportResult, path string) []any {
	items := []any{}
	for _, f := range c.Folders {
		res.Folders++
		items = append(items, map[string]any{
			"name": f.Name,
			"item": exportItems(f, res, path+" / "+f.Name),
		})
	}
	for _, r := range c.Requests {
		res.Requests++
		items = append(items, exportRequest(r, res, path+" / "+r.Name))
	}
	return items
}

func exportRequest(r *models.Request, res *ExportResult, path string) map[string]any {
	req := map[string]any{
		"method": r.Method,
		"header": exportKVs(r.Headers),
		"url":    exportURL(r.URL, r.Params),
		"auth":   exportAuth(r.Auth),
	}
	if body := exportBody(r); body != nil {
		req["body"] = body
	}
	item := map[string]any{"name": r.Name, "request": req}

	var events []any
	if js := exportScript(r.PreRequest, true, res, path); len(js) > 0 {
		events = append(events, map[string]any{
			"listen": "prerequest",
			"script": map[string]any{"type": "text/javascript", "exec": js},
		})
	}
	if js := exportScript(r.Tests, false, res, path); len(js) > 0 {
		events = append(events, map[string]any{
			"listen": "test",
			"script": map[string]any{"type": "text/javascript", "exec": js},
		})
	}
	if len(events) > 0 {
		item["event"] = events
	}
	return item
}

func exportKVs(kvs []models.KeyValue) []any {
	out := []any{}
	for _, kv := range kvs {
		if kv.Key == "" {
			continue
		}
		m := map[string]any{"key": exportDynamic(kv.Key), "value": exportDynamic(kv.Value), "type": "text"}
		if kv.Disabled {
			m["disabled"] = true
		}
		out = append(out, m)
	}
	return out
}

var reScheme = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*)://`)

// exportURL builds Postman's url object: raw (with enabled params) plus
// host/path/query so Postman shows the parts, disabled params included.
func exportURL(raw string, params []models.KeyValue) map[string]any {
	raw = exportDynamic(strings.TrimSpace(raw))
	base, existing, _ := strings.Cut(raw, "?")

	var query []any
	var enabled []string
	if existing != "" {
		for _, part := range strings.Split(existing, "&") {
			k, v, _ := strings.Cut(part, "=")
			query = append(query, map[string]any{"key": k, "value": v})
			enabled = append(enabled, part)
		}
	}
	for _, p := range params {
		if p.Key == "" {
			continue
		}
		q := map[string]any{"key": exportDynamic(p.Key), "value": exportDynamic(p.Value)}
		if p.Disabled {
			q["disabled"] = true
		} else {
			enabled = append(enabled, exportDynamic(p.Key)+"="+exportDynamic(p.Value))
		}
		query = append(query, q)
	}
	full := base
	if len(enabled) > 0 {
		full += "?" + strings.Join(enabled, "&")
	}

	u := map[string]any{"raw": full}
	rest := base
	if m := reScheme.FindStringSubmatch(rest); m != nil {
		u["protocol"] = m[1]
		rest = rest[len(m[0]):]
	}
	hostPart, pathPart, _ := strings.Cut(rest, "/")
	if strings.HasPrefix(hostPart, "{{") {
		u["host"] = []string{hostPart}
	} else {
		if h, port, ok := strings.Cut(hostPart, ":"); ok {
			hostPart = h
			u["port"] = port
		}
		if hostPart != "" {
			u["host"] = strings.Split(hostPart, ".")
		}
	}
	if pathPart != "" {
		u["path"] = strings.Split(pathPart, "/")
	}
	if len(query) > 0 {
		u["query"] = query
	}
	return u
}

func exportAuth(a models.Auth) map[string]any {
	param := func(k, v string) map[string]any {
		return map[string]any{"key": k, "value": exportDynamic(v), "type": "string"}
	}
	switch a.Type {
	case models.AuthBearer:
		return map[string]any{"type": "bearer", "bearer": []any{param("token", a.Token)}}
	case models.AuthBasic:
		return map[string]any{"type": "basic", "basic": []any{param("username", a.Username), param("password", a.Password)}}
	case models.AuthAPIKey:
		in := "header"
		if a.In == "query" {
			in = "query"
		}
		return map[string]any{"type": "apikey", "apikey": []any{param("key", a.Key), param("value", a.Value), param("in", in)}}
	}
	return map[string]any{"type": "noauth"}
}

func exportBody(r *models.Request) map[string]any {
	switch r.BodyType {
	case models.BodyJSON, models.BodyText, models.BodyXML:
		lang := map[string]string{models.BodyJSON: "json", models.BodyText: "text", models.BodyXML: "xml"}[r.BodyType]
		return map[string]any{
			"mode":    "raw",
			"raw":     exportDynamic(r.Body),
			"options": map[string]any{"raw": map[string]any{"language": lang}},
		}
	case models.BodyForm:
		return map[string]any{"mode": "urlencoded", "urlencoded": exportKVs(models.ParseKV(r.Body, "="))}
	case models.BodyGraphQL:
		return map[string]any{"mode": "graphql", "graphql": map[string]any{
			"query":     exportDynamic(r.Body),
			"variables": exportDynamic(r.GraphQLVariables),
		}}
	}
	return nil
}

// ---------- variables ----------

// ExportEnvironment writes e as a Postman environment file.
func ExportEnvironment(e *models.Environment) ([]byte, error) {
	return exportVariables(e.Name, e.Variables, "environment")
}

// ExportGlobals writes the workspace globals as a Postman globals file.
func ExportGlobals(vars []models.KeyValue) ([]byte, error) {
	return exportVariables("Globals", vars, "globals")
}

func exportVariables(name string, kvs []models.KeyValue, scope string) ([]byte, error) {
	values := []any{}
	for _, kv := range kvs {
		if kv.Key == "" {
			continue
		}
		values = append(values, map[string]any{
			"key":     kv.Key,
			"value":   exportDynamic(kv.Value),
			"type":    "default",
			"enabled": !kv.Disabled,
		})
	}
	data, err := json.MarshalIndent(map[string]any{
		"id":                      newID(),
		"name":                    name,
		"values":                  values,
		"_postman_variable_scope": scope,
		"_postman_exported_at":    time.Now().UTC().Format(time.RFC3339),
		"_postman_exported_using": "term-rest-client",
	}, "", "\t")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ---------- scripts ----------

const originalMarker = "# Original Postman script (JavaScript, not run here):"

// exportScript turns a script into Postman JavaScript. Scripts that came
// from a Postman import get their original JavaScript back unchanged;
// otherwise each line is translated, and lines that can't be are kept as
// comments.
func exportScript(src string, pre bool, res *ExportResult, path string) []string {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	if i := strings.Index(src, originalMarker); i >= 0 {
		var js []string
		for _, l := range strings.Split(src[i+len(originalMarker):], "\n") {
			if l = strings.TrimRight(l, " \r"); l == "" {
				continue
			}
			js = append(js, strings.TrimPrefix(strings.TrimPrefix(l, "#"), " "))
		}
		return js
	}

	var js []string
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//"):
			js = append(js, "// "+strings.TrimSpace(strings.TrimLeft(line, "#/")))
			continue
		}
		var out string
		var ok bool
		if pre {
			out, ok = preLineToJS(line)
		} else {
			out, ok = testLineToJS(line)
		}
		if !ok {
			res.Notes = append(res.Notes, fmt.Sprintf("%s: kept as a comment: %s", path, line))
			out = "// term-rest-client: " + line
		}
		js = append(js, out)
	}
	return js
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// jsValue renders an expected value: numbers, booleans and null stay
// literals, {{variables}} are resolved by Postman at run time.
func jsValue(s string) string {
	if strings.Contains(s, "{{") {
		return "pm.variables.replaceIn(" + jsString(exportDynamic(s)) + ")"
	}
	if s == "true" || s == "false" || s == "null" {
		return s
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return s
	}
	return jsString(s)
}

func preLineToJS(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "set "):
		name, value, ok := strings.Cut(strings.TrimSpace(line[4:]), "=")
		if !ok {
			return "", false
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		return fmt.Sprintf("pm.environment.set(%s, %s);", jsString(strings.TrimSpace(name)), jsValue(value)), true
	case strings.HasPrefix(line, "unset "):
		return fmt.Sprintf("pm.environment.unset(%s);", jsString(strings.TrimSpace(line[6:]))), true
	}
	return "", false
}

// eventsJS counts the events of a streamed response like the app does:
// SSE events with data, or non-empty NDJSON lines.
const eventsJS = `((pm.response.headers.get("Content-Type") || "").includes("event-stream")` +
	` ? pm.response.text().split(/\r?\n\r?\n|\r\r/).filter(b => /^data:/m.test(b)).length` +
	` : pm.response.text().split(/\r?\n|\r/).filter(l => l.trim() !== "").length)`

var reJSONPath = regexp.MustCompile(`^json((?:\.[A-Za-z_$][\w$]*|\[\d+\])*)$`)

// subjectJS returns the JavaScript for a test subject and the remaining
// tokens, or ok=false.
func subjectJS(toks []string) (js string, rest []string, isHeader bool, header string, ok bool) {
	if len(toks) == 0 {
		return "", nil, false, "", false
	}
	switch t := toks[0]; {
	case t == "status":
		return "pm.response.code", toks[1:], false, "", true
	case t == "time":
		return "pm.response.responseTime", toks[1:], false, "", true
	case t == "size":
		return "pm.response.responseSize", toks[1:], false, "", true
	case t == "body":
		return "pm.response.text()", toks[1:], false, "", true
	case t == "events":
		return eventsJS, toks[1:], false, "", true
	case t == "header" && len(toks) >= 2:
		return "pm.response.headers.get(" + jsString(toks[1]) + ")", toks[2:], true, toks[1], true
	case reJSONPath.MatchString(t):
		m := reJSONPath.FindStringSubmatch(t)
		return "pm.response.json()" + m[1], toks[1:], false, "", true
	}
	return "", nil, false, "", false
}

func testLineToJS(line string) (string, bool) {
	toks := splitTokens(line)
	if len(toks) >= 4 && toks[0] == "set" && toks[2] == "=" {
		subj, rest, _, _, ok := subjectJS(toks[3:])
		if !ok || len(rest) > 0 {
			return "", false
		}
		return fmt.Sprintf("pm.environment.set(%s, %s);", jsString(toks[1]), subj), true
	}
	subj, rest, isHeader, header, ok := subjectJS(toks)
	if !ok || len(rest) == 0 {
		return "", false
	}
	op := rest[0]
	expected := strings.Join(rest[1:], " ")
	isStatus := subj == "pm.response.code"
	var check string
	switch op {
	case "exists":
		if isHeader {
			check = fmt.Sprintf("pm.expect(pm.response.headers.has(%s)).to.be.true;", jsString(header))
		} else {
			check = fmt.Sprintf("pm.expect(%s).to.not.be.undefined;", subj)
		}
	case "!exists":
		if isHeader {
			check = fmt.Sprintf("pm.expect(pm.response.headers.has(%s)).to.be.false;", jsString(header))
		} else {
			check = fmt.Sprintf("pm.expect(%s).to.be.undefined;", subj)
		}
	case "==", "!=", "<", "<=", ">", ">=", "contains", "!contains", "matches", "!matches":
		if len(rest) < 2 {
			return "", false
		}
		chain := map[string]string{
			"==": "to.eql", "!=": "to.not.eql", "<": "to.be.below", "<=": "to.be.at.most",
			">": "to.be.above", ">=": "to.be.at.least", "contains": "to.include", "!contains": "to.not.include",
			"matches": "to.match", "!matches": "to.not.match",
		}[op]
		val := jsValue(expected)
		switch {
		case op == "matches" || op == "!matches":
			if strings.Contains(expected, "{{") {
				val = "new RegExp(" + val + ")"
			} else {
				val = "new RegExp(" + jsString(expected) + ")"
			}
		case op == "contains" || op == "!contains":
			if !strings.Contains(expected, "{{") {
				val = jsString(expected)
			}
		case op == "==" || op == "!=":
			// Match this app's equality: numbers compare numerically
			// ("1" == 1, 1.50 == 1.5), everything else as text.
			if _, err := strconv.ParseFloat(expected, 64); err == nil {
				subj = "Number(" + subj + ")"
			} else if strings.Contains(expected, "{{") || expected == "true" || expected == "false" || expected == "null" {
				subj = "String(" + subj + ")"
				if !strings.Contains(expected, "{{") {
					val = jsString(expected)
				}
			} else {
				val = jsString(expected)
			}
		}
		check = fmt.Sprintf("pm.expect(%s).%s(%s);", subj, chain, val)
	default:
		return "", false
	}
	if op == "==" && isStatus {
		check = fmt.Sprintf("pm.response.to.have.status(%s);", jsValue(expected))
	}
	return fmt.Sprintf("pm.test(%s, function () { %s });", jsString(line), check), true
}

// splitTokens splits on spaces, keeping a token that starts with a quote
// together up to the closing quote (quotes removed).
func splitTokens(s string) []string {
	var toks []string
	var cur strings.Builder
	in := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case (r == '"' || r == '\'') && !in:
			quote, in = r, true
		case r == ' ' || r == '\t':
			if in {
				toks = append(toks, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		toks = append(toks, cur.String())
	}
	return toks
}

// exportDynamic maps this app's built-in variables to Postman's names.
func exportDynamic(s string) string {
	return strings.ReplaceAll(s, "{{$uuid}}", "{{$randomUUID}}")
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
