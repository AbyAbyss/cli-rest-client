package curl

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
)

// Command is a request rendered as a curl command line.
type Command struct {
	Text string
	// Missing lists {{variables}} that had no value (resolved mode only).
	Missing []string
}

// Resolved renders r with variables filled in, exactly as Send would: the
// pre-request script runs first, on a copy of vars, so nothing is saved.
func Resolved(r models.Request, vars map[string]string) (*Command, error) {
	v := make(map[string]string, len(vars))
	for k, x := range vars {
		v[k] = x
	}
	if strings.TrimSpace(r.PreRequest) != "" {
		script.RunPre(r.PreRequest, v)
	}
	p, err := engine.Prepare(r, v)
	if err != nil {
		return nil, err
	}
	return &Command{Text: p.Curl(), Missing: p.Missing}, nil
}

// Template renders r with {{variables}} kept as they are, for sharing or
// docs. Pasting it back into the app (or importing it) restores them.
func Template(r models.Request) *Command {
	r.Normalize()
	q := shellQuote
	var lines []string

	full := strings.TrimSpace(r.URL)
	var query []string
	for _, p := range r.Params {
		if p.Disabled || p.Key == "" {
			continue
		}
		query = append(query, escapeQuery(p.Key)+"="+escapeQuery(p.Value))
	}
	if r.Auth.Type == models.AuthAPIKey && r.Auth.In == "query" && r.Auth.Key != "" {
		query = append(query, escapeQuery(r.Auth.Key)+"="+escapeQuery(r.Auth.Value))
	}
	gqlGet := r.BodyType == models.BodyGraphQL && r.Method == "GET"
	gqlVars := compactTemplate(r.GraphQLVariables)
	if gqlGet {
		query = append(query, "query="+escapeQuery(r.Body))
		if gqlVars != "" {
			query = append(query, "variables="+escapeQuery(gqlVars))
		}
	}
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(full, "?") {
			sep = "&"
		}
		full += sep + strings.Join(query, "&")
	}

	first := "curl"
	switch r.Method {
	case "GET":
	case "HEAD":
		first += " -I"
	default:
		first += " -X " + r.Method
	}
	lines = append(lines, first+" "+q(full))

	hasCT := false
	for _, h := range r.Headers {
		if h.Disabled || h.Key == "" {
			continue
		}
		if strings.EqualFold(h.Key, "Content-Type") {
			hasCT = true
		}
		lines = append(lines, "-H "+q(h.Key+": "+h.Value))
	}
	switch r.Auth.Type {
	case models.AuthBasic:
		lines = append(lines, "-u "+q(r.Auth.Username+":"+r.Auth.Password))
	case models.AuthBearer:
		lines = append(lines, "-H "+q("Authorization: Bearer "+r.Auth.Token))
	case models.AuthAPIKey:
		if r.Auth.In != "query" && r.Auth.Key != "" {
			lines = append(lines, "-H "+q(r.Auth.Key+": "+r.Auth.Value))
		}
	}

	ct := map[string]string{
		models.BodyJSON: "application/json",
		models.BodyText: "text/plain; charset=utf-8",
		models.BodyXML:  "application/xml",
		models.BodyForm: "application/x-www-form-urlencoded",
	}[r.BodyType]
	if r.BodyType == models.BodyGraphQL && !gqlGet {
		ct = "application/json"
	}
	if ct != "" && !hasCT {
		lines = append(lines, "-H "+q("Content-Type: "+ct))
	}
	switch r.BodyType {
	case models.BodyJSON, models.BodyText, models.BodyXML:
		if r.Body != "" {
			lines = append(lines, "--data-raw "+q(r.Body))
		}
	case models.BodyGraphQL:
		if !gqlGet {
			body := `{"query":` + quoteJSON(r.Body)
			if gqlVars != "" {
				body += `,"variables":` + gqlVars
			}
			lines = append(lines, "--data-raw "+q(body+"}"))
		}
	case models.BodyForm:
		var parts []string
		for _, kv := range models.ParseKV(r.Body, "=") {
			if kv.Disabled || kv.Key == "" {
				continue
			}
			parts = append(parts, escapeQuery(kv.Key)+"="+escapeQuery(kv.Value))
		}
		if len(parts) > 0 {
			lines = append(lines, "--data-raw "+q(strings.Join(parts, "&")))
		}
	}
	return &Command{Text: strings.Join(lines, " \\\n  ")}
}

// escapeQuery URL-encodes a query part but keeps {{variables}} readable.
func escapeQuery(s string) string {
	var sb strings.Builder
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			sb.WriteString(url.QueryEscape(s))
			return sb.String()
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			sb.WriteString(url.QueryEscape(s))
			return sb.String()
		}
		sb.WriteString(url.QueryEscape(s[:i]))
		sb.WriteString(s[i : i+j+2])
		s = s[i+j+2:]
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// quoteJSON quotes s as a JSON string, leaving {{variables}} readable.
func quoteJSON(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

// compactTemplate removes insignificant whitespace from JSON that may hold
// bare {{variables}} ({"n": {{count}}}), which json.Compact would reject.
func compactTemplate(text string) string {
	text = strings.TrimSpace(text)
	var sb strings.Builder
	inString, escaped := false, false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inString {
			sb.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		case '"':
			inString = true
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
