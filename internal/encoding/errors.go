package encoding

import "errors"

var ErrNotEnoughData = errors.New("not enough data")
var ErrOverflow = errors.New("overflow")
