// Package vars implements {{variable}} substitution.
package vars

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var pattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.$-]+)\s*\}\}`)

// Pattern matches a {{name}} reference; submatch 1 is the name.
var Pattern = pattern

// Now is the clock used by dynamic variables; tests replace it.
var Now = time.Now

// Dynamic lists the built-in variables that produce a fresh value each time.
var Dynamic = []string{"$timestamp", "$isoTimestamp", "$uuid", "$randomInt"}

// Substitute replaces {{name}} with values from vars and the built-in dynamic
// variables. Unknown names are left untouched and returned (sorted, unique)
// in missing.
func Substitute(text string, vars map[string]string) (result string, missing []string) {
	seen := map[string]bool{}
	result = pattern.ReplaceAllStringFunc(text, func(m string) string {
		name := pattern.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		if v, ok := dynamic(name); ok {
			return v
		}
		if !seen[name] {
			seen[name] = true
			missing = append(missing, name)
		}
		return m
	})
	sort.Strings(missing)
	return result, missing
}

func dynamic(name string) (string, bool) {
	switch name {
	case "$timestamp":
		return strconv.FormatInt(Now().Unix(), 10), true
	case "$isoTimestamp":
		return Now().UTC().Format(time.RFC3339), true
	case "$uuid":
		return newUUID(), true
	case "$randomInt":
		n, err := rand.Int(rand.Reader, big.NewInt(1001))
		if err != nil {
			return "0", true
		}
		return n.String(), true
	}
	return "", false
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
