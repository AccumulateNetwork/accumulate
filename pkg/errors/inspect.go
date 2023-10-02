// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

import "errors"

// As calls stdlib errors.As.
func As(err error, target interface{}) bool { return errors.As(err, target) }

// Is calls stdlib errors.Is.
func Is(err, target error) bool { return errors.Is(err, target) }

// Unwrap calls stdlib errors.Unwrap.
func Unwrap(err error) error { return errors.Unwrap(err) }

// Code returns the status code if the error is an [Error], or 0.
func Code(err error) Status {
	var err2 *ErrorBase[Status]
	if !As(err, &err2) {
		return 0
	}
	for err2.Code == UnknownError && err2.Cause != nil {
		err2 = err2.Cause
	}
	return err2.Code
}
