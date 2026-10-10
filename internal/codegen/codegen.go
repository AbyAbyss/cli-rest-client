// Package codegen renders a saved request as code: curl, Python
// (requests), JavaScript (fetch), Go (net/http) and HTTPie.
package codegen

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/AbyAbyss/cli-rest-client/internal/curl"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// Language is a snippet target.
type Language struct {
	ID   string // used on the command line and in settings
	Name string // shown in the UI
}

// Languages lists the targets in the order the UI cycles through them.
var Languages = []Language{
	{"curl", "cURL"},
	{"python", "Python"},
	{"javascript", "JavaScript"},
	{"go", "Go"},
	{"httpie", "HTTPie"},
}

var aliases = map[string]string{
	"py": "python", "requests": "python",
	"js": "javascript", "fetch": "javascript", "node": "javascript",
	"golang": "go",
	"http":   "httpie",
}

// Lookup finds a language by ID, name or a common alias, ignoring case.
func Lookup(name string) (Language, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if id, ok := aliases[name]; ok {
		name = id
	}
	for _, l := range Languages {
		if l.ID == name || strings.ToLower(l.Name) == name {
			return l, true
		}
	}
	return Language{}, false
}

// Snippet is generated code.
type Snippet struct {
	Text string
	// Missing lists variables with no value. In resolved mode they stay
	// as {{name}}; in template mode they are declared empty.
	Missing []string
}

// Generate renders r in the given language. With template false, values
// fill in the variables (after running the pre-request script on a copy,
// as Send would). With template true, variables become named variables
// at the top of the snippet, set to their current values.
func Generate(lang string, r models.Request, values map[string]string, template bool) (*Snippet, error) {
	l, ok := Lookup(lang)
	if !ok {
		return nil, fmt.Errorf("unknown language %q (use %s)", lang, IDs())
	}
	if r.Type == models.TypeWebSocket {
		return generateWS(l.ID, r, values, template)
	}
	if l.ID == "curl" {
		if template {
			c := curl.Template(r)
			return &Snippet{Text: c.Text}, nil
		}
		c, err := curl.Resolved(r, values)
		if err != nil {
			return nil, err
		}
		return &Snippet{Text: c.Text, Missing: c.Missing}, nil
	}
	s, err := build(r, values, template)
	if err != nil {
		return nil, err
	}
	var text string
	switch l.ID {
	case "python":
		text = python(s)
	case "javascript":
		text = javascript(s)
	case "go":
		text = golang(s)
	case "httpie":
		text = httpie(s)
	}
	return &Snippet{Text: text, Missing: s.Missing}, nil
}

// IDs returns the language IDs, for help text.
func IDs() string {
	ids := make([]string, len(Languages))
	for i, l := range Languages {
		ids[i] = l.ID
	}
	return strings.Join(ids, ", ")
}

// ---------- variable names ----------

// words splits a variable name like baseUrl, api_key or $uuid into
// lower-case words.
func words(name string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(name)
	for i, c := range rs {
		switch {
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			flush()
			continue
		case unicode.IsUpper(c) && i > 0 && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1])):
			flush()
		}
		cur = append(cur, c)
	}
	flush()
	return out
}

type naming struct {
	join     func(w []string) string
	reserved map[string]bool
	suffix   string
}

func snake(w []string) string { return strings.Join(w, "_") }

func camel(w []string) string {
	var sb strings.Builder
	for i, x := range w {
		if i > 0 {
			x = strings.ToUpper(x[:1]) + x[1:]
		}
		sb.WriteString(x)
	}
	return sb.String()
}

func screaming(w []string) string { return strings.ToUpper(strings.Join(w, "_")) }

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// names maps each variable to an identifier that is valid in the target
// language and doesn't clash with keywords or the snippet's own names.
func (n naming) names(vs []Variable) map[string]string {
	out := map[string]string{}
	used := map[string]bool{}
	for _, v := range vs {
		w := words(v.Name)
		if len(w) == 0 {
			w = []string{"var"}
		}
		id := n.join(w)
		if unicode.IsDigit(rune(id[0])) {
			id = n.join(append([]string{"v"}, w...))
		}
		if n.reserved[id] {
			id += n.suffix
		}
		base := id
		for i := 2; used[id]; i++ {
			id = fmt.Sprintf("%s%d", base, i)
		}
		used[id] = true
		out[v.Name] = id
	}
	return out
}

// hasDuplicateKeys reports whether a form repeats a field name, in which
// case generators use a list of pairs instead of a dict or object.
func hasDuplicateKeys(fields []Field) bool {
	seen := map[string]bool{}
	for _, f := range fields {
		k, ok := f.Key.Plain()
		if !ok {
			continue
		}
		if seen[k] {
			return true
		}
		seen[k] = true
	}
	return false
}

// literal reports whether v is declared as a JSON literal rather than a
// string (see Variable.Bare).
func literal(v Variable) bool {
	if !v.Bare || v.InString {
		return false
	}
	b := &builder{values: map[string]string{}}
	return b.parseJSON(v.Value) != nil
}

// parsedVars lists the variables that are used as whole JSON values but
// declared as strings, so the snippet has to parse them.
func parsedVars(vs []Variable) map[string]bool {
	out := map[string]bool{}
	for _, v := range vs {
		if v.Bare && !literal(v) {
			out[v.Name] = true
		}
	}
	return out
}

// mergeHeaders joins repeated headers into one comma-separated value
// (equivalent in HTTP), for languages whose header maps can't repeat a key.
func mergeHeaders(hs []Header) []Header {
	var out []Header
	at := map[string]int{}
	for _, h := range hs {
		name, ok := h.Name.Plain()
		key := strings.ToLower(name)
		if i, seen := at[key]; ok && seen {
			out[i].Value = concat(out[i].Value, lit(", "), h.Value)
			continue
		}
		if ok {
			at[key] = len(out)
		}
		out = append(out, Header{h.Name, append(Str(nil), h.Value...)})
	}
	return out
}
