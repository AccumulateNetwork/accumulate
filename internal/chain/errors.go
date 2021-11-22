package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

// protocolError formats the error as a protocol error. If the code is 0, it will
// be determined from the error.
func protocolError(code protocol.ErrorCode, err error) *protocol.Error {
	switch {
	case code != 0:
		// OK

	case err == nil:
		return nil

	case errors.Is(err, storage.ErrNotFound):
		code = protocol.CodeNotFound

	default:
		code = protocol.CodeInternalError
	}

	return &protocol.Error{
		Code:    code,
		Message: err.Error(),
	}
}

// protocolErrorf formats the error as a protocol error. If the code is 0, it
// will be determined from the error.
func protocolErrorf(code protocol.ErrorCode, format string, args ...interface{}) *protocol.Error {
	return protocolError(code, fmt.Errorf(format, args...))
}
