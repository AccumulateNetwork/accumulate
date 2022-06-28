package errors

// Error implements error.
func (s Status) Error() string {
	return s.String()
}

func NotFound(format string, args ...interface{}) error {
	e := makeError(StatusNotFound)
	e.errorf(format, args...)
	return e
}

func Unknown(format string, args ...interface{}) error {
	e := makeError(StatusUnknownError)
	e.errorf(format, args...)
	return e
}
