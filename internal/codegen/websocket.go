package codegen

import (
	"encoding/base64"
	"fmt"
	"go/format"
	"net/url"
	"strconv"
	"strings"

	"github.com/AbyAbyss/cli-rest-client/internal/engine"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/script"
)

// wsSpec is a WebSocket request: where to connect, the handshake headers,
// and the message to send once connected.
type wsSpec struct {
	URL       Str
	Headers   []Header
	Basic     *Basic
	Protocols []string // from a Sec-WebSocket-Protocol header
	Message   Str
	Vars      []Variable
	Missing   []string
}

func buildWS(r models.Request, values map[string]string, template bool) (*wsSpec, error) {
	r.Normalize()
	v := make(map[string]string, len(values))
	for k, x := range values {
		v[k] = x
	}
	if strings.TrimSpace(r.PreRequest) != "" {
		script.RunPre(r.PreRequest, v)
	}
	p, err := engine.Prepare(r, v)
	if err != nil {
		return nil, err
	}
	s := &wsSpec{Missing: p.Missing}
	b := &builder{template: template, values: v, index: map[string]int{}, spec: &Spec{}}

	if template {
		u := strings.TrimSpace(r.URL)
		switch {
		case strings.HasPrefix(u, "http://"):
			u = "ws://" + u[len("http://"):]
		case strings.HasPrefix(u, "https://"):
			u = "wss://" + u[len("https://"):]
		case !strings.Contains(u, "://") && !strings.HasPrefix(u, "{{"):
			u = "ws://" + u
		}
		s.URL = b.str(u)
		// Query values are URL-encoded: literal text now, variables when
		// the snippet runs.
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		addQuery := func(k, v string) {
			s.URL = append(s.URL, Part{Lit: sep})
			s.URL = append(s.URL, b.queryStr(k)...)
			s.URL = append(s.URL, Part{Lit: "="})
			s.URL = append(s.URL, b.queryStr(v)...)
			sep = "&"
		}
		for _, kv := range r.Params {
			if !kv.Disabled && kv.Key != "" {
				addQuery(kv.Key, kv.Value)
			}
		}
		if r.Auth.Type == models.AuthAPIKey && r.Auth.In == "query" && r.Auth.Key != "" {
			addQuery(r.Auth.Key, r.Auth.Value)
		}
		s.URL = mergeLits(s.URL)
		s.Message = b.str(r.Body)
	} else {
		s.URL = lit(p.Request.URL.String())
		s.Message = lit(p.Message)
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
	for _, h := range r.Headers {
		if h.Disabled || h.Key == "" {
			continue
		}
		name := strings.ToLower(b.resolve(h.Key))
		if name == authHeader {
			continue
		}
		if name == "sec-websocket-protocol" {
			for _, p := range strings.Split(b.resolve(h.Value), ",") {
				if p = strings.TrimSpace(p); p != "" {
					s.Protocols = append(s.Protocols, p)
				}
			}
			continue
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
	s.Vars = b.spec.Vars
	return s, nil
}

// queryStr is a query key or value: literal text encoded now, variables
// marked to be encoded at run time.
func (b *builder) queryStr(s string) Str {
	out := b.str(s)
	for i := range out {
		if out[i].Var != "" {
			out[i].Escape = true
		} else {
			out[i].Lit = url.QueryEscape(out[i].Lit)
		}
	}
	return out
}

// hasEscape reports whether s URL-encodes a variable at run time.
func hasEscape(s Str) bool {
	for _, p := range s {
		if p.Escape {
			return true
		}
	}
	return false
}

func generateWS(lang string, r models.Request, values map[string]string, template bool) (*Snippet, error) {
	s, err := buildWS(r, values, template)
	if err != nil {
		return nil, err
	}
	var text string
	switch lang {
	case "curl":
		text = wsWebsocat(s)
	case "python":
		text = wsPython(s)
	case "javascript":
		text = wsJavaScript(s)
	case "go":
		text = wsGo(s)
	case "httpie":
		text = "# HTTPie doesn't do WebSocket connections.\n# The cURL tab has the same request as a websocat command.\n"
	}
	return &Snippet{Text: text, Missing: s.Missing}, nil
}

// basicPlain is the Basic credential when it has no variables.
func basicPlain(b *Basic) (string, bool) {
	u, ok1 := b.User.Plain()
	p, ok2 := b.Pass.Plain()
	if !ok1 || !ok2 {
		return "", false
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p)), true
}

func wsWebsocat(s *wsSpec) string {
	ids := shNames.names(s.Vars)
	arg := func(x Str) string { return shArg(x, ids) }
	var sb strings.Builder
	sb.WriteString("# curl can't keep a WebSocket open: this uses websocat (https://github.com/vi/websocat).\n")
	if hasEscape(s.URL) {
		sb.WriteString("# Variables in the query string are inserted as they are: URL-encode their values.\n")
	}
	for _, v := range s.Vars {
		fmt.Fprintf(&sb, "%s=%s\n", ids[v.Name], shellQuote(v.Value))
	}
	if len(s.Vars) > 0 {
		sb.WriteString("\n")
	}
	var parts []string
	if len(s.Message) > 0 {
		sb.WriteString("printf '%s' " + arg(s.Message) + " | ")
	}
	parts = append(parts, "websocat")
	if len(s.Message) > 0 {
		// -n keeps reading replies after stdin ends; --no-line sends the
		// message as it is instead of one message per line.
		parts = append(parts, "-n", "--no-line")
	}
	for _, h := range s.Headers {
		// --header= takes exactly one value (-H can swallow the URL).
		parts = append(parts, "--header="+arg(concat(h.Name, lit(": "), h.Value)))
	}
	if s.Basic != nil {
		if hdr, ok := basicPlain(s.Basic); ok {
			parts = append(parts, "--header="+shellQuote("Authorization: "+hdr))
		} else {
			creds := strings.TrimSuffix(strings.TrimPrefix(arg(concat(s.Basic.User, lit(":"), s.Basic.Pass)), `"`), `"`)
			parts = append(parts, `--header="Authorization: Basic $(printf '%s' "`+creds+`" | base64)"`)
		}
	}
	for _, p := range s.Protocols {
		parts = append(parts, "--protocol "+shellQuote(p))
	}
	parts = append(parts, arg(s.URL))
	sb.WriteString(strings.Join(parts, " \\\n  ") + "\n")
	return sb.String()
}

func wsPython(s *wsSpec) string {
	ids := pyNames.names(s.Vars)
	str := func(x Str) string { return pyStr(x, ids) }
	var sb strings.Builder
	if s.Basic != nil {
		sb.WriteString("import base64\n\n")
	}
	if hasEscape(s.URL) {
		sb.WriteString("from urllib.parse import quote_plus\n\n")
	}
	sb.WriteString("from websockets.sync.client import connect\n\n")
	for _, v := range s.Vars {
		fmt.Fprintf(&sb, "%s = %s\n", ids[v.Name], pyStr(lit(v.Value), nil))
	}
	if len(s.Vars) > 0 {
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "url = %s\n", str(s.URL))
	args := []string{"url"}
	headers := mergeHeaders(s.Headers)
	if len(headers) > 0 || s.Basic != nil {
		sb.WriteString("headers = {\n")
		for _, h := range headers {
			fmt.Fprintf(&sb, "    %s: %s,\n", str(h.Name), str(h.Value))
		}
		if s.Basic != nil {
			creds := pyStr(concat(s.Basic.User, lit(":"), s.Basic.Pass), ids)
			fmt.Fprintf(&sb, "    \"Authorization\": \"Basic \" + base64.b64encode(%s.encode()).decode(),\n", creds)
		}
		sb.WriteString("}\n")
		args = append(args, "additional_headers=headers")
	}
	if len(s.Protocols) > 0 {
		quoted := make([]string, len(s.Protocols))
		for i, p := range s.Protocols {
			quoted[i] = quoteJSON(p)
		}
		args = append(args, "subprotocols=["+strings.Join(quoted, ", ")+"]")
	}
	sb.WriteString("\n# websockets 14 or newer (pip install websockets)\n")
	fmt.Fprintf(&sb, "with connect(%s) as ws:\n", strings.Join(args, ", "))
	if len(s.Message) > 0 {
		fmt.Fprintf(&sb, "    ws.send(%s)\n", str(s.Message))
	}
	sb.WriteString("    for message in ws:\n        print(message)\n")
	return sb.String()
}

func wsJavaScript(s *wsSpec) string {
	ids := jsNames.names(s.Vars)
	str := func(x Str) string { return jsStr(x, ids) }
	var sb strings.Builder
	for _, v := range s.Vars {
		fmt.Fprintf(&sb, "const %s = %s;\n", ids[v.Name], quoteJSON(v.Value))
	}
	if len(s.Vars) > 0 {
		sb.WriteString("\n")
	}
	headers := mergeHeaders(s.Headers)
	var opts []string
	if len(headers) > 0 || s.Basic != nil {
		var hb strings.Builder
		hb.WriteString("headers: {\n")
		for _, h := range headers {
			fmt.Fprintf(&hb, "    %s: %s,\n", jsKey(h.Name, ids), str(h.Value))
		}
		if s.Basic != nil {
			fmt.Fprintf(&hb, "    \"Authorization\": \"Basic \" + btoa(%s),\n", str(concat(s.Basic.User, lit(":"), s.Basic.Pass)))
		}
		hb.WriteString("  }")
		opts = append(opts, hb.String())
	}
	if len(s.Protocols) > 0 {
		quoted := make([]string, len(s.Protocols))
		for i, p := range s.Protocols {
			quoted[i] = quoteJSON(p)
		}
		opts = append(opts, "protocols: ["+strings.Join(quoted, ", ")+"]")
	}
	if len(opts) > 0 {
		sb.WriteString("// Node.js 22+. Browsers can't set headers on a WebSocket; pass only the URL there.\n")
		fmt.Fprintf(&sb, "const ws = new WebSocket(%s, {\n  %s,\n});\n", str(s.URL), strings.Join(opts, ",\n  "))
	} else {
		fmt.Fprintf(&sb, "const ws = new WebSocket(%s);\n", str(s.URL))
	}
	if len(s.Message) > 0 {
		fmt.Fprintf(&sb, "\nws.addEventListener(\"open\", () => {\n  ws.send(%s);\n});\n", str(s.Message))
	}
	sb.WriteString("\nws.addEventListener(\"message\", (event) => {\n  console.log(event.data);\n});\n")
	sb.WriteString("ws.addEventListener(\"close\", (event) => {\n  console.log(\"closed\", event.code, event.reason);\n});\n")
	return sb.String()
}

func wsGo(s *wsSpec) string {
	ids := goNames.names(s.Vars)
	str := func(x Str) string { return goStr(x, ids) }
	var sb strings.Builder
	imports := []string{"context", "fmt", "github.com/coder/websocket"}
	if len(s.Headers) > 0 || s.Basic != nil {
		imports = append(imports, "net/http")
	}
	if hasEscape(s.URL) {
		imports = append(imports, "net/url")
	}
	if s.Basic != nil {
		imports = append(imports, "encoding/base64")
	}
	sb.WriteString("package main\n\nimport (\n")
	var std, ext []string
	for _, i := range sortedImports(imports) {
		if strings.Contains(i, ".") {
			ext = append(ext, i)
		} else {
			std = append(std, i)
		}
	}
	for _, i := range std {
		fmt.Fprintf(&sb, "\t%q\n", i)
	}
	sb.WriteString("\n")
	for _, i := range ext {
		fmt.Fprintf(&sb, "\t%q\n", i)
	}
	sb.WriteString(")\n\n// go get github.com/coder/websocket\nfunc main() {\n\tctx := context.Background()\n")
	for _, v := range s.Vars {
		fmt.Fprintf(&sb, "\t%s := %s\n", ids[v.Name], strconv.Quote(v.Value))
	}
	opts := []string{}
	if len(s.Headers) > 0 || s.Basic != nil {
		sb.WriteString("\theader := http.Header{}\n")
		seen := map[string]bool{}
		for _, h := range s.Headers {
			name, _ := h.Name.Plain()
			fn := "Set"
			if seen[strings.ToLower(name)] {
				fn = "Add"
			}
			seen[strings.ToLower(name)] = true
			fmt.Fprintf(&sb, "\theader.%s(%s, %s)\n", fn, str(h.Name), str(h.Value))
		}
		if s.Basic != nil {
			fmt.Fprintf(&sb, "\theader.Set(\"Authorization\", \"Basic \"+base64.StdEncoding.EncodeToString([]byte(%s)))\n",
				str(concat(s.Basic.User, lit(":"), s.Basic.Pass)))
		}
		opts = append(opts, "HTTPHeader: header")
	}
	if len(s.Protocols) > 0 {
		quoted := make([]string, len(s.Protocols))
		for i, p := range s.Protocols {
			quoted[i] = strconv.Quote(p)
		}
		opts = append(opts, "Subprotocols: []string{"+strings.Join(quoted, ", ")+"}")
	}
	dialOpts := "nil"
	if len(opts) > 0 {
		dialOpts = "&websocket.DialOptions{" + strings.Join(opts, ", ") + "}"
	}
	fmt.Fprintf(&sb, "\n\tconn, _, err := websocket.Dial(ctx, %s, %s)\n", str(s.URL), dialOpts)
	sb.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n\tdefer conn.CloseNow()\n")
	if len(s.Message) > 0 {
		fmt.Fprintf(&sb, "\n\tif err := conn.Write(ctx, websocket.MessageText, []byte(%s)); err != nil {\n\t\tpanic(err)\n\t}\n", str(s.Message))
	}
	sb.WriteString(`
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			fmt.Println("closed:", websocket.CloseStatus(err))
			return
		}
		fmt.Println(string(data))
	}
}
`)
	if out, err := format.Source([]byte(sb.String())); err == nil {
		return string(out)
	}
	return sb.String()
}
