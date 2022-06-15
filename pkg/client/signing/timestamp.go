package signing

import "sync/atomic"

type Timestamp interface {
	Get() (uint64, error)
}

type TimestampFromValue uint64
type TimestampFromVariable uint64

func (t TimestampFromValue) Get() (uint64, error) {
	return uint64(t), nil
}

func (t *TimestampFromVariable) Get() (uint64, error) {
	return atomic.AddUint64((*uint64)(t), 1), nil
}
