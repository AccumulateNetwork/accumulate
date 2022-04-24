package errors

import (
	"errors"
	"fmt"
	"runtime"
)

// As calls stdlib errors.As.
func As(err error, target interface{}) bool { return errors.As(err, target) }

// Is calls stdlib errors.Is.
func Is(err, target error) bool { return errors.Is(err, target) }

// Unwrap calls stdlib errors.Unwrap.
func Unwrap(err error) error { return errors.Unwrap(err) }

func makeError(code Status) *Error {
	e := new(Error)
	e.Code = code
	cs := callSite(3)
	if cs != nil {
		e.CallStack = []*CallSite{cs}
	}
	return e
}

func (e *Error) errorf(format string, args ...interface{}) {
	err := fmt.Errorf(format, args...)
	e.Message = err.Error()

	if u, ok := err.(interface{ Unwrap() error }); ok {
		e.setCause(convert(u.Unwrap()))
	}
}

func convert(err error) *Error {
	e, ok := err.(*Error)
	if ok {
		return e
	}

	e = &Error{Message: err.Error()}

	u, ok := err.(interface{ Unwrap() error })
	if ok {
		e.Cause = convert(u.Unwrap())
	}

	return e
}

func (e *Error) setCause(f *Error) {
	e.Cause = f
	if f == nil {
		return
	}

	if e.Code != StatusUnknown {
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

func New(code Status, v interface{}) *Error {
	if v == nil {
		return nil
	}

	e := makeError(code)
	if err, ok := v.(error); ok {
		e.setCause(convert(err))
	} else {
		e.Message = fmt.Sprint(v)
	}
	return e
}

func Wrap(code Status, err error) *Error {
	if err == nil {
		return nil
	}
	e := makeError(code)
	e.setCause(convert(err))
	return e
}

func Format(code Status, format string, args ...interface{}) *Error {
	err := fmt.Errorf(format, args...)

	u, ok := err.(interface {
		Unwrap() error
	})
	if ok {
		e := makeError(code)
		e.Message = err.Error()
		e.setCause(convert(u.Unwrap()))
		return e
	}

	e := convert(err)
	e.Code = code
	e.CallStack = []*CallSite{callSite(2)}
	return e
}

func callSite(depth int) *CallSite {
	if !trackLocation {
		return nil
	}

	pc, file, line, ok := runtime.Caller(depth)
	if !ok {
		return nil
	}

	cs := &CallSite{File: file, Line: int64(line)}
	fn := runtime.FuncForPC(pc)
	if fn != nil {
		cs.FuncName = fn.Name()
	}

	return cs
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

func (e *Error) Print() string {
	str := e.Message + "\n"
	for e != nil {
		for _, cs := range e.CallStack {
			str += fmt.Sprintf("%s\n    %s:%d\n", cs.FuncName, cs.File, cs.Line)
		}
		e = e.Cause
	}
	return str
}
