// Package curl turns curl command lines (as copied from browser dev tools,
// Postman, API docs or this app's own cURL view) into requests.
package curl

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// Result is one parsed curl command.
type Result struct {
	Request models.Request
	// Notes lists options that were ignored or changed meaning.
	Notes []string
}

// LooksLikeCurl reports whether text starts with a curl command.
func LooksLikeCurl(text string) bool {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "$ ")
	return strings.HasPrefix(t, "curl ") || strings.HasPrefix(t, "curl\t") || strings.HasPrefix(t, "curl\\") || strings.HasPrefix(t, "curl\n")
}

// ParseAll parses every curl command in text. Commands can be on separate
// lines or joined with ;, && or ||; pipes into other programs (| jq) are
// ignored.
func ParseAll(text string) ([]*Result, error) {
	words, err := shellSplit(text)
	if err != nil {
		return nil, err
	}
	var cmds [][]string
	var cur []string
	inCurl, skipping := false, false
	for _, w := range words {
		switch {
		case w.op != "":
			if inCurl && len(cur) > 0 {
				cmds = append(cmds, cur)
			}
			cur, inCurl = nil, false
			skipping = w.op == "|"
		case !inCurl && !w.quoted && (w.text == "curl" || strings.HasSuffix(w.text, "/curl")):
			inCurl, skipping = true, false
			cur = nil
		case inCurl && !skipping:
			cur = append(cur, w.text)
		}
	}
	if inCurl {
		cmds = append(cmds, cur)
	}
	if len(cmds) == 0 {
		return nil, errors.New("no curl command found")
	}
	var out []*Result
	for _, args := range cmds {
		r, err := parseArgs(args)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Parse parses a single curl command.
func Parse(text string) (*Result, error) {
	all, err := ParseAll(text)
	if err != nil {
		return nil, err
	}
	if len(all) > 1 {
		return nil, fmt.Errorf("found %d curl commands, expected one", len(all))
	}
	return all[0], nil
}

// Options that take a value but don't affect the request.
var ignoredWithValue = map[string]bool{
	"-o": true, "--output": true, "-m": true, "--max-time": true, "--connect-timeout": true,
	"-w": true, "--write-out": true, "--retry": true, "--retry-delay": true, "--retry-max-time": true,
	"-x": true, "--proxy": true, "-U": true, "--proxy-user": true, "--cacert": true, "--capath": true,
	"-E": true, "--cert": true, "--key": true, "--cert-type": true, "--key-type": true,
	"--resolve": true, "--connect-to": true, "--interface": true, "-c": true, "--cookie-jar": true,
	"-D": true, "--dump-header": true, "--limit-rate": true, "-r": true, "--range": true,
	"--max-redirs": true, "-T": true, "--upload-file": true, "-K": true, "--config": true,
	"--trace": true, "--trace-ascii": true, "--stderr": true, "-y": true, "-Y": true,
}

// Short options that take a value (for splitting combined flags like -sSXPOST).
const shortWithValue = "XHduFAebomwxUEcDrTKyY"

func parseArgs(args []string) (*Result, error) {
	res := &Result{}
	r := models.NewRequest("")
	method := ""
	var rawURL string
	var data []string // -d style pieces, joined with &
	var form []models.KeyValue
	jsonBody := false
	getMode := false
	note := func(format string, a ...any) { res.Notes = append(res.Notes, fmt.Sprintf(format, a...)) }

	// Expand combined short flags: -sSL -> -s -S -L, -XPOST -> -X POST.
	var expanded []string
	for _, a := range args {
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			for i := 1; i < len(a); i++ {
				c := a[i]
				if strings.IndexByte(shortWithValue, c) >= 0 {
					expanded = append(expanded, "-"+string(c))
					if i+1 < len(a) {
						expanded = append(expanded, a[i+1:])
					}
					break
				}
				expanded = append(expanded, "-"+string(c))
			}
			continue
		}
		expanded = append(expanded, a)
	}
	args = expanded

	for i := 0; i < len(args); i++ {
		a := args[i]
		val := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("option %s needs a value", a)
			}
			i++
			return args[i], nil
		}
		// --opt=value is not curl syntax, but tools sometimes emit it.
		if strings.HasPrefix(a, "--") && strings.Contains(a, "=") {
			name, v, _ := strings.Cut(a, "=")
			args = append(args[:i+1], append([]string{v}, args[i+1:]...)...)
			a = name
		}
		switch a {
		case "-X", "--request":
			v, err := val()
			if err != nil {
				return nil, err
			}
			method = strings.ToUpper(v)
		case "-H", "--header":
			v, err := val()
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(v, "@") {
				note("headers from a file (%s) were not read", v)
				continue
			}
			k, hv, ok := strings.Cut(v, ":")
			if !ok {
				// "Name;" sends an empty header; "Name" alone removes one.
				if strings.HasSuffix(v, ";") {
					r.Headers = append(r.Headers, models.KeyValue{Key: strings.TrimSuffix(v, ";")})
				}
				continue
			}
			r.Headers = append(r.Headers, models.KeyValue{Key: strings.TrimSpace(k), Value: strings.TrimSpace(hv)})
		case "-d", "--data", "--data-ascii", "--data-raw", "--data-binary":
			v, err := val()
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(v, "@") && a != "--data-raw" {
				note("body from a file (%s) was not read; paste the content into the Body tab", v)
				continue
			}
			if a == "-d" || a == "--data" || a == "--data-ascii" {
				v = strings.NewReplacer("\r", "", "\n", "").Replace(v) // curl strips newlines here
			}
			data = append(data, v)
		case "--data-urlencode":
			v, err := val()
			if err != nil {
				return nil, err
			}
			if name, content, ok := strings.Cut(v, "="); ok {
				data = append(data, url.QueryEscape(name)+"="+url.QueryEscape(content))
			} else {
				data = append(data, url.QueryEscape(v))
			}
		case "--json":
			v, err := val()
			if err != nil {
				return nil, err
			}
			data = append(data, v)
			jsonBody = true
		case "-F", "--form", "--form-string":
			v, err := val()
			if err != nil {
				return nil, err
			}
			k, fv, _ := strings.Cut(v, "=")
			if a != "--form-string" && (strings.HasPrefix(fv, "@") || strings.HasPrefix(fv, "<")) {
				note("form file field %q was skipped (file uploads are not supported)", k)
				continue
			}
			form = append(form, models.KeyValue{Key: k, Value: fv})
		case "-u", "--user":
			v, err := val()
			if err != nil {
				return nil, err
			}
			user, pass, _ := strings.Cut(v, ":")
			r.Auth = models.Auth{Type: models.AuthBasic, Username: user, Password: pass, In: "header"}
		case "--oauth2-bearer":
			v, err := val()
			if err != nil {
				return nil, err
			}
			r.Auth = models.Auth{Type: models.AuthBearer, Token: v, In: "header"}
		case "-A", "--user-agent":
			v, err := val()
			if err != nil {
				return nil, err
			}
			r.Headers = append(r.Headers, models.KeyValue{Key: "User-Agent", Value: v})
		case "-e", "--referer":
			v, err := val()
			if err != nil {
				return nil, err
			}
			r.Headers = append(r.Headers, models.KeyValue{Key: "Referer", Value: v})
		case "-b", "--cookie":
			v, err := val()
			if err != nil {
				return nil, err
			}
			if !strings.Contains(v, "=") {
				note("cookies from a file (%s) were not read", v)
				continue
			}
			r.Headers = append(r.Headers, models.KeyValue{Key: "Cookie", Value: v})
		case "--url":
			v, err := val()
			if err != nil {
				return nil, err
			}
			rawURL = v
		case "-G", "--get":
			getMode = true
		case "-I", "--head":
			method = "HEAD"
		case "-k", "--insecure":
			note("-k (skip TLS verification) is a setting here: Settings → Verify TLS certs")
		case "-L", "--location", "--compressed", "-s", "--silent", "-S", "--show-error", "-v", "--verbose",
			"-i", "--include", "-f", "--fail", "--fail-with-body", "-g", "--globoff", "-N", "--no-buffer",
			"--http1.1", "--http2", "--http2-prior-knowledge", "--http1.0", "-#", "--progress-bar",
			"-q", "--disable", "--no-progress-meter", "-n", "--netrc", "--location-trusted", "-O", "--remote-name",
			"--tlsv1.2", "--tlsv1.3", "-4", "-6", "--ipv4", "--ipv6", "--raw", "--path-as-is":
			// No effect on the request itself.
		default:
			switch {
			case ignoredWithValue[a]:
				if _, err := val(); err != nil {
					return nil, err
				}
			case strings.HasPrefix(a, "-") && len(a) > 1:
				note("option %s was ignored", a)
			case rawURL == "":
				rawURL = a
			default:
				note("extra argument %q was ignored", a)
			}
		}
	}
	if rawURL == "" {
		return nil, errors.New("the curl command has no URL")
	}

	// URL: query string goes to Params.
	base, query, _ := strings.Cut(rawURL, "?")
	r.URL = base
	r.Params = append(r.Params, parsePairs(query, "&")...)

	body := strings.Join(data, "&")
	switch {
	case getMode && body != "":
		r.Params = append(r.Params, parsePairs(body, "&")...)
		body = ""
	case len(form) > 0:
		r.BodyType = models.BodyForm
		r.Body = models.FormatKV(form, "=")
		note("multipart form (-F) is sent as urlencoded form fields")
	}
	if method == "" {
		method = "GET"
		if body != "" || len(form) > 0 {
			method = "POST"
		}
	}
	r.Method = method
	if indexOf(models.Methods, r.Method) < 0 {
		note("method %s is not supported, imported as GET", r.Method)
		r.Method = "GET"
	}

	contentType := strings.ToLower(headerValue(r.Headers, "Content-Type"))
	if body != "" {
		trimmed := strings.TrimSpace(body)
		looksJSON := strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
		switch {
		case jsonBody || strings.Contains(contentType, "json"):
			r.BodyType = models.BodyJSON
			r.Body = body
		case strings.Contains(contentType, "xml"):
			r.BodyType = models.BodyXML
			r.Body = body
		case strings.Contains(contentType, "x-www-form-urlencoded") || (contentType == "" && !looksJSON && isPairs(body)):
			r.BodyType = models.BodyForm
			r.Body = models.FormatKV(parsePairs(body, "&"), "=")
		case contentType == "" && looksJSON:
			r.BodyType = models.BodyJSON
			r.Body = body
			note("the body is JSON but no Content-Type was given (curl would send application/x-www-form-urlencoded); imported as JSON")
		default:
			r.BodyType = models.BodyText
			r.Body = body
		}
	}
	if jsonBody {
		if headerValue(r.Headers, "Content-Type") == "" {
			r.Headers = append(r.Headers, models.KeyValue{Key: "Content-Type", Value: "application/json"})
		}
		if headerValue(r.Headers, "Accept") == "" {
			r.Headers = append(r.Headers, models.KeyValue{Key: "Accept", Value: "application/json"})
		}
	}

	// An Authorization header becomes Auth, so it shows in the Auth tab.
	if auth := headerValue(r.Headers, "Authorization"); auth != "" && r.Auth.Type == models.AuthNone {
		scheme, cred, _ := strings.Cut(auth, " ")
		switch strings.ToLower(scheme) {
		case "bearer":
			r.Auth = models.Auth{Type: models.AuthBearer, Token: strings.TrimSpace(cred), In: "header"}
			r.Headers = removeHeader(r.Headers, "Authorization")
		case "basic":
			if dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cred)); err == nil {
				user, pass, _ := strings.Cut(string(dec), ":")
				r.Auth = models.Auth{Type: models.AuthBasic, Username: user, Password: pass, In: "header"}
				r.Headers = removeHeader(r.Headers, "Authorization")
			}
		}
	}

	r.Name = requestName(r)
	r.Normalize()
	res.Request = r
	return res, nil
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

