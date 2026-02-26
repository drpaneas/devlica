package textutil

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
