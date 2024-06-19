// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import "io"

func closeIfError(err *error, closer io.Closer) {
	if *err != nil {
		_ = closer.Close()
	}
}
