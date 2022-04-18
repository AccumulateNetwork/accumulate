package protocol

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

var ErrNotEnoughData = encoding.ErrNotEnoughData
var ErrOverflow = encoding.ErrOverflow

type ErrorCode int

type Error struct {
	Code    ErrorCode
	Message error
}

var _ error = (*Error)(nil)

func NewError(code ErrorCode, err error) *Error {
	if err, ok := err.(*Error); ok {
		return err
	}

	if errors.Is(err, storage.ErrNotFound) {
		return &Error{ErrorCodeNotFound, err}
	}

	return &Error{code, err}
}

func Errorf(code ErrorCode, format string, args ...interface{}) *Error {
	return NewError(code, fmt.Errorf(format, args...))
}

func (err *Error) Error() string {
	return err.Message.Error()
}

func (err *Error) Unwrap() error {
	return err.Message
}

// ConvertErrorStatus converts a status code to an old protocol error code.
//
// Protocol error codes are deprecated and should be converted to status codes.
func ConvertErrorStatus(s errors.Status) ErrorCode {
	switch s {
	case errors.StatusOK, errors.StatusDelivered, errors.StatusPending, errors.StatusRemote:
		return 0

	case errors.StatusBadRequest:
		return ErrorCodeInvalidRequest
	case errors.StatusUnauthenticated:
		return ErrorCodeInvalidSignature
	case errors.StatusInsufficientCredits:
		return ErrorCodeInsufficientCredits
	case errors.StatusUnauthorized:
		return ErrorCodeUnauthorized
	case errors.StatusNotFound:
		return ErrorCodeNotFound

	default:
		return ErrorCodeUnknownError
	}
}
