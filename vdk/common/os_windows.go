// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package vdk

import "errors"

var ErrOnePassNotSupported = errors.New("1Password integration is not supported on macOS")

func canExecOnePass() error {
	return ErrOnePassNotSupported
}

func isOnePassWarn(err error) bool {
	return errors.Is(err, ErrOnePassNotSupported)
}
