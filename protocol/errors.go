package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/encoding"

var ErrNotEnoughData = encoding.ErrNotEnoughData
var ErrOverflow = encoding.ErrOverflow

type ErrorCode int

type Error struct {
	Code    ErrorCode
	Message error
}

var _ error = (*Error)(nil)

func (err *Error) Error() string {
	return err.Message.Error()
}

func (err *Error) Unwrap() error {
	return err.Message
}
