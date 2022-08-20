package encoding

import "errors"

var ErrNotEnoughData = errors.New("not enough data")
var ErrMalformedBigInt = errors.New("invalid big integer string")
var ErrOverflow = errors.New("overflow")
