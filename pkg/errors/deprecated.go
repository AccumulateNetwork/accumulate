// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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
