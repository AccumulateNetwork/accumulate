package errors

import (
	"errors"
	"fmt"
	"runtime"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

// New returns a new Error with values sprint-formatted.
func (code Status) New(values ...interface{}) *Error {
	e := makeError(code)
	e.Message = fmt.Sprint(values...)
	return e
}

// Wrap returns a new Error wrapping the given error. If err is nil, Wrap
// returns nil; otherwise Wrap always returns an *Error.
//
// If location tracking is disabled, the code is UnknownError, and err is an
// *Error, Wrap returns err unchanged.
func (code Status) Wrap(err error) error {
	if err == nil {
		// The return type must be `error` - otherwise this returns statement
		// can cause strange errors
		return nil
	}

	// If err is an Error and we're not going to add anything, return it
	if !trackLocation && code == StatusUnknownError {
		if _, ok := err.(*Error); ok {
			return err
		}
	}

	e := makeError(code)
	e.setCause(convert(err))
	return e
}

// Format returns a new Error with format and values errorf-formatted.
//
// If the format contains %w (and the corresponding value is an error), that
// error is used as the cause of the returned Error.
func (code Status) Format(format string, values ...interface{}) *Error {
	err := fmt.Errorf(format, values...)

	u, ok := err.(interface{ Unwrap() error })
	if ok {
		e := makeError(code)
		e.Message = err.Error()
		e.setCause(convert(u.Unwrap()))
		return e
	}

	e := convert(err)
	e.Code = code
	e.recordCallSite(2)
	return e
}

// FormatWithCause returns a new Error with the given cause and format and
// values sprintf-formatted.
func (code Status) FormatWithCause(cause error, format string, values ...interface{}) *Error {
	e := makeError(code)
	e.Message = fmt.Sprintf(format, values...)
	e.setCause(convert(cause))
	return e
}

func makeError(code Status) *Error {
	e := new(Error)
	e.Code = code
	e.recordCallSite(3)
	return e
}

func convert(err error) *Error {
	switch err := err.(type) {
	case *Error:
		return err
	case Status:
		return &Error{Code: err, Message: err.Error()}
	}

	e := &Error{
		Code:    StatusUnknownError,
		Message: err.Error(),
	}

	var encErr encoding.Error
	if errors.As(err, &encErr) {
		e.Code = StatusEncodingError
		err = encErr.E
	}

	u, ok := err.(interface{ Unwrap() error })
	if ok {
		e.setCause(convert(u.Unwrap()))
	}

	return e
}

func (e *Error) setCause(f *Error) {
	e.Cause = f
	if f == nil {
		return
	}

	if e.Code != StatusUnknownError {
		return
	}

	if e.Message != "" {
		// Copy the code
		e.Code = f.Code
		return
	}

	// Inherit everything
	cs := e.CallStack
	*e = *f
	e.CallStack = append(cs, f.CallStack...)
}

func (e *Error) recordCallSite(depth int) {
	if !trackLocation {
		return
	}

	pc, file, line, ok := runtime.Caller(depth)
	if !ok {
		return
	}

	cs := &CallSite{File: file, Line: int64(line)}
	fn := runtime.FuncForPC(pc)
	if fn != nil {
		cs.FuncName = fn.Name()
	}

	e.CallStack = append(e.CallStack, cs)
}
