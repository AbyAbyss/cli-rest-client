package codegen

import (
	"fmt"
	"go/format"
	"net/url"
	"strconv"
	"strings"
)

var goNames = naming{
	join:   camel,
	suffix: "Var",
	reserved: set("req", "resp", "err", "payload", "form", "data", "fmt", "io", "http", "strings",
		"url", "main", "string", "int", "bool", "error", "len", "nil", "true", "false", "panic",
		"break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough",
		"for", "func", "go", "goto", "if", "import", "interface", "map", "package", "range",
		"return", "select", "struct", "switch", "type", "var"),
}

var goMethods = map[string]string{
	"GET": "MethodGet", "POST": "MethodPost", "PUT": "MethodPut", "PATCH": "MethodPatch",
	"DELETE": "MethodDelete", "HEAD": "MethodHead", "OPTIONS": "MethodOptions",
}

func golang(s *Spec) string {
	ids := goNames.names(s.Vars)
	str := func(x Str) string { return goStr(x, ids) }

	imports := []string{"fmt", "io", "net/http"}
	for _, q := range s.Query {
		if _, plain := concat(q.Key, q.Value).Plain(); !plain {
			imports = append(imports, "net/url")
		}
	}
	var body strings.Builder
	payload := "nil"
	switch s.BodyKind {
	case BodyJSON, BodyRaw:
		imports = append(imports, "strings")
		fmt.Fprintf(&body, "\tpayload := strings.NewReader(%s)\n", str(s.Raw))
		payload = "payload"
	case BodyForm:
		imports = append(imports, "net/url", "strings")
		body.WriteString("\tform := url.Values{}\n")
		for _, f := range s.Form {
			fmt.Fprintf(&body, "\tform.Add(%s, %s)\n", str(f.Key), str(f.Value))
		}
		body.WriteString("\tpayload := strings.NewReader(form.Encode())\n")
		payload = "payload"
	}

	var sb strings.Builder
	sb.WriteString("package main\n\nimport (\n")
	for _, imp := range sortedImports(imports) {
		fmt.Fprintf(&sb, "\t%q\n", imp)
	}
	sb.WriteString(")\n\nfunc main() {\n")
	for _, v := range s.Vars {
		fmt.Fprintf(&sb, "\t%s := %s\n", ids[v.Name], strconv.Quote(v.Value))
	}
	if len(s.Vars) > 0 {
		sb.WriteString("\n")
	}
	if body.Len() > 0 {
		sb.WriteString(body.String() + "\n")
	}

	method := strconv.Quote(s.Method)
	if c, ok := goMethods[s.Method]; ok {
		method = "http." + c
	}
	target := str(s.URL)
	if len(s.Query) > 0 {
		target = goQuery(s, ids)
	}
	fmt.Fprintf(&sb, "\treq, err := http.NewRequest(%s, %s, %s)\n", method, target, payload)
	sb.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")

	headers := s.Headers
	if s.ContentType != "" {
		headers = append([]Header{{lit("Content-Type"), lit(s.ContentType)}}, headers...)
	}
	seen := map[string]bool{}
	for _, h := range headers {
		name, plain := h.Name.Plain()
		key := strings.ToLower(name)
		switch {
		case plain && key == "host":
			fmt.Fprintf(&sb, "\treq.Host = %s\n", str(h.Value))
		case plain && seen[key]:
			fmt.Fprintf(&sb, "\treq.Header.Add(%s, %s)\n", str(h.Name), str(h.Value))
		default:
			fmt.Fprintf(&sb, "\treq.Header.Set(%s, %s)\n", str(h.Name), str(h.Value))
		}
		seen[key] = true
	}
	if s.Basic != nil {
		fmt.Fprintf(&sb, "\treq.SetBasicAuth(%s, %s)\n", str(s.Basic.User), str(s.Basic.Pass))
	}

	sb.WriteString(`
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Status)
	fmt.Println(string(data))
}
`)
	if out, err := format.Source([]byte(sb.String())); err == nil {
		return string(out)
	}
	return sb.String()
}

// sortedImports orders imports as gofmt would, without duplicates.
func sortedImports(imps []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, i := range imps {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// goStr renders a Go string expression: literals joined with variables by
// +. Multi-line text and text with quotes use raw string literals.
func goStr(s Str, ids map[string]string) string {
	if len(s) == 0 {
		return `""`
	}
	s = mergeLits(s)
	parts := make([]string, len(s))
	for i, p := range s {
		if p.Var != "" && p.Escape {
			parts[i] = "url.QueryEscape(" + ids[p.Var] + ")"
			continue
		}
		if p.Var != "" {
			parts[i] = ids[p.Var]
			continue
		}
		parts[i] = goLit(p.Lit)
	}
	return strings.Join(parts, " + ")
}

func goLit(t string) string {
	if (strings.Contains(t, "\n") || strings.Contains(t, `"`)) && !strings.ContainsAny(t, "`\r") {
		return "`" + t + "`"
	}
	return strconv.Quote(t)
}

// goQuery appends the query fields to the URL, escaping literal text now
// and variables with url.QueryEscape at run time, as the app does.
func goQuery(s *Spec, ids map[string]string) string {
	sep := "?"
	for _, p := range s.URL {
		if p.Var == "" && strings.Contains(p.Lit, "?") {
			sep = "&"
		}
	}
	parts := append(Str(nil), s.URL...)
	escaped := func(x Str) {
		for _, p := range x {
			if p.Var != "" {
				parts = append(parts, Part{Var: p.Var, Lit: "escape"})
			} else {
				parts = append(parts, Part{Lit: url.QueryEscape(p.Lit)})
			}
		}
	}
	for _, q := range s.Query {
		parts = append(parts, Part{Lit: sep})
		escaped(q.Key)
		parts = append(parts, Part{Lit: "="})
		escaped(q.Value)
		sep = "&"
	}
	// Merge neighbouring literals so the expression stays short.
	var merged []string
	lits := ""
	for _, p := range parts {
		if p.Var == "" {
			lits += p.Lit
			continue
		}
		if lits != "" {
			merged = append(merged, strconv.Quote(lits))
			lits = ""
		}
		if p.Lit == "escape" {
			merged = append(merged, "url.QueryEscape("+ids[p.Var]+")")
		} else {
			merged = append(merged, ids[p.Var])
		}
	}
	if lits != "" {
		merged = append(merged, strconv.Quote(lits))
	}
	return strings.Join(merged, " + ")
}

// mergeLits joins neighbouring literal parts.
func mergeLits(s Str) Str {
	var out Str
	for _, p := range s {
		if n := len(out); n > 0 && p.Var == "" && out[n-1].Var == "" {
			out[n-1].Lit += p.Lit
			continue
		}
		out = append(out, p)
	}
	return out
}
