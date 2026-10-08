// Package script runs the small line-based languages used by the
// Pre-request and Tests tabs.
//
// Pre-request scripts (run before sending):
//
//	set name = value      # value may use {{variables}}
//	unset name
//
// Test scripts (run after the response arrives), one assertion per line:
//
//	status == 200
//	time < 500                       # milliseconds
//	size <= 1024                     # body bytes
//	header Content-Type contains json
//	body contains "hello"
//	json.data.items[0].id == 42
//	json.token exists
//	set token = json.token           # capture into a variable
//
// Operators: == != > >= < <= contains !contains matches !matches exists !exists.
// Lines starting with # are comments.
package script

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/vars"
)

// Assignment is a variable change produced by a script.
type Assignment struct {
	Name  string
	Value string
	Unset bool
}

// Error is a script problem tied to a line.
type Error struct {
	Line int
	Msg  string
}

func (e Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// RunPre executes a pre-request script. Assignments are applied to vars in
// order (so later lines can use earlier ones) and also returned so the caller
// can persist them.
func RunPre(src string, variables map[string]string) ([]Assignment, []error) {
	var out []Assignment
	var errs []error
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		switch {
		case hasWord(line, "set"):
			name, value, ok := strings.Cut(strings.TrimSpace(line[3:]), "=")
			name = strings.TrimSpace(name)
			if !ok || !validName(name) {
				errs = append(errs, Error{i + 1, "expected: set name = value"})
				continue
			}
			v, _ := vars.Substitute(unquote(strings.TrimSpace(value)), variables)
			variables[name] = v
			out = append(out, Assignment{Name: name, Value: v})
		case hasWord(line, "unset"):
			name := strings.TrimSpace(line[5:])
			if !validName(name) {
				errs = append(errs, Error{i + 1, "expected: unset name"})
				continue
			}
			delete(variables, name)
			out = append(out, Assignment{Name: name, Unset: true})
		default:
			errs = append(errs, Error{i + 1, fmt.Sprintf("unknown statement %q (use set or unset)", line)})
		}
	}
	return out, errs
}

// Response is what test scripts can inspect.
type Response struct {
	Status   int
	Headers  http.Header
	Body     []byte
	Duration time.Duration
}

// Result is the outcome of one test line.
type Result struct {
	Line   int
	Source string
	Passed bool
	// Message explains a failure, or describes a capture.
	Message string
	// Capture is set for "set name = ..." lines that succeeded.
	Capture *Assignment
}

// RunTests evaluates a test script. Captures are applied to vars and returned
// as Results with Capture set.
func RunTests(src string, resp Response, variables map[string]string) []Result {
	var results []Result
	ctx := &evalCtx{resp: resp}
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		res := Result{Line: i + 1, Source: line}
		if hasWord(line, "set") {
			name, expr, ok := strings.Cut(strings.TrimSpace(line[3:]), "=")
			name = strings.TrimSpace(name)
			if !ok || !validName(name) {
				res.Message = "expected: set name = <status|time|size|body|header Name|json.path>"
			} else if toks, err := tokenize(strings.TrimSpace(expr)); err != nil {
				res.Message = err.Error()
			} else if val, exists, rest, err := ctx.subject(toks); err != nil {
				res.Message = err.Error()
			} else if len(rest) > 0 {
				res.Message = fmt.Sprintf("unexpected %q", strings.Join(rest, " "))
			} else if !exists {
				res.Message = fmt.Sprintf("%s not found in response", strings.TrimSpace(expr))
			} else {
				variables[name] = val
				res.Passed = true
				res.Capture = &Assignment{Name: name, Value: val}
				res.Message = fmt.Sprintf("%s = %s", name, abbreviate(val))
			}
			results = append(results, res)
			continue
		}
		res.Passed, res.Message = ctx.assert(line, variables)
		results = append(results, res)
	}
	return results
}

type evalCtx struct {
	resp    Response
	parsed  bool
	jsonDoc any
	jsonErr error
}

func (c *evalCtx) json() (any, error) {
	if !c.parsed {
		c.parsed = true
		dec := json.NewDecoder(bytes.NewReader(c.resp.Body))
		dec.UseNumber()
		c.jsonErr = dec.Decode(&c.jsonDoc)
		if c.jsonErr != nil {
			c.jsonErr = fmt.Errorf("response body is not JSON")
		}
	}
	return c.jsonDoc, c.jsonErr
}

// subject resolves the value being tested and returns the remaining tokens.
func (c *evalCtx) subject(toks []string) (value string, exists bool, rest []string, err error) {
	if len(toks) == 0 {
		return "", false, nil, fmt.Errorf("missing subject")
	}
	head := toks[0]
	switch {
	case head == "status":
		return strconv.Itoa(c.resp.Status), true, toks[1:], nil
	case head == "time":
		return strconv.FormatInt(c.resp.Duration.Milliseconds(), 10), true, toks[1:], nil
	case head == "size":
		return strconv.Itoa(len(c.resp.Body)), true, toks[1:], nil
	case head == "body":
		return string(c.resp.Body), true, toks[1:], nil
	case head == "header":
		if len(toks) < 2 {
			return "", false, nil, fmt.Errorf("expected: header <Name> ...")
		}
		vals, ok := c.resp.Headers[http.CanonicalHeaderKey(toks[1])]
		return strings.Join(vals, ", "), ok, toks[2:], nil
	case head == "json" || strings.HasPrefix(head, "json.") || strings.HasPrefix(head, "json["):
		doc, err := c.json()
		if err != nil {
			return "", false, nil, err
		}
		v, ok, err := lookup(doc, strings.TrimPrefix(head, "json"))
		if err != nil {
			return "", false, nil, err
		}
		if !ok {
			return "", false, toks[1:], nil
		}
		return render(v), true, toks[1:], nil
	}
	return "", false, nil, fmt.Errorf("unknown subject %q (use status, time, size, body, header, json)", head)
}

