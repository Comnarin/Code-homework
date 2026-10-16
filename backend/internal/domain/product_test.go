package domain

import "testing"

func TestNormalizeCard(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"trims spaces", "  MEM0001  ", "MEM0001"},
		{"uppercases", "mem0001", "MEM0001"},
		{"blank stays blank", "   ", ""},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeCard(tt.in); got != tt.want {
				t.Fatalf("NormalizeCard(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
