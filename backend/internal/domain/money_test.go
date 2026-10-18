package domain

import "testing"

func TestMoney_ApplyPercent(t *testing.T) {
	tests := []struct {
		name    string
		m       Money
		percent int
		want    Money
	}{
		{"5 percent of 240 THB", 24000, 5, 1200},
		{"10 percent of 228 THB", 22800, 10, 2280},
		{"exactly half a satang rounds up", 10, 5, 1},
		{"below half a satang rounds down", 9, 5, 0},
		{"zero amount", 0, 10, 0},
		{"100 percent is the whole amount", 4000, 100, 4000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.ApplyPercent(tt.percent); got != tt.want {
				t.Fatalf("Money(%d).ApplyPercent(%d) = %d, want %d", tt.m, tt.percent, got, tt.want)
			}
		})
	}
}
