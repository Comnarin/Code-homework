package domain

// Money is an amount in satang (1/100 THB). Integers avoid float rounding errors.
type Money int64

// ApplyPercent returns m x percent / 100 rounded half-up. percent is a whole
// number: 5 means 5%.
func (m Money) ApplyPercent(percent int) Money {
	return Money((int64(m)*int64(percent) + 50) / 100)
}
