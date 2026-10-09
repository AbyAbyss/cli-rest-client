package codegen

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
	"github.com/AbyAbyss/cli-rest-client/internal/vars"
)

// Part is a piece of a string: literal text, or a reference to a variable.
type Part struct {
	Lit string
	Var string // variable name when this part is a reference
}

// Str is a string that may contain variable references. In resolved mode
// it is always plain text.
type Str []Part

func lit(s string) Str {
	if s == "" {
		return nil
	}
	return Str{{Lit: s}}
}

// Plain returns the text when s has no variable references.
func (s Str) Plain() (string, bool) {
	var sb strings.Builder
	for _, p := range s {
		if p.Var != "" {
			return "", false
		}
		sb.WriteString(p.Lit)
	}
	return sb.String(), true
}

func concat(parts ...Str) Str {
	var out Str
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Header is one request header.
type Header struct{ Name, Value Str }

// Field is one form field.
type Field struct{ Key, Value Str }

// Variable is a variable a template snippet declares at the top.
type Variable struct {
	Name  string // the name in the workspace, such as baseUrl
	Value string
	// Bare is set when the variable stands for a whole JSON value
	// ({"amount": {{amount}}}); InString when it is used inside text.
	// Only a variable that is bare and never inside text is declared as a
	// JSON literal (a number, true, ...) rather than a string.
	Bare, InString bool
}

// NodeKind is the type of a JSON value.
type NodeKind int

// JSON value kinds. NodeBare is a {{variable}} used as a whole value.
const (
	NodeObject NodeKind = iota
	NodeArray
	NodeString
	NodeNumber
	NodeBool
	NodeNull
	NodeBare
)

// Node is a JSON value that keeps object keys in their original order.
type Node struct {
	Kind    NodeKind
	Str     Str    // NodeString
	Raw     string // NodeNumber and NodeBool text, NodeBare variable name
	Members []Member
	Items   []*Node
}

// Member is one key of a JSON object.
type Member struct {
	Key Str
	Val *Node
}

// Body kinds.
const (
	BodyNone = iota
	BodyJSON
	BodyForm
	BodyRaw
)

// Spec is a request in a form every language generator understands.
type Spec struct {
	Method string
	// URL is the address as typed; Query holds the Params tab (and a query
	// API key), which the app URL-encodes, so generators encode them too.
	URL     Str
	Query   []Field
	Headers []Header
	// Basic is set for Basic auth, so generators can use their language's
	// own helper instead of a hand-built Authorization header.
	Basic *Basic
	// ContentType is the type the app adds when the request has a body and
	// no Content-Type header of its own. Empty otherwise.
	ContentType string
	BodyKind    int
	Raw         Str // the body text (JSON, text, XML)
	JSON        *Node
	Form        []Field
	// Vars are the variables a template snippet uses, in order of first use.
	Vars []Variable
	// Missing lists variables with no value.
	Missing []string
}

// Basic holds Basic auth credentials.
type Basic struct{ User, Pass Str }

type builder struct {
	template bool
	rawBody  bool // building the body text, where JSON use is decided by parsing
	values   map[string]string
	index    map[string]int
	spec     *Spec
}

// build turns r into a Spec. The pre-request script runs on a copy of
// values first, as in the app, so nothing is saved.
func build(r models.Request, values map[string]string, template bool) (*Spec, error) {
	r.Normalize()
	v := make(map[string]string, len(values))
	for k, x := range values {
		v[k] = x
	}
	if strings.TrimSpace(r.PreRequest) != "" {
		script.RunPre(r.PreRequest, v)
	}
	// Prepare validates the request exactly as Send would and gives the
	// resolved URL and body.
	p, err := engine.Prepare(r, v)
	if err != nil {
		return nil, err
	}

	s := &Spec{Method: p.Request.Method, Missing: p.Missing}
	b := &builder{template: template, values: v, index: map[string]int{}, spec: s}

	if template {
		u := strings.TrimSpace(r.URL)
		if !strings.Contains(u, "://") && !strings.HasPrefix(u, "{{") {
			u = "http://" + u
		}
		s.URL = b.str(u)
	} else {
		u := strings.TrimSpace(b.resolve(r.URL))
		if !strings.Contains(u, "://") {
			u = "http://" + u
		}
		s.URL = lit(u)
	}
	for _, kv := range r.Params {
		if !kv.Disabled && kv.Key != "" {
			s.Query = append(s.Query, Field{b.str(kv.Key), b.str(kv.Value)})
		}
	}
	if r.Auth.Type == models.AuthAPIKey && r.Auth.In == "query" {
		s.Query = append(s.Query, Field{b.str(r.Auth.Key), b.str(r.Auth.Value)})
	}

	authHeader := ""
	switch r.Auth.Type {
	case models.AuthBasic, models.AuthBearer:
		authHeader = "authorization"
	case models.AuthAPIKey:
		if r.Auth.In != "query" {
			authHeader = strings.ToLower(b.resolve(r.Auth.Key))
		}
	}
	hasCT := false
	for _, h := range r.Headers {
		if h.Disabled || h.Key == "" {
			continue
		}
		name := strings.ToLower(b.resolve(h.Key))
		if authHeader != "" && name == authHeader {
			continue // auth replaces it, as in the app
		}
		if name == "content-type" {
			hasCT = true
		}
		s.Headers = append(s.Headers, Header{b.str(h.Key), b.str(h.Value)})
	}
	switch r.Auth.Type {
	case models.AuthBasic:
		s.Basic = &Basic{b.str(r.Auth.Username), b.str(r.Auth.Password)}
	case models.AuthBearer:
		s.Headers = append(s.Headers, Header{lit("Authorization"), concat(lit("Bearer "), b.str(r.Auth.Token))})
	case models.AuthAPIKey:
		if r.Auth.In != "query" {
			s.Headers = append(s.Headers, Header{b.str(r.Auth.Key), b.str(r.Auth.Value)})
		}
	}

	gqlGet := r.BodyType == models.BodyGraphQL && s.Method == "GET"
	if ct := contentTypes[r.BodyType]; ct != "" && !hasCT && !gqlGet {
		s.ContentType = ct
	}
	switch r.BodyType {
	case models.BodyGraphQL:
		b.graphQL(r, string(p.Body), gqlGet)
	case models.BodyJSON:
		if strings.TrimSpace(r.Body) == "" {
			break
		}
		s.BodyKind = BodyJSON
		if template {
			b.rawBody = true
			s.Raw = b.str(r.Body)
			b.rawBody = false
			s.JSON = b.parseTemplateJSON(r.Body)
		} else {
			s.Raw = lit(string(p.Body))
			s.JSON = b.parseJSON(string(p.Body))
		}
		if s.JSON == nil {
			s.BodyKind = BodyRaw
		}
	case models.BodyText, models.BodyXML:
		if r.Body != "" {
			s.BodyKind = BodyRaw
			if template {
				s.Raw = b.str(r.Body)
			} else {
				s.Raw = lit(string(p.Body))
			}
		}
	case models.BodyForm:
		for _, kv := range models.ParseKV(r.Body, "=") {
			if kv.Disabled || kv.Key == "" {
				continue
			}
			s.Form = append(s.Form, Field{b.str(kv.Key), b.str(kv.Value)})
		}
		if len(s.Form) > 0 {
			s.BodyKind = BodyForm
		}
	}
	return s, nil
}

var contentTypes = map[string]string{
	models.BodyJSON:    "application/json",
	models.BodyGraphQL: "application/json",
	models.BodyText:    "text/plain; charset=utf-8",
	models.BodyXML:     "application/xml",
	models.BodyForm:    "application/x-www-form-urlencoded",
}

// graphQL fills in a GraphQL request: a JSON body {"query", "variables"}
// for POST, or query and variables parameters for GET, as engine.Prepare
// sends them. body is the body Prepare built (resolved mode).
func (b *builder) graphQL(r models.Request, body string, get bool) {
	s := b.spec
	varsText := strings.TrimSpace(r.GraphQLVariables)
	if get {
		s.Query = append(s.Query, Field{lit("query"), b.str(r.Body)})
		if varsText != "" {
			if b.template {
				v := b.str(varsText)
				if n := b.parseTemplateJSON(varsText); n != nil {
					v = compactJSON(n) // compacted like Prepare does, variables kept
				}
				s.Query = append(s.Query, Field{lit("variables"), v})
			} else {
				var buf bytes.Buffer
				_ = json.Compact(&buf, []byte(b.resolve(varsText)))
				s.Query = append(s.Query, Field{lit("variables"), lit(buf.String())})
			}
		}
		return
	}
	s.BodyKind = BodyJSON
	if !b.template {
		s.Raw = lit(body)
		s.JSON = b.parseJSON(body)
		return
	}
	obj := &Node{Kind: NodeObject, Members: []Member{{lit("query"), &Node{Kind: NodeString, Str: b.str(r.Body)}}}}
	if varsText != "" {
		vars := b.parseTemplateJSON(varsText)
		if vars == nil {
			vars = &Node{Kind: NodeObject} // Prepare already rejected invalid variables
		}
		obj.Members = append(obj.Members, Member{lit("variables"), vars})
	}
	s.JSON = obj
	s.Raw = compactJSON(obj)
}

// resolve substitutes variables, for decisions that need the final text.
func (b *builder) resolve(s string) string {
	out, _ := vars.Substitute(s, b.values)
	return out
}

// str converts text to a Str: substituted in resolved mode, split into
// literal and variable parts in template mode.
func (b *builder) str(s string) Str {
	if !b.template {
		return lit(b.resolve(s))
	}
	var out Str
	last := 0
	for _, m := range vars.Pattern.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > last {
			out = append(out, Part{Lit: s[last:m[0]]})
		}
		name := s[m[2]:m[3]]
		if !b.rawBody {
			b.use(name, false)
		} else {
			b.declare(name)
		}
		out = append(out, Part{Var: name})
		last = m[1]
	}
	if last < len(s) {
		out = append(out, Part{Lit: s[last:]})
	}
	return out
}

