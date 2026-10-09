package codegen

import (
	"fmt"
	"strings"
)

var shNames = naming{
	join:   screaming,
	suffix: "_VAR",
	reserved: set("PATH", "HOME", "USER", "PWD", "SHELL", "IFS", "PS1", "PS2", "TERM", "LANG",
		"HOSTNAME", "UID", "RANDOM", "SECONDS", "LINENO", "OLDPWD", "TMPDIR", "EDITOR"),
}

func httpie(s *Spec) string {
	ids := shNames.names(s.Vars)
	arg := func(x Str) string { return shArg(x, ids) }
	var sb strings.Builder
	for _, v := range s.Vars {
		fmt.Fprintf(&sb, "%s=%s\n", ids[v.Name], shellQuote(v.Value))
	}
	if len(s.Vars) > 0 {
		sb.WriteString("\n")
	}

	first := []string{"http"}
	var items []string
	if s.Basic != nil {
		first = append(first, "-a", arg(concat(s.Basic.User, lit(":"), s.Basic.Pass)))
	}
	if s.BodyKind == BodyForm {
		first = append(first, "--form")
	}
	first = append(first, s.Method, arg(s.URL))

	header := func(h Header) {
		if len(h.Value) == 0 {
			items = append(items, arg(concat(h.Name, lit(";")))) // HTTPie's syntax for an empty header
			return
		}
		items = append(items, arg(concat(h.Name, lit(":"), h.Value)))
	}

	for _, q := range s.Query {
		items = append(items, arg(concat(escapeItemKey(q.Key), lit("=="), q.Value)))
	}

	var data []string
	raw := false
	switch s.BodyKind {
	case BodyJSON:
		if fields, ok := jsonItems(s.JSON); ok {
			for _, f := range fields {
				data = append(data, arg(f))
			}
		} else {
			raw = true
		}
	case BodyForm:
		for _, f := range s.Form {
			data = append(data, arg(concat(escapeItemKey(f.Key), lit("="), f.Value)))
		}
	case BodyRaw:
		raw = true
	}
	// HTTPie sets Content-Type itself for JSON and form items; a raw body
	// needs it spelled out.
	if s.ContentType != "" && raw {
		header(Header{lit("Content-Type"), lit(s.ContentType)})
	}
	for _, h := range s.Headers {
		header(h)
	}
	if raw {
		data = append(data, "--raw "+arg(s.Raw))
	}

	sb.WriteString(strings.Join(first, " "))
	for _, it := range append(items, data...) {
		sb.WriteString(" \\\n  " + it)
	}
	sb.WriteString("\n")
	return sb.String()
}

// jsonItems turns a JSON object body into HTTPie request items: key=value
// for strings and key:=json for everything else. It reports false when the
// body can't be expressed that way (not an object, or awkward keys).
func jsonItems(n *Node) ([]Str, bool) {
	if n == nil || n.Kind != NodeObject || len(n.Members) == 0 {
		return nil, false
	}
	var out []Str
	for _, m := range n.Members {
		k, ok := m.Key.Plain()
		if !ok || k == "" || strings.ContainsAny(k, `:=@\[]`) {
			return nil, false
		}
		if m.Val.Kind == NodeString {
			out = append(out, concat(lit(k+"="), m.Val.Str))
		} else {
			out = append(out, concat(lit(k+":="), compactJSON(m.Val)))
		}
	}
	return out, true
}

// compactJSON writes a node as one-line JSON, keeping variables as parts.
func compactJSON(n *Node) Str {
	switch n.Kind {
	case NodeObject:
		out := lit("{")
		for i, m := range n.Members {
			if i > 0 {
				out = append(out, Part{Lit: ","})
			}
			out = append(out, jsonString(m.Key)...)
			out = append(out, Part{Lit: ":"})
			out = append(out, compactJSON(m.Val)...)
		}
		return append(out, Part{Lit: "}"})
	case NodeArray:
		out := lit("[")
		for i, it := range n.Items {
			if i > 0 {
				out = append(out, Part{Lit: ","})
			}
			out = append(out, compactJSON(it)...)
		}
		return append(out, Part{Lit: "]"})
	case NodeString:
		return jsonString(n.Str)
	case NodeNumber, NodeBool:
		return lit(n.Raw)
	case NodeBare:
		return Str{{Var: n.Raw}}
	}
	return lit("null")
}

func jsonString(s Str) Str {
	out := lit(`"`)
	for _, p := range s {
		if p.Var != "" {
			out = append(out, p)
			continue
		}
		q := quoteJSON(p.Lit)
		out = append(out, Part{Lit: q[1 : len(q)-1]})
	}
	return append(out, Part{Lit: `"`})
}

// escapeItemKey backslash-escapes characters HTTPie would read as item
// separators or nesting.
func escapeItemKey(s Str) Str {
	out := make(Str, len(s))
	for i, p := range s {
		if p.Var == "" {
			var sb strings.Builder
			for _, c := range p.Lit {
				if strings.ContainsRune(`:=@\[]`, c) {
					sb.WriteByte('\\')
				}
				sb.WriteRune(c)
			}
			p.Lit = sb.String()
		}
		out[i] = p
	}
	return out
}

// shArg quotes a shell argument: single quotes for plain text, double
// quotes with ${NAME} when it uses variables.
func shArg(s Str, ids map[string]string) string {
	if t, ok := s.Plain(); ok {
		if t != "" && strings.Trim(t, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:=@%+,") == "" {
			return t
		}
		return shellQuote(t)
	}
	var sb strings.Builder
	sb.WriteString(`"`)
	for _, p := range s {
		if p.Var != "" {
			sb.WriteString("${" + ids[p.Var] + "}")
			continue
		}
		for _, c := range p.Lit {
			if strings.ContainsRune("\\\"$`", c) {
				sb.WriteByte('\\')
			}
			sb.WriteRune(c)
		}
	}
	sb.WriteString(`"`)
	return sb.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
