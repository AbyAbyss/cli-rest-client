package codegen

import (
	"fmt"
	"strings"
)

var jsNames = naming{
	join:   camel,
	suffix: "Var",
	reserved: set("url", "URL", "response", "fetch", "btoa", "console", "JSON", "URLSearchParams",
		"break", "case", "catch", "class", "const", "continue", "debugger", "default", "delete",
		"do", "else", "export", "extends", "false", "finally", "for", "function", "if", "import",
		"in", "instanceof", "let", "new", "null", "return", "super", "switch", "this", "throw",
		"true", "try", "typeof", "var", "void", "while", "with", "yield", "await", "enum",
		"static", "implements", "interface", "package", "private", "protected", "public", "undefined"),
}

func javascript(s *Spec) string {
	ids := jsNames.names(s.Vars)
	parse := parsedVars(s.Vars)
	str := func(x Str) string { return jsStr(x, ids) }
	var sb strings.Builder

	if len(s.Vars) > 0 {
		for _, v := range s.Vars {
			fmt.Fprintf(&sb, "const %s = %s;\n", ids[v.Name], varValue(v, func(n *Node) string { return jsNode(n, nil, nil, "") }, func(t string) string { return quoteJSON(t) }))
		}
		sb.WriteString("\n")
	}

	headers := s.Headers
	if s.ContentType != "" {
		headers = append([]Header{{lit("Content-Type"), lit(s.ContentType)}}, headers...)
	}
	headers = mergeHeaders(headers)
	var opts []string
	if s.Method != "GET" {
		opts = append(opts, "method: "+quoteJSON(s.Method))
	}
	if len(headers) > 0 || s.Basic != nil {
		var hb strings.Builder
		hb.WriteString("headers: {\n")
		for _, h := range headers {
			fmt.Fprintf(&hb, "    %s: %s,\n", jsKey(h.Name, ids), str(h.Value))
		}
		if s.Basic != nil {
			creds := concat(s.Basic.User, lit(":"), s.Basic.Pass)
			fmt.Fprintf(&hb, "    \"Authorization\": \"Basic \" + btoa(%s),\n", str(creds))
		}
		hb.WriteString("  }")
		opts = append(opts, hb.String())
	}
	body := ""
	switch s.BodyKind {
	case BodyJSON:
		body = "JSON.stringify(" + jsNode(s.JSON, ids, parse, "  ") + ")"
	case BodyForm:
		var fb strings.Builder
		if hasDuplicateKeys(s.Form) {
			fb.WriteString("new URLSearchParams([\n")
			for _, f := range s.Form {
				fmt.Fprintf(&fb, "    [%s, %s],\n", str(f.Key), str(f.Value))
			}
			fb.WriteString("  ])")
		} else {
			fb.WriteString("new URLSearchParams({\n")
			for _, f := range s.Form {
				fmt.Fprintf(&fb, "    %s: %s,\n", jsKey(f.Key, ids), str(f.Value))
			}
			fb.WriteString("  })")
		}
		body = fb.String()
	case BodyRaw:
		body = str(s.Raw)
	}
	if body != "" {
		if s.Method == "GET" || s.Method == "HEAD" {
			sb.WriteString("// fetch can't send a body with " + s.Method + ", so it is left out.\n")
		} else {
			opts = append(opts, "body: "+body)
		}
	}

	target := str(s.URL)
	if len(s.Query) > 0 {
		fmt.Fprintf(&sb, "const url = new URL(%s);\n", target)
		for _, q := range s.Query {
			fmt.Fprintf(&sb, "url.searchParams.append(%s, %s);\n", str(q.Key), str(q.Value))
		}
		sb.WriteString("\n")
		target = "url"
	}
	fmt.Fprintf(&sb, "const response = await fetch(%s", target)
	if len(opts) > 0 {
		sb.WriteString(", {\n")
		for _, o := range opts {
			sb.WriteString("  " + o + ",\n")
		}
		sb.WriteString("}")
	}
	sb.WriteString(");\n\nconsole.log(response.status);\nconsole.log(await response.text());\n")
	return sb.String()
}

// jsStr renders a JavaScript string, or a template literal when it uses
// variables.
func jsStr(s Str, ids map[string]string) string {
	if t, ok := s.Plain(); ok {
		return quoteJSON(t)
	}
	if len(s) == 1 {
		return ids[s[0].Var]
	}
	var sb strings.Builder
	sb.WriteString("`")
	for _, p := range s {
		if p.Var != "" {
			sb.WriteString("${" + ids[p.Var] + "}")
			continue
		}
		t := strings.ReplaceAll(p.Lit, `\`, `\\`)
		t = strings.ReplaceAll(t, "`", "\\`")
		t = strings.ReplaceAll(t, "${", "\\${")
		sb.WriteString(t)
	}
	sb.WriteString("`")
	return sb.String()
}

// jsKey renders an object key; keys with variables become computed keys.
func jsKey(s Str, ids map[string]string) string {
	if _, ok := s.Plain(); ok {
		return jsStr(s, ids)
	}
	return "[" + jsStr(s, ids) + "]"
}

func jsNode(n *Node, ids map[string]string, parse map[string]bool, pad string) string {
	in := pad + "  "
	switch n.Kind {
	case NodeObject:
		if len(n.Members) == 0 {
			return "{}"
		}
		var sb strings.Builder
		sb.WriteString("{\n")
		for _, m := range n.Members {
			fmt.Fprintf(&sb, "%s%s: %s,\n", in, jsKey(m.Key, ids), jsNode(m.Val, ids, parse, in))
		}
		return sb.String() + pad + "}"
	case NodeArray:
		if len(n.Items) == 0 {
			return "[]"
		}
		var sb strings.Builder
		sb.WriteString("[\n")
		for _, it := range n.Items {
			fmt.Fprintf(&sb, "%s%s,\n", in, jsNode(it, ids, parse, in))
		}
		return sb.String() + pad + "]"
	case NodeString:
		return jsStr(n.Str, ids)
	case NodeNumber, NodeBool:
		return n.Raw
	case NodeBare:
		if parse[n.Raw] {
			return "JSON.parse(" + ids[n.Raw] + ")"
		}
		return ids[n.Raw]
	}
	return "null"
}
