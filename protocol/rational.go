package protocol

func (r *Rational) Set(num, denom uint64) {
	r.Numerator, r.Denominator = num, denom
}

func (r *Rational) GetFloat() float64 {
	return float64(r.Numerator) / float64(r.Denominator)
}
