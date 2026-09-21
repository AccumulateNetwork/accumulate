// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build debug

package indexing

import "gitlab.com/accumulatenetwork/accumulate/pkg/errors"

// debugMismatch fails loudly under -tags debug. A retained receipt that does not
// reach the entry means retention wrote the wrong thing, which is a defect worth
// stopping a test for — even though a production node degrades instead.
func debugMismatch(anchor, start []byte) error {
	return errors.InternalError.WithFormat(
		"the retained state receipt reaches %x but the proof starts at %x", anchor, start)
}
