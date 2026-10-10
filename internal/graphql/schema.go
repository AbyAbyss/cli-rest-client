// Package graphql reads a GraphQL schema through introspection and builds
// query skeletons from it, for the schema browser.
package graphql

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// IntrospectionQuery asks a server for its types, fields and arguments.
// It is the usual introspection query without directives, enum values and
// input fields beyond what the browser shows.
const IntrospectionQuery = `query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      kind name description
      fields(includeDeprecated: false) {
        name description
        args { name description type { ...TypeRef } defaultValue }
        type { ...TypeRef }
      }
    }
  }
}

fragment TypeRef on __Type {
  kind name
  ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name } } } } }
}`

// TypeRef is a possibly wrapped type: [Country!]! is NON_NULL > LIST >
// NON_NULL > Country.
type TypeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *TypeRef `json:"ofType"`
}

// String writes the type as GraphQL does: [Country!]!.
func (t *TypeRef) String() string {
	if t == nil {
		return "?"
	}
	switch t.Kind {
	case "NON_NULL":
		return t.OfType.String() + "!"
	case "LIST":
		return "[" + t.OfType.String() + "]"
	}
	return t.Name
}

// Named is the innermost named type.
func (t *TypeRef) Named() *TypeRef {
	for t != nil && t.OfType != nil {
		t = t.OfType
	}
	return t
}

// Arg is a field argument.
type Arg struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Type         *TypeRef `json:"type"`
	DefaultValue *string  `json:"defaultValue"`
}

// Field is a field of an object type.
type Field struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Args        []Arg    `json:"args"`
	Type        *TypeRef `json:"type"`
}

// Type is a named type.
type Type struct {
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
}

// Schema is the part of an introspection result the browser uses.
type Schema struct {
	Query, Mutation, Subscription string // root type names, "" if absent
	Types                         map[string]*Type
}

// Roots lists the operation types the schema has, with their root type.
func (s *Schema) Roots() [][2]string {
	var out [][2]string
	for _, r := range [][2]string{{"query", s.Query}, {"mutation", s.Mutation}, {"subscription", s.Subscription}} {
		if r[1] != "" && s.Types[r[1]] != nil {
			out = append(out, r)
		}
	}
	return out
}

// Parse reads a response to IntrospectionQuery.
func Parse(body []byte) (*Schema, error) {
	var resp struct {
		Data *struct {
			Schema *struct {
				QueryType        *struct{ Name string } `json:"queryType"`
				MutationType     *struct{ Name string } `json:"mutationType"`
				SubscriptionType *struct{ Name string } `json:"subscriptionType"`
				Types            []*Type                `json:"types"`
			} `json:"__schema"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("the response is not a GraphQL result: %v", err)
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return nil, errors.New("the server answered with errors: " + strings.Join(msgs, "; "))
	}
	if resp.Data == nil || resp.Data.Schema == nil {
		return nil, errors.New("the response has no __schema (introspection may be turned off on this server)")
	}
	sc := resp.Data.Schema
	s := &Schema{Types: map[string]*Type{}}
	for _, t := range sc.Types {
		if t != nil && !strings.HasPrefix(t.Name, "__") {
			s.Types[t.Name] = t
		}
	}
	if sc.QueryType != nil {
		s.Query = sc.QueryType.Name
	}
	if sc.MutationType != nil {
		s.Mutation = sc.MutationType.Name
	}
	if sc.SubscriptionType != nil {
		s.Subscription = sc.SubscriptionType.Name
	}
	return s, nil
}

// TypeNames lists the schema's named types, sorted.
func (s *Schema) TypeNames() []string {
	names := make([]string, 0, len(s.Types))
	for n := range s.Types {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Skeleton builds an operation that calls one root field: its arguments
// become variables, and the selection set lists the scalar fields of the
// result, nested one object level deep. It returns the query and the
// variables as indented JSON (null placeholders), or "" when there are none.
func (s *Schema) Skeleton(op string, f Field) (query, variables string) {
	var sb strings.Builder
	name := strings.ToUpper(f.Name[:1]) + f.Name[1:]
	sb.WriteString(op + " " + name)
	if len(f.Args) > 0 {
		defs := make([]string, len(f.Args))
		for i, a := range f.Args {
			defs[i] = "$" + a.Name + ": " + a.Type.String()
		}
		sb.WriteString("(" + strings.Join(defs, ", ") + ")")
	}
	sb.WriteString(" {\n  " + f.Name)
	if len(f.Args) > 0 {
		uses := make([]string, len(f.Args))
		for i, a := range f.Args {
			uses[i] = a.Name + ": $" + a.Name
		}
		sb.WriteString("(" + strings.Join(uses, ", ") + ")")
	}
	sb.WriteString(s.selection(f.Type, "  ", 1))
	sb.WriteString("\n}\n")

	if len(f.Args) > 0 {
		var vb strings.Builder
		vb.WriteString("{\n")
		for i, a := range f.Args {
			comma := ","
			if i == len(f.Args)-1 {
				comma = ""
			}
			fmt.Fprintf(&vb, "  %q: %s%s\n", a.Name, placeholder(a.Type), comma)
		}
		vb.WriteString("}")
		variables = vb.String()
	}
	return sb.String(), variables
}

// selection writes " { scalar fields }" for object types, or nothing for
// scalars. depth limits how far nested objects are expanded.
func (s *Schema) selection(t *TypeRef, indent string, depth int) string {
	named := t.Named()
	if named == nil {
		return ""
	}
	obj := s.Types[named.Name]
	if obj == nil || (obj.Kind != "OBJECT" && obj.Kind != "INTERFACE") {
		return ""
	}
	var lines []string
	for _, f := range obj.Fields {
		if len(f.Args) > 0 && requiresArgs(f.Args) {
			continue
		}
		ft := f.Type.Named()
		if ft == nil {
			continue
		}
		switch ft.Kind {
		case "SCALAR", "ENUM":
			lines = append(lines, indent+"  "+f.Name)
		case "OBJECT", "INTERFACE":
			if depth > 0 {
				if sub := s.selection(f.Type, indent+"  ", depth-1); sub != "" {
					lines = append(lines, indent+"  "+f.Name+sub)
				}
			}
		}
	}
	if len(lines) == 0 {
		if obj.Kind == "OBJECT" {
			return " {\n" + indent + "  __typename\n" + indent + "}"
		}
		return ""
	}
	return " {\n" + strings.Join(lines, "\n") + "\n" + indent + "}"
}

func requiresArgs(args []Arg) bool {
	for _, a := range args {
		if a.Type != nil && a.Type.Kind == "NON_NULL" && a.DefaultValue == nil {
			return true
		}
	}
	return false
}

// placeholder is an example value for an argument of type t.
func placeholder(t *TypeRef) string {
	if t != nil && t.Kind == "NON_NULL" {
		t = t.OfType
	}
	if t == nil {
		return "null"
	}
	if t.Kind == "LIST" {
		return "[]"
	}
	switch t.Name {
	case "Int", "Float":
		return "0"
	case "Boolean":
		return "false"
	case "String", "ID":
		return `""`
	}
	return "null"
}
