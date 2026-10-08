package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rivo/tview"
)

const (
	// maxDisplayBytes caps how much body text is put into the response view.
	maxDisplayBytes = 2 << 20
	// maxHighlightBytes caps syntax highlighting, which is slower.
	maxHighlightBytes = 512 << 10
)

// prettyBody returns the body formatted for display (with color tags) and
// whether it was recognised as JSON.
func prettyBody(body []byte, contentType string, t *Theme) (string, bool) {
	if len(body) == 0 {
		return fmt.Sprintf("[%s](empty body)[-]", t.HexMuted), false
	}
	truncated := false
	if len(body) > maxDisplayBytes {
		body = body[:maxDisplayBytes]
		truncated = true
	}
	suffix := ""
	if truncated {
		suffix = fmt.Sprintf("\n[%s]... display truncated at %s[-]", t.HexWarning, humanBytes(maxDisplayBytes))
	}

	trimmed := bytes.TrimSpace(body)
	looksJSON := strings.Contains(contentType, "json") ||
		(len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '['))
	if looksJSON && !truncated && json.Valid(trimmed) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, trimmed, "", "  "); err == nil {
			if buf.Len() <= maxHighlightBytes {
				return highlightJSON(buf.String(), t), true
			}
			return tview.Escape(buf.String()), true
		}
	}

	if !utf8.Valid(body) {
		return fmt.Sprintf("[%s](binary body, %s)[-]", t.HexMuted, humanBytes(len(body))), false
	}
	return tview.Escape(string(body)) + suffix, false
}

// highlightJSON adds color tags to already indented JSON.
func highlightJSON(s string, t *Theme) string {
	var sb strings.Builder
	sb.Grow(len(s) * 2)
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' {
					j += 2
					continue
				}
				if s[j] == '"' {
					break
				}
				j++
			}
			if j >= len(s) {
				j = len(s) - 1
			}
			str := s[i : j+1]
			// A string followed by ':' is an object key.
			k := j + 1
			for k < len(s) && s[k] == ' ' {
				k++
			}
			color := t.HexJSONString
			if k < len(s) && s[k] == ':' {
				color = t.HexJSONKey
			}
			sb.WriteString("[" + color + "]" + tview.Escape(str) + "[-]")
			i = j + 1
		case c == '-' || (c >= '0' && c <= '9'):
			j := i
			for j < len(s) && strings.IndexByte("+-0123456789.eE", s[j]) >= 0 {
				j++
			}
			sb.WriteString("[" + t.HexJSONNumber + "]" + s[i:j] + "[-]")
			i = j
		case strings.HasPrefix(s[i:], "true"), strings.HasPrefix(s[i:], "null"):
			sb.WriteString("[" + t.HexJSONLiteral + "]" + s[i:i+4] + "[-]")
			i += 4
		case strings.HasPrefix(s[i:], "false"):
			sb.WriteString("[" + t.HexJSONLiteral + "]false[-]")
			i += 5
		default:
			sb.WriteByte(c)
			i++
		}
	}
	return sb.String()
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
