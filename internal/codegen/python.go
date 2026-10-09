package codegen

import (
	"fmt"
	"strings"
)

var pyNames = naming{
	join:   snake,
	suffix: "_var",
	reserved: set("url", "params", "headers", "payload", "response", "requests", "auth",
		"and", "as", "assert", "async", "await", "break", "class", "continue", "def", "del",
		"elif", "else", "except", "false", "finally", "for", "from", "global", "if", "import",
		"in", "is", "lambda", "none", "nonlocal", "not", "or", "pass", "raise", "return",
		"true", "try", "while", "with", "yield", "print", "json", "type", "id"),
}

var pyMethods = set("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS")

func python(s *Spec) string {
	ids := pyNames.names(s.Vars)
	str := func(x Str) string { return pyStr(x, ids) }
	var sb strings.Builder
	parse := parsedVars(s.Vars)
	if len(parse) > 0 {
		sb.WriteString("import json\n\n")
	}
	sb.WriteString("import requests\n\n")

	if len(s.Vars) > 0 {
		for _, v := range s.Vars {
			fmt.Fprintf(&sb, "%s = %s\n", ids[v.Name], varValue(v, func(n *Node) string { return pyNode(n, nil, nil, "") }, func(t string) string { return pyStr(lit(t), nil) }))
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "url = %s\n", str(s.URL))
	headers := s.Headers
	if s.ContentType != "" {
		headers = append([]Header{{lit("Content-Type"), lit(s.ContentType)}}, headers...)
	}
	headers = mergeHeaders(headers)
	args := []string{"url"}
	if len(s.Query) > 0 {
		sb.WriteString(pyPairs("params", s.Query, str))
		args = append(args, "params=params")
	}
	if len(headers) > 0 {
		sb.WriteString("headers = {\n")
		for _, h := range headers {
			fmt.Fprintf(&sb, "    %s: %s,\n", str(h.Name), str(h.Value))
		}
		sb.WriteString("}\n")
		args = append(args, "headers=headers")
	}
	switch s.BodyKind {
	case BodyJSON:
		fmt.Fprintf(&sb, "payload = %s\n", pyNode(s.JSON, ids, parse, ""))
		args = append(args, "json=payload")
	case BodyForm:
		sb.WriteString(pyPairs("payload", s.Form, str))
		args = append(args, "data=payload")
	case BodyRaw:
		fmt.Fprintf(&sb, "payload = %s\n", str(s.Raw))
		args = append(args, "data=payload")
	}
	if s.Basic != nil {
		args = append(args, fmt.Sprintf("auth=(%s, %s)", str(s.Basic.User), str(s.Basic.Pass)))
	}

	call := "requests." + strings.ToLower(s.Method) + "("
	if !pyMethods[s.Method] {
		call = fmt.Sprintf("requests.request(%s, ", pyStr(lit(s.Method), nil))
	}
	fmt.Fprintf(&sb, "\nresponse = %s%s)\n", call, strings.Join(args, ", "))
	sb.WriteString("print(response.status_code)\nprint(response.text)\n")
	return sb.String()
}

// varValue renders a variable's declared value: a JSON literal when the
// variable stands for a whole JSON value and holds valid JSON, otherwise
// a string.
func varValue(v Variable, node func(*Node) string, text func(string) string) string {
	if literal(v) {
		b := &builder{values: map[string]string{}}
		return node(b.parseJSON(v.Value))
	}
	return text(v.Value)
}

// pyStr renders a Python string literal, or an f-string when it uses
// variables.
func pyStr(s Str, ids map[string]string) string {
	if t, ok := s.Plain(); ok {
		return quoteJSON(t)
	}
	if len(s) == 1 {
		return ids[s[0].Var]
	}
	var sb strings.Builder
	sb.WriteString(`f"`)
	for _, p := range s {
		if p.Var != "" {
			sb.WriteString("{" + ids[p.Var] + "}")
			continue
		}
		q := quoteJSON(p.Lit)
		q = q[1 : len(q)-1]
		q = strings.ReplaceAll(q, "{", "{{")
		q = strings.ReplaceAll(q, "}", "}}")
		sb.WriteString(q)
	}
	sb.WriteString(`"`)
	return sb.String()
}

func pyNode(n *Node, ids map[string]string, parse map[string]bool, pad string) string {
	in := pad + "    "
	switch n.Kind {
	case NodeObject:
		if len(n.Members) == 0 {
			return "{}"
		}
		var sb strings.Builder
		sb.WriteString("{\n")
		for _, m := range n.Members {
			fmt.Fprintf(&sb, "%s%s: %s,\n", in, pyStr(m.Key, ids), pyNode(m.Val, ids, parse, in))
		}
		return sb.String() + pad + "}"
	case NodeArray:
		if len(n.Items) == 0 {
			return "[]"
		}
		var sb strings.Builder
		sb.WriteString("[\n")
		for _, it := range n.Items {
			fmt.Fprintf(&sb, "%s%s,\n", in, pyNode(it, ids, parse, in))
		}
		return sb.String() + pad + "]"
	case NodeString:
		return pyStr(n.Str, ids)
	case NodeNumber:
		return n.Raw
	case NodeBool:
		if n.Raw == "true" {
			return "True"
		}
		return "False"
	case NodeNull:
		return "None"
	case NodeBare:
		if parse[n.Raw] {
			return "json.loads(" + ids[n.Raw] + ")"
		}
		return ids[n.Raw]
	}
	return "None"
}

// pyPairs writes a dict, or a list of pairs when a key repeats.
func pyPairs(name string, fields []Field, str func(Str) string) string {
	var sb strings.Builder
	if hasDuplicateKeys(fields) {
		sb.WriteString(name + " = [\n")
		for _, f := range fields {
			fmt.Fprintf(&sb, "    (%s, %s),\n", str(f.Key), str(f.Value))
		}
		sb.WriteString("]\n")
		return sb.String()
	}
	sb.WriteString(name + " = {\n")
	for _, f := range fields {
		fmt.Fprintf(&sb, "    %s: %s,\n", str(f.Key), str(f.Value))
	}
	sb.WriteString("}\n")
	return sb.String()
}
