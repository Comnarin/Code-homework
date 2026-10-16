package domain

// Money is an amount in satang (1/100 THB). Integers avoid float rounding errors.
type Money int64

// ApplyBP returns m x bp / 10000 rounded half-up. bp is basis points: 500 = 5%.
func (m Money) ApplyBP(bp int) Money {
	return Money((int64(m)*int64(bp) + 5000) / 10000)
}
