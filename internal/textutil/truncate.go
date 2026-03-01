package textutil

import "strings"

// Truncate returns s unchanged if len(s) <= maxLen (measured in bytes).
// Otherwise it cuts at maxLen, walks back to avoid splitting a multi-byte
// UTF-8 sequence, and appends suffix.
func Truncate(s string, maxLen int, suffix string) string {
	if len(s) <= maxLen {
		return s
	}
	cut := maxLen
	for cut > 0 && s[cut]>>6 == 0b10 {
		cut--
	}
	return s[:cut] + suffix
}

// SanitizeJSON fixes common JSON issues in LLM output: literal control
// characters inside string values and invalid escape sequences like \s
// (JSON only allows \", \\, \/, \b, \f, \n, \r, \t, \uXXXX).
func SanitizeJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false
	for _, ch := range s {
		if escaped {
			escaped = false
			if !validJSONEscape(ch) {
				b.WriteRune('\\')
			}
			b.WriteRune(ch)
			continue
		}
		if ch == '\\' && inString {
			b.WriteRune(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			b.WriteRune(ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				b.WriteString("\\n")
				continue
			case '\r':
				b.WriteString("\\r")
				continue
			case '\t':
				b.WriteString("\\t")
				continue
			}
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func validJSONEscape(ch rune) bool {
	switch ch {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	}
	return false
}
