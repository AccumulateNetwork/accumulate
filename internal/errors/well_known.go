package errors

// Error implements error.
func (s Status) Error() string {
	return s.String()
}

func NotFound(format string, args ...interface{}) error {
	e := make(StatusNotFound)
	e.errorf(format, args...)
	return e
}

func Unknown(format string, args ...interface{}) error {
	e := make(StatusUnknown)
	e.errorf(format, args...)
	return e
}
