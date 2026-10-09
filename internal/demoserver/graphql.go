package demoserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

// A tiny GraphQL server shaped like the public countries API
// (countries.trevorblades.com): enough to test sending queries, variables,
// selection sets and schema introspection, not a GraphQL implementation.
//
//	type Query { countries(continent: String): [Country!]!  country(code: ID!): Country }
//	type Country { code: ID!  name: String!  capital: String  currency: String  continent: Continent! }
//	type Continent { code: ID!  name: String! }

type gqlObject map[string]any

var countries = []gqlObject{
	{"code": "BR", "name": "Brazil", "capital": "Brasília", "currency": "BRL", "continent": gqlObject{"code": "SA", "name": "South America"}},
	{"code": "GB", "name": "United Kingdom", "capital": "London", "currency": "GBP", "continent": gqlObject{"code": "EU", "name": "Europe"}},
	{"code": "IN", "name": "India", "capital": "New Delhi", "currency": "INR", "continent": gqlObject{"code": "AS", "name": "Asia"}},
	{"code": "JP", "name": "Japan", "capital": "Tokyo", "currency": "JPY", "continent": gqlObject{"code": "AS", "name": "Asia"}},
}

func addGraphQL(mux *http.ServeMux) {
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		switch r.Method {
		case http.MethodGet:
			req.Query = r.URL.Query().Get("query")
			if v := r.URL.Query().Get("variables"); v != "" {
				_ = json.Unmarshal([]byte(v), &req.Variables)
			}
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				gqlError(w, "request body is not JSON: "+err.Error())
				return
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if strings.Contains(req.Query, "__schema") {
			writeJSON(w, http.StatusOK, gqlObject{"data": gqlObject{"__schema": introspection()}})
			return
		}
		sel, err := parseQuery(req.Query)
		if err != nil {
			gqlError(w, err.Error())
			return
		}
		data := gqlObject{}
		for _, f := range sel {
			arg := func(name string) string {
				v := f.args[name]
				if strings.HasPrefix(v, "$") {
					s, _ := req.Variables[v[1:]].(string)
					return s
				}
				return v
			}
			switch f.name {
			case "countries":
				var list []any
				for _, c := range countries {
					if cont := arg("continent"); cont == "" || c["continent"].(gqlObject)["code"] == cont {
						list = append(list, project(c, f.sub))
					}
				}
				data[f.alias] = list
			case "country":
				data[f.alias] = nil
				for _, c := range countries {
					if c["code"] == arg("code") {
						data[f.alias] = project(c, f.sub)
					}
				}
			default:
				gqlError(w, fmt.Sprintf("Cannot query field %q on type \"Query\".", f.name))
				return
			}
		}
		writeJSON(w, http.StatusOK, gqlObject{"data": data})
	})
}

func gqlError(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusOK, gqlObject{"errors": []any{gqlObject{"message": msg}}})
}

// project keeps the selected fields of an object, recursively.
func project(obj gqlObject, sel []gqlField) gqlObject {
	out := gqlObject{}
	for _, f := range sel {
		v := obj[f.name]
		if sub, ok := v.(gqlObject); ok && len(f.sub) > 0 {
			v = project(sub, f.sub)
		}
		out[f.alias] = v
	}
	return out
}

type gqlField struct {
	alias, name string
	args        map[string]string // literal values, or "$name" for variables
	sub         []gqlField
}

// parseQuery reads the operation's top-level selection set.
func parseQuery(q string) ([]gqlField, error) {
	toks := gqlTokens(q)
	i := 0
	// Skip "query Name(...)" up to the first "{".
	for i < len(toks) && toks[i] != "{" {
		i++
	}
	if i == len(toks) {
		return nil, fmt.Errorf("Syntax Error: Expected {")
	}
	sel, _, err := parseSelection(toks, i)
	return sel, err
}