func headerValue(h []models.KeyValue, name string) string {
	for _, kv := range h {
		if strings.EqualFold(kv.Key, name) && !kv.Disabled {
			return kv.Value
		}
	}
	return ""
}

func removeHeader(h []models.KeyValue, name string) []models.KeyValue {
	out := h[:0]
	for _, kv := range h {
		if !strings.EqualFold(kv.Key, name) {
			out = append(out, kv)
		}
	}
	return out
}

// isPairs reports whether s looks like key=value&key=value.
func isPairs(s string) bool {
	if s == "" || strings.ContainsAny(s, "\n") {
		return false
	}
	for _, p := range strings.Split(s, "&") {
		k, _, ok := strings.Cut(p, "=")
		if !ok || k == "" || strings.ContainsAny(k, " {}[]\"") {
			return false
		}
	}
	return true
}

func parsePairs(s, sep string) []models.KeyValue {
	if s == "" {
		return nil
	}
	var out []models.KeyValue
	for _, p := range strings.Split(s, sep) {
		if p == "" {
			continue
		}
		k, v, _ := strings.Cut(p, "=")
		if uk, err := url.QueryUnescape(k); err == nil {
			k = uk
		}
		if uv, err := url.QueryUnescape(v); err == nil {
			v = uv
		}
		out = append(out, models.KeyValue{Key: k, Value: v})
	}
	return out
}

