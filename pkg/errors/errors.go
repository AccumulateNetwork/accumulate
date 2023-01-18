// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// Success returns true if the status represents success.
func (s Status) Success() bool { return s < 300 }

// IsClientError returns true if the status is a server error.
func (s Status) IsClientError() bool { return s >= 400 && s < 500 }

// IsServerError returns true if the status is a server error.
func (s Status) IsServerError() bool { return s >= 500 }

// Error implements error.
func (s Status) Error() string { return s.String() }

func (s Status) Wrap(err error) error {
	if err == nil {
		// The return type must be `error` - otherwise this returns statement
		// can cause strange errors
		return nil
	}

	// If err is an Error and we're not going to add anything, return it
	if !trackLocation && s == UnknownError {
		if _, ok := err.(*Error); ok {
			return err
		}
	}

	e := s.new()
	e.setCause(convert(err))
	return e
}

func (s Status) With(v ...interface{}) *Error {
	e := s.new()
	e.Message = fmt.Sprint(v...)
	return e
}

func (s Status) WithCauseAndFormat(cause error, format string, args ...interface{}) *Error {
	e := s.new()
	e.Message = fmt.Sprintf(format, args...)
	e.setCause(convert(cause))
	return e
}

func (s Status) WithFormat(format string, args ...interface{}) *Error {
	err := fmt.Errorf(format, args...)

	u, ok := err.(interface{ Unwrap() error })
	if ok {
		e := s.new()
		e.Message = err.Error()
		e.setCause(convert(u.Unwrap()))
		return e
	}

	e := convert(err)
	e.Code = s
	e.recordCallSite(2)
	return e
}

func (s Status) new() *Error {
	e := new(Error)
	e.Code = s
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
		Code:    UnknownError,
		Message: err.Error(),
	}

	var encErr encoding.Error
	if errors.As(err, &encErr) {
		e.Code = EncodingError
		err = encErr.E
	}

	if u, ok := err.(interface{ Unwrap() error }); ok {
		if err := u.Unwrap(); err != nil {
			e.setCause(convert(err))
		}
	}

	return e
}

func (e *Error) setCause(f *Error) {
	e.Cause = f
	if f == nil {
		return
	}

	if e.Code != UnknownError {
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

caller:
	pc, file, line, ok := runtime.Caller(depth)
	if !ok {
		return
	}

	// HACK: to get around call depth issues with pkg/errors/compat.go
	if strings.HasSuffix(file, "pkg/errors/compat.go") {
		depth++
		goto caller
	}

	cs := &CallSite{File: file, Line: int64(line)}
	fn := runtime.FuncForPC(pc)
	if fn != nil {
		cs.FuncName = fn.Name()
	}

	e.CallStack = append(e.CallStack, cs)
}

func (e *Error) CodeID() uint64 {
	return uint64(e.Code)
}

func (e *Error) Error() string {
	if e.Message == "" && e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Code
}

func (e *Error) Format(f fmt.State, verb rune) {
	if f.Flag('+') {
		f.Write([]byte(e.Print()))
	} else {
		f.Write([]byte(e.Error()))
	}
}

// Print prints an error message plus its call stack and causal chain. Compound
// errors are usually formatted as '<description>: <cause>'. Print will print
// this out as:
//
//	<description>:
//	<call stack>
//
//	<cause>
//	<call stack>
func (e *Error) Print() string {
	// If the error has no call stack just return the message
	if e.CallStack == nil {
		return e.Error()
	}

	var str []string
	for e != nil {
		// Remove the suffix if the error is compound, as per the method
		// description
		msg := e.Message
		if e.Cause != nil {
			msg = strings.TrimSuffix(msg, e.Cause.Message)
		}

		str = append(str, msg+"\n"+e.printCallstack())
		e = e.Cause
	}
	return strings.Join(str, "\n")
}

func (e *Error) printCallstack() string {
	var str string
	for _, cs := range e.CallStack {
		str += fmt.Sprintf("%s\n    %s:%d\n", cs.FuncName, cs.File, cs.Line)
	}
	return str
}

func (e *Error) PrintFullCallstack() string {
	var str string
	for e != nil {
		str += e.printCallstack()
		e = e.Cause
	}
	return str
}

func (e *Error) Is(target error) bool {
	switch f := target.(type) {
	case *Error:
		if e.Code == f.Code {
			return true
		}
	case Status:
		if e.Code == f {
			return true
		}
	}
	if e.Cause != nil {
		return e.Cause.Is(target)
	}
	return false
}
