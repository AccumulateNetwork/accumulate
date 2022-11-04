package errors

// New is deprecated.
//
// Deprecated: Use code.With.
func New(code Status, v interface{}) *Error {
	return code.With(v)
}

// Wrap is deprecated.
//
// Deprecated: Use code.Wrap.
func Wrap(code Status, err error) error {
	return code.Wrap(err)
}

// FormatWithCause is deprecated.
//
// Deprecated: Use code.WithCauseAndFormat.
func FormatWithCause(code Status, cause error, format string, args ...interface{}) *Error {
	return code.WithCauseAndFormat(cause, format, args...)
}

// Format is deprecated.
//
// Deprecated: Use code.WithFormat.
func Format(code Status, format string, args ...interface{}) *Error {
	return code.WithFormat(format, args...)
}
