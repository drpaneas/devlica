package textutil

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		max    int
		suffix string
		want   string
	}{
		{
			name:   "shorter than max",
			input:  "hello",
			max:    10,
			suffix: "...",
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			max:    5,
			suffix: "...",
			want:   "hello",
		},
		{
			name:   "truncate ascii",
			input:  "hello world",
			max:    5,
			suffix: "...",
			want:   "hello...",
		},
		{
			name:   "empty input",
			input:  "",
			max:    10,
			suffix: "...",
			want:   "",
		},
		{
			name:   "max zero",
			input:  "hello",
			max:    0,
			suffix: "...",
			want:   "...",
		},
		{
			name:   "two-byte utf8 not split",
			input:  "ab\xc3\xa9cd", // "abécd" - é is 2 bytes
			max:    3,              // lands on the second byte of é
			suffix: "!",
			want:   "ab!",
		},
		{
			name:   "three-byte utf8 not split",
			input:  "a\xe2\x82\xacb", // "a€b" - € is 3 bytes
			max:    2,                 // lands inside €
			suffix: "!",
			want:   "a!",
		},
		{
			name:   "four-byte utf8 not split",
			input:  "a\xf0\x9f\x98\x80b", // "a😀b" - 😀 is 4 bytes
			max:    3,                      // lands inside 😀
			suffix: "!",
			want:   "a!",
		},
		{
			name:   "cut at utf8 boundary is clean",
			input:  "a\xc3\xa9b", // "aéb"
			max:    1,
			suffix: "!",
			want:   "a!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.input, tt.max, tt.suffix)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d, %q) = %q, want %q",
					tt.input, tt.max, tt.suffix, got, tt.want)
			}
		})
	}
}
