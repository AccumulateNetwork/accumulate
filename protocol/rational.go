package protocol

import "math"

func (r *Rational) Set(num, denom uint64) {
	r.Numerator, r.Denominator = num, denom
}

func (r *Rational) GetFloat() float64 {
	return float64(r.Numerator) / float64(r.Denominator)
}

// Threshold returns keyCount * num / denom rounded up.
func (r *Rational) Threshold(keyCount int) uint64 {
	v := float64(keyCount) * float64(r.Numerator) / float64(r.Denominator)
	return uint64(math.Ceil(v))
}
