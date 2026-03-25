package utils

// Truncate 按 rune 数截断字符串，必要时在尾部追加 "...".
// 这样可以避免直接按字节截断导致的 Unicode 字符损坏。
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