// use records a variable reference for the declarations at the top.
func (b *builder) use(name string, bare bool) {
	v := &b.spec.Vars[b.declare(name)]
	if bare {
		v.Bare = true
	} else {
		v.InString = true
	}
}

// declare adds a variable to the declarations, keeping first-use order.
func (b *builder) declare(name string) int {
	if i, ok := b.index[name]; ok {
		return i
	}
	value, missing := vars.Substitute("{{"+name+"}}", b.values)
	if len(missing) > 0 {
		value = ""
	}
	b.index[name] = len(b.spec.Vars)
	b.spec.Vars = append(b.spec.Vars, Variable{Name: name, Value: value})
	return b.index[name]
}

const bareMark = "\x00bare\x00"

// parseTemplateJSON parses a JSON body that may use {{variables}} as whole
// values ({"n": {{count}}}), which plain JSON doesn't allow.
func (b *builder) parseTemplateJSON(text string) *Node {
	// Wrap variables outside strings in quotes with a marker, so the text
	// parses; the marked strings become NodeBare.
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
		if c == '"' {
			inString = true
		}
		if c == '{' {
			if loc := vars.Pattern.FindStringSubmatchIndex(text[i:]); loc != nil && loc[0] == 0 {
				// JSON text can't hold a raw NUL, so write it escaped.
				sb.WriteString(`"\u0000bare\u0000` + text[i+loc[2]:i+loc[3]] + `"`)
				i += loc[1] - 1
				continue
			}
		}
		sb.WriteByte(c)
	}
	return b.parseJSON(sb.String())
}

