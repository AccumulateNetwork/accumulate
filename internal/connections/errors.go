package connections

import (
	"errors"
)

var ErrUnknownSubnet = errors.New("unknown subnet")
var NoHealthyNodes = errors.New("no healthy nodes available")
