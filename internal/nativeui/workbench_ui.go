package nativeui

// pluralS returns "s" for counts other than exactly one.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