func (c *evalCtx) assert(line string, variables map[string]string) (bool, string) {
	toks, err := tokenize(line)
	if err != nil {
		return false, err.Error()
	}
	actual, exists, rest, err := c.subject(toks)
	if err != nil {
		return false, err.Error()
	}
	if len(rest) == 0 {
		return false, "missing operator"
	}
	op := rest[0]
	expected := ""
	if len(rest) > 1 {
		expected, _ = vars.Substitute(strings.Join(rest[1:], " "), variables)
	}

	switch op {
	case "exists":
		if exists {
			return true, ""
		}
		return false, "does not exist"
	case "!exists":
		if !exists {
			return true, ""
		}
		return false, "exists"
	}
	if len(rest) < 2 {
		return false, fmt.Sprintf("operator %q needs a value", op)
	}
	if !exists {
		return false, "does not exist"
	}

	switch op {
	case "==", "!=":
		eq := equal(actual, expected)
		if eq == (op == "==") {
			return true, ""
		}
		return false, fmt.Sprintf("got %s", abbreviate(actual))
	case "<", "<=", ">", ">=":
		a, errA := strconv.ParseFloat(actual, 64)
		e, errE := strconv.ParseFloat(expected, 64)
		if errA != nil || errE != nil {
			return false, fmt.Sprintf("cannot compare %s and %s as numbers", abbreviate(actual), abbreviate(expected))
		}
		ok := map[string]bool{"<": a < e, "<=": a <= e, ">": a > e, ">=": a >= e}[op]
		if ok {
			return true, ""
		}
		return false, fmt.Sprintf("got %s", abbreviate(actual))
	case "contains", "!contains":
		if strings.Contains(actual, expected) == (op == "contains") {
			return true, ""
		}
		return false, fmt.Sprintf("got %s", abbreviate(actual))
	case "matches", "!matches":
		re, err := regexp.Compile(expected)
		if err != nil {
			return false, fmt.Sprintf("bad regexp: %v", err)
		}
		if re.MatchString(actual) == (op == "matches") {
			return true, ""
		}
		return false, fmt.Sprintf("got %s", abbreviate(actual))
	}
	return false, fmt.Sprintf("unknown operator %q", op)
}

// equal compares numerically when both sides are numbers, otherwise as text.
func equal(a, b string) bool {
	if a == b {
		return true
	}
	fa, errA := strconv.ParseFloat(a, 64)
	fb, errB := strconv.ParseFloat(b, 64)
	return errA == nil && errB == nil && fa == fb
}

// lookup walks a path like ".data.items[0].id" (or ".items.0.id").
func lookup(doc any, path string) (any, bool, error) {
	cur := doc
	for path != "" {
		var key string
		index := -1
		switch path[0] {
		case '.':
			path = path[1:]
			end := strings.IndexAny(path, ".[")
			if end < 0 {
				end = len(path)
			}
			key, path = path[:end], path[end:]
			if key == "" {
				return nil, false, fmt.Errorf("empty key in JSON path")
			}
		case '[':
			end := strings.IndexByte(path, ']')
			if end < 0 {
				return nil, false, fmt.Errorf("missing ] in JSON path")
			}
			inner := strings.Trim(path[1:end], `"'`)
			path = path[end+1:]
			if n, err := strconv.Atoi(inner); err == nil {
				index = n
			} else {
				key = inner
			}
		default:
			return nil, false, fmt.Errorf("bad JSON path near %q", path)
		}

		switch v := cur.(type) {
		case map[string]any:
			if index >= 0 {
				key = strconv.Itoa(index)
			}
			next, ok := v[key]
			if !ok {
				return nil, false, nil
			}
			cur = next
		case []any:
			if index < 0 {
				if key == "length" && path == "" {
					return json.Number(strconv.Itoa(len(v))), true, nil
				}
				n, err := strconv.Atoi(key)
				if err != nil {
					return nil, false, nil
				}
				index = n
			}
			if index < 0 || index >= len(v) {
				return nil, false, nil
			}
			cur = v[index]
		default:
			return nil, false, nil
		}
	}
	return cur, true, nil
}

func render(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// tokenize splits on whitespace. A token that starts with a "double" or
// 'single' quote runs to the matching quote, which is removed; quotes inside
// a token (as in {"k":"v"}) are kept literally.
func tokenize(s string) ([]string, error) {
	var toks []string
	var cur strings.Builder
	inTok := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case (r == '"' || r == '\'') && !inTok:
			quote = r
			inTok = true
		case r == ' ' || r == '\t':
			if inTok {
				toks = append(toks, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inTok {
		toks = append(toks, cur.String())
	}
	return toks, nil
}

func hasWord(line, word string) bool {
	return strings.HasPrefix(line, word) && len(line) > len(word) && (line[len(word)] == ' ' || line[len(word)] == '\t')
}

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

func validName(s string) bool { return nameRe.MatchString(s) }

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func abbreviate(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 80 {
		return strconv.Quote(s[:77] + "...")
	}
	return strconv.Quote(s)
}
