// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import "gitlab.com/accumulatenetwork/accumulate/pkg/errors"

// Error codes for mining operations
var (
	ErrNoActiveWindow        = errors.BadRequest.With("no active mining window")
	ErrWindowClosed          = errors.BadRequest.With("mining window is closed")
	ErrInsufficientDifficulty = errors.BadRequest.With("mining solution does not meet the minimum difficulty")
	ErrInvalidBlockHash      = errors.BadRequest.With("block hash in the mining solution is invalid")
	ErrInvalidSignature      = errors.BadRequest.With("mining signature is invalid")
)
