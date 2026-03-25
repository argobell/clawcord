package utils

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "ascii string truncated",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "unicode string truncated by runes",
			input:  "你好世界和平",
			maxLen: 5,
			want:   "你好...",
		},
		{
			name:   "non-positive length returns empty",
			input:  "hello",
			maxLen: 0,
			want:   "",
		},
		{
			name:   "very small max length skips ellipsis",
			input:  "hello",
			maxLen: 3,
			want:   "hel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Fatalf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
