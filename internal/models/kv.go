package models

import "strings"

// ParseKV parses one entry per line in "key<sep>value" form. Lines starting
// with '#' are kept as disabled entries. Blank lines are ignored. Keys and
// values are trimmed.
func ParseKV(text string, sep string) []KeyValue {
	var out []KeyValue
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		disabled := false
		if strings.HasPrefix(line, "#") {
			disabled = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if line == "" {
				continue
			}
		}
		key, value, _ := strings.Cut(line, sep)
		out = append(out, KeyValue{
			Key:      strings.TrimSpace(key),
			Value:    strings.TrimSpace(value),
			Disabled: disabled,
		})
	}
	return out
}

// FormatKV is the inverse of ParseKV. joiner is placed between key and value,
// e.g. ": " for headers or "=" for parameters.
func FormatKV(kvs []KeyValue, joiner string) string {
	var sb strings.Builder
	for i, kv := range kvs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if kv.Disabled {
			sb.WriteString("# ")
		}
		sb.WriteString(kv.Key)
		sb.WriteString(joiner)
		sb.WriteString(kv.Value)
	}
	return sb.String()
}
