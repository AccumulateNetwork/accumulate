// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

import (
	"bufio"
	"io"
)

func SendCommit(rd *bufio.Reader, wr io.Writer) error {
	_, err := roundTrip[*okResponse](rd, wr, &commitCall{})
	return err
}