// requestName is "METHOD last-path-segment", e.g. "POST users".
func requestName(r models.Request) string {
	u := r.URL
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	parts := strings.Split(strings.Trim(u, "/"), "/")
	for i := len(parts) - 1; i >= 1; i-- {
		if p := strings.TrimSpace(parts[i]); p != "" {
			return r.Method + " " + p
		}
	}
	if len(parts) > 0 && parts[0] != "" {
		return r.Method + " " + parts[0]
	}
	return r.Method + " request"
}

// ---------- shell words ----------

type word struct {
	text   string
	quoted bool   // any part was quoted
	op     string // ";", "&&", "||", "|" or newline-separated command
}

// shellSplit splits text into words like a POSIX shell (bash flavour):
// '...' literal, "..." with \ escapes, $'...' ANSI-C quoting, \ escapes and
// \-newline continuations. Control operators become op words.
func shellSplit(s string) ([]word, error) {
	var words []word
	var cur strings.Builder
	inWord, quoted := false, false
	flush := func() {
		if inWord {
			words = append(words, word{text: cur.String(), quoted: quoted})
		}
		cur.Reset()
		inWord, quoted = false, false
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\':
			if i+1 >= len(rs) {
				continue
			}
			next := rs[i+1]
			if next == '\n' || next == '\r' {
				// Line continuation.
				i++
				if next == '\r' && i+1 < len(rs) && rs[i+1] == '\n' {
					i++
				}
				continue
			}
			// "\ " at the end of a line whose newline was lost (pasted
			// into a single-line field): treat as a continuation too.
			if next == ' ' && !inWord {
				continue
			}
			cur.WriteRune(next)
			inWord = true
			i++
		case c == '\'':
			end := indexRune(rs, '\'', i+1)
			if end < 0 {
				return nil, errors.New("unterminated ' quote")
			}
			cur.WriteString(string(rs[i+1 : end]))
			inWord, quoted = true, true
			i = end
		case c == '$' && i+1 < len(rs) && rs[i+1] == '\'':
			j := i + 2
			for ; j < len(rs) && rs[j] != '\''; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				}
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated $' quote")
			}
			cur.WriteString(ansiC(string(rs[i+2 : j])))
			inWord, quoted = true, true
			i = j
		case c == '"':
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) && strings.ContainsRune("\"\\$`\n", rs[j+1]) {
					if rs[j+1] != '\n' {
						cur.WriteRune(rs[j+1])
					}
					j++
					continue
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, errors.New(`unterminated " quote`)
			}
			inWord, quoted = true, true
			i = j
		case c == ' ' || c == '\t':
			flush()
		case c == '\n' || c == '\r' || c == ';':
			flush()
			words = append(words, word{op: ";"})
		case c == '&' && i+1 < len(rs) && rs[i+1] == '&', c == '|' && i+1 < len(rs) && rs[i+1] == '|':
			flush()
			words = append(words, word{op: string(c) + string(c)})
			i++
		case c == '|':
			flush()
			words = append(words, word{op: "|"})
		case c == '#' && !inWord:
			// Comment to end of line.
			for i+1 < len(rs) && rs[i+1] != '\n' {
				i++
			}
		default:
			cur.WriteRune(c)
			inWord = true
		}
	}
	flush()
	return words, nil
}

func indexRune(rs []rune, r rune, from int) int {
	for i := from; i < len(rs); i++ {
		if rs[i] == r {
			return i
		}
	}
	return -1
}

// ansiC decodes bash $'...' escapes.
func ansiC(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			sb.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case '\\', '\'', '"', '?':
			sb.WriteByte(s[i])
		case 'x':
			if i+2 < len(s) {
				var b byte
				if _, err := fmt.Sscanf(s[i+1:i+3], "%02x", &b); err == nil {
					sb.WriteByte(b)
					i += 2
					continue
				}
			}
			sb.WriteString(`\x`)
		case 'u':
			if i+4 < len(s) {
				var r rune
				if _, err := fmt.Sscanf(s[i+1:i+5], "%04x", &r); err == nil {
					sb.WriteRune(r)
					i += 4
					continue
				}
			}
			sb.WriteString(`\u`)
		default:
			sb.WriteByte('\\')
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}
