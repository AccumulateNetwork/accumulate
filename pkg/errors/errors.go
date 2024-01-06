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

// Delivered returns true if the status represents an executed transaction.
func (s Status) Delivered() bool { return s >= 300 || s == Delivered }

// IsKnownError returns true if the status is non-zero and not UnknownError.
func (s Status) IsKnownError() bool { return s != 0 && s != UnknownError }

// IsClientError returns true if the status is a server error.
func (s Status) IsClientError() bool { return s >= 400 && s < 500 }

// IsServerError returns true if the status is a server error.
func (s Status) IsServerError() bool { return s >= 500 }

// Error implements error.
func (s Status) Error() string { return s.String() }

// Skip skips N frames when locating the call site.
func (s Status) Skip(n int) Factory[Status] {
	return Factory[Status]{n, s, UnknownError, EncodingError}
}

func (s Status) Wrap(err error) error {
	return s.Skip(1).Wrap(err)
}

func (s Status) With(v ...interface{}) *Error {
	return s.Skip(1).With(v...)
}

func (s Status) WithFormat(format string, args ...interface{}) *Error {
	return s.Skip(1).WithFormat(format, args...)
}

func (s Status) WithCauseAndFormat(cause error, format string, args ...interface{}) *Error {
	return s.Skip(1).WithCauseAndFormat(cause, format, args...)
}

type Factory[Status statusType] struct {
	Skip         int
	Code         Status
	UnknownCode  Status
	EncodingCode Status
}

func (f Factory[Status]) Wrap(err error) error {
	if err == nil {
		// The return type must be `error` - otherwise this returns statement
		// can cause strange errors
		return nil
	}

	// If err is an Error and we're not going to add anything, return it
	if !trackLocation && !f.Code.IsKnownError() {
		if _, ok := err.(*ErrorBase[Status]); ok {
			return err
		}
	}

	e := f.new()
	e.setCause(f.convert(err))
	return e
}

func (f Factory[Status]) With(v ...interface{}) *ErrorBase[Status] {
	e := f.new()
	e.Message = fmt.Sprint(v...)
	return e
}

func (f Factory[Status]) WithCauseAndFormat(cause error, format string, args ...interface{}) *ErrorBase[Status] {
	e := f.new()
	e.Message = fmt.Sprintf(format, args...)
	e.setCause(f.convert(cause))
	return e
}

func (f Factory[Status]) WithFormat(format string, args ...interface{}) *ErrorBase[Status] {
	err := fmt.Errorf(format, args...)

	u, ok := err.(interface{ Unwrap() error })
	if ok {
		e := f.new()
		e.Message = err.Error()
		e.setCause(f.convert(u.Unwrap()))
		return e
	}

	e := f.convert(err)
	e.Code = f.Code
	e.recordCallSite(2 + f.Skip)
	return e
}

func (f Factory[Status]) new() *ErrorBase[Status] {
	e := new(ErrorBase[Status])
	e.Code = f.Code
	e.recordCallSite(3 + f.Skip)
	return e
}

func (f Factory[Status]) convert(err error) *ErrorBase[Status] {
	if x := (*ErrorBase[Status])(nil); errors.As(err, &x) {
		return x
	}
	var msg string
	if err == nil {
		msg = "(nil)"
	} else {
		msg = err.Error()
	}
	if x := Status(0); errors.As(err, &x) {
		return &ErrorBase[Status]{Code: x, Message: msg}
	}

	e := &ErrorBase[Status]{
		Code:    f.UnknownCode,
		Message: msg,
	}

	var encErr encoding.Error
	if errors.As(err, &encErr) {
		if f.EncodingCode != 0 {
			e.Code = f.EncodingCode
		} else {
			e.Code = f.UnknownCode
		}
		err = encErr.E
	}

	if u, ok := err.(interface{ Unwrap() error }); ok {
		if err := u.Unwrap(); err != nil {
			e.setCause(f.convert(err))
		}
	}

	return e
}

func (e *ErrorBase[Status]) setCause(f *ErrorBase[Status]) {
	e.Cause = f
	if f == nil {
		return
	}

	if e.Code.IsKnownError() {
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

func (e *ErrorBase[Status]) recordCallSite(depth int) {
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

func (e *ErrorBase[Status]) CodeID() uint64 {
	return uint64(e.Code)
}

func (e *ErrorBase[Status]) Error() string {
	if e.Message == "" && e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Message
}

func (e *ErrorBase[Status]) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Code
}

func (e *ErrorBase[Status]) Format(f fmt.State, verb rune) {
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
func (e *ErrorBase[Status]) Print() string {
	// If the error has no call stack just return the message
	if e.CallStack == nil {
		return e.Error()
	}

	var str []string
	for e != nil {
		// Remove the suffix if the error is compound, as per the method
		// description
		msg := e.Message
		if msg == "" {
			msg = e.Code.String()
		} else if e.Cause != nil {
			msg = strings.TrimSuffix(msg, e.Cause.Message)
		}

		str = append(str, msg+"\n"+e.printCallstack())
		e = e.Cause
	}
	return strings.Join(str, "\n")
}

func (e *ErrorBase[Status]) printCallstack() string {
	var str string
	for _, cs := range e.CallStack {
		str += fmt.Sprintf("%s\n    %s:%d\n", cs.FuncName, cs.File, cs.Line)
	}
	return str
}

func (e *ErrorBase[Status]) PrintFullCallstack() string {
	var str string
	for e != nil {
		str += e.printCallstack()
		e = e.Cause
	}
	return str
}

func (e *ErrorBase[Status]) Is(target error) bool {
	switch f := target.(type) {
	case *ErrorBase[Status]:
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
