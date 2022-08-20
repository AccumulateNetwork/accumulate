package errors

import (
	"fmt"
	"strings"
)

// Error implements error.
func (s Status) Error() string {
	return s.String()
}

// Success returns true if the status represents success.
func (s Status) Success() bool { return s < 300 }

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

func (e *Error) Format(f fmt.State, verb rune) {
	if f.Flag('+') {
		f.Write([]byte(e.Print()))
	} else {
		f.Write([]byte(e.Error()))
	}
}

func (e *Error) Print() string {
	var str []string
	for e != nil {
		str = append(str, e.Message+"\n"+e.printCallstack())
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