func parseSelection(toks []string, i int) ([]gqlField, int, error) {
	i++ // {
	var out []gqlField
	for i < len(toks) && toks[i] != "}" {
		f := gqlField{name: toks[i], args: map[string]string{}}
		i++
		if i < len(toks) && toks[i] == ":" {
			f.alias, f.name = f.name, toks[i+1]
			i += 2
		}
		if f.alias == "" {
			f.alias = f.name
		}
		if i < len(toks) && toks[i] == "(" {
			i++
			for i+2 < len(toks) && toks[i] != ")" {
				f.args[toks[i]] = strings.Trim(toks[i+2], `"`) // name : value
				i += 3
			}
			i++ // )
		}
		if i < len(toks) && toks[i] == "{" {
			var err error
			if f.sub, i, err = parseSelection(toks, i); err != nil {
				return nil, i, err
			}
		}
		out = append(out, f)
	}
	if i >= len(toks) {
		return nil, i, fmt.Errorf("Syntax Error: Expected }")
	}
	return out, i + 1, nil
}

func gqlTokens(q string) []string {
	var toks []string
	for i := 0; i < len(q); {
		c := rune(q[i])
		switch {
		case unicode.IsSpace(c) || c == ',':
			i++
		case c == '#':
			for i < len(q) && q[i] != '\n' {
				i++
			}
		case c == '"':
			j := i + 1
			for j < len(q) && q[j] != '"' {
				j++
			}
			toks = append(toks, q[i:min(j+1, len(q))])
			i = j + 1
		case strings.ContainsRune("{}():!=[]", c):
			toks = append(toks, string(c))
			i++
		default:
			j := i
			for j < len(q) && !unicode.IsSpace(rune(q[j])) && !strings.ContainsRune("{}():!=[],#\"", rune(q[j])) {
				j++
			}
			toks = append(toks, q[i:j])
			i = j
		}
	}
	return toks
}

// introspection describes the schema in the shape of the standard
// introspection query's result.
func introspection() gqlObject {
	named := func(kind, name string) gqlObject { return gqlObject{"kind": kind, "name": name, "ofType": nil} }
	nonNull := func(t gqlObject) gqlObject { return gqlObject{"kind": "NON_NULL", "name": nil, "ofType": t} }
	list := func(t gqlObject) gqlObject { return gqlObject{"kind": "LIST", "name": nil, "ofType": t} }
	field := func(name, desc string, t gqlObject, args ...gqlObject) gqlObject {
		if args == nil {
			args = []gqlObject{}
		}
		return gqlObject{"name": name, "description": desc, "args": args, "type": t}
	}
	arg := func(name string, t gqlObject) gqlObject {
		return gqlObject{"name": name, "description": nil, "type": t, "defaultValue": nil}
	}
	object := func(name, desc string, fields ...gqlObject) gqlObject {
		return gqlObject{"kind": "OBJECT", "name": name, "description": desc, "fields": fields}
	}
	scalar := func(name string) gqlObject {
		return gqlObject{"kind": "SCALAR", "name": name, "description": nil, "fields": nil}
	}
	country := named("OBJECT", "Country")
	return gqlObject{
		"queryType":        gqlObject{"name": "Query"},
		"mutationType":     nil,
		"subscriptionType": nil,
		"types": []gqlObject{
			object("Query", "",
				field("countries", "All countries, optionally on one continent.", nonNull(list(nonNull(country))), arg("continent", named("SCALAR", "String"))),
				field("country", "A country by its ISO 3166 code.", country, arg("code", nonNull(named("SCALAR", "ID"))))),
			object("Country", "",
				field("code", "", nonNull(named("SCALAR", "ID"))),
				field("name", "", nonNull(named("SCALAR", "String"))),
				field("capital", "", named("SCALAR", "String")),
				field("currency", "", named("SCALAR", "String")),
				field("continent", "", nonNull(named("OBJECT", "Continent")))),
			object("Continent", "",
				field("code", "", nonNull(named("SCALAR", "ID"))),
				field("name", "", nonNull(named("SCALAR", "String")))),
			scalar("ID"), scalar("String"), scalar("Boolean"),
		},
	}
}
