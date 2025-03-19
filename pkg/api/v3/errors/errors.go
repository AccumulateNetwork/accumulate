// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

import "errors"

var (
	// ErrUnexpectedResponse is returned when an unexpected response is received
	ErrUnexpectedResponse = errors.New("unexpected response")
)
