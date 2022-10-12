// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

func NotFound(format string, args ...interface{}) error {
	return Format(StatusNotFound, format, args...)
}

func Unknown(format string, args ...interface{}) error {
	return Format(StatusUnknownError, format, args...)
}