func (b *builder) parseJSON(text string) *Node {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	n, err := b.readNode(dec)
	if err != nil {
		return nil
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil // trailing data
	}
	return n
}

var errJSON = errors.New("invalid JSON")

func (b *builder) readNode(dec *json.Decoder) (*Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			n := &Node{Kind: NodeObject}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, errJSON
				}
				val, err := b.readNode(dec)
				if err != nil {
					return nil, err
				}
				n.Members = append(n.Members, Member{b.jsonStr(key), val})
			}
			_, err := dec.Token() // }
			return n, err
		case '[':
			n := &Node{Kind: NodeArray}
			for dec.More() {
				item, err := b.readNode(dec)
				if err != nil {
					return nil, err
				}
				n.Items = append(n.Items, item)
			}
			_, err := dec.Token() // ]
			return n, err
		}
		return nil, errJSON
	case string:
		if strings.HasPrefix(t, bareMark) {
			name := strings.TrimPrefix(t, bareMark)
			b.use(name, true)
			return &Node{Kind: NodeBare, Raw: name}, nil
		}
		return &Node{Kind: NodeString, Str: b.jsonStr(t)}, nil
	case json.Number:
		return &Node{Kind: NodeNumber, Raw: t.String()}, nil
	case bool:
		if t {
			return &Node{Kind: NodeBool, Raw: "true"}, nil
		}
		return &Node{Kind: NodeBool, Raw: "false"}, nil
	case nil:
		return &Node{Kind: NodeNull}, nil
	}
	return nil, errJSON
}

// jsonStr turns a decoded JSON string into a Str. In resolved mode the
// text is already substituted, so it stays literal.
func (b *builder) jsonStr(s string) Str {
	if !b.template {
		return lit(s)
	}
	return b.str(s)
}

// quoteJSON quotes s as a JSON string without escaping <, > and &.
func quoteJSON(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}
