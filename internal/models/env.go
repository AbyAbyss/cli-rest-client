package models

import (
	"strconv"
	"strings"
)

// Environment is a named set of variables, like a Postman environment.
type Environment struct {
	Name      string     `json:"name"`
	Variables []KeyValue `json:"variables,omitempty"`
}

// Active returns the active environment, or nil for "No Environment".
func (w *Workspace) Active() *Environment {
	return w.Environment(w.ActiveEnvironment)
}

// Environment finds an environment by name (case-insensitive).
func (w *Workspace) Environment(name string) *Environment {
	if name == "" {
		return nil
	}
	for _, e := range w.Environments {
		if strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return nil
}

// SetActive switches environment; "" (or an unknown name) means none.
func (w *Workspace) SetActive(name string) {
	if e := w.Environment(name); e != nil {
		w.ActiveEnvironment = e.Name
	} else {
		w.ActiveEnvironment = ""
	}
}

// UniqueEnvName returns base, or base with a number appended, so that it
// doesn't clash with an existing environment.
func (w *Workspace) UniqueEnvName(base string) string {
	name := base
	for i := 2; w.Environment(name) != nil; i++ {
		name = base + " " + strconv.Itoa(i)
	}
	return name
}

// Clone returns a copy of e with its own variable list.
func (e *Environment) Clone() *Environment {
	return &Environment{Name: e.Name, Variables: append([]KeyValue(nil), e.Variables...)}
}
