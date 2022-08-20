package errors

// Error implements error.
func (s Status) Error() string {
	return s.String()
}

func NotFound(format string, args ...interface{}) error {
	return Format(StatusNotFound, format, args...)
}

func Unknown(format string, args ...interface{}) error {
	return Format(StatusUnknownError, format, args...)
}
