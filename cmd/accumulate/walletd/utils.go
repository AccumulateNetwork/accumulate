// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package walletd

import (
	"errors"
)

var ErrNotEnoughData = errors.New("not enough data")
var ErrOverflow = errors.New("overflow")
