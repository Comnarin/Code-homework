package domain

import "testing"

func TestMoney_ApplyBP(t *testing.T) {
	tests := []struct {
		name string
		m    Money
		bp   int
		want Money
	}{
		{"5 percent of 240 THB", 24000, 500, 1200},
		{"10 percent of 228 THB", 22800, 1000, 2280},
		{"exactly half a satang rounds up", 10, 500, 1},
		{"below half a satang rounds down", 9, 500, 0},
		{"zero amount", 0, 1000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.ApplyBP(tt.bp); got != tt.want {
				t.Fatalf("Money(%d).ApplyBP(%d) = %d, want %d", tt.m, tt.bp, got, tt.want)
			}
		})
	}
}
