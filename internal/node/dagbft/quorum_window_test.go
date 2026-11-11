// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/primary"
)

// TestAStoppedValidatorHasAgedOutByTheFirstStallLine — #4610. A validator
// that authors the round the partition then stalls at, and stops, was heard
// after the partition's last block. The first stall line comes
// blockStallThreshold after that block, so it says the partition lost its
// quorum only if the stopped validator has stopped counting as producing by
// then, with time to spare for the rounds between the last block and its
// header. With the two windows equal it was still counted.
func TestAStoppedValidatorHasAgedOutByTheFirstStallLine(t *testing.T) {
	if 2*primary.HeardWithin > blockStallThreshold {
		t.Fatalf("a member counts as producing for %v after it was last heard, and the stall is reported %v after the last block: a validator stopped after the last block is still counted at the first stall line", primary.HeardWithin, blockStallThreshold)
	}
}
