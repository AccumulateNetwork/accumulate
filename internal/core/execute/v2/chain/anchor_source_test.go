// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An anchor body without a source is refused before the source is read
// (#4572).
func TestAnchorWithoutSourceIsRefused(t *testing.T) {
	st := new(StateManager)
	st.NetworkType = protocol.PartitionTypeDirectory

	for name, c := range map[string]struct {
		exec TransactionExecutor
		body protocol.TransactionBody
	}{
		"directory anchor": {DirectoryAnchor{}, &protocol.DirectoryAnchor{}},
		"partition anchor": {PartitionAnchor{}, &protocol.BlockValidatorAnchor{}},
	} {
		t.Run(name, func(t *testing.T) {
			tx := &Delivery{Transaction: &protocol.Transaction{Body: c.body}}
			require.NotPanics(t, func() {
				_, err := c.exec.Validate(st, tx)
				require.ErrorContains(t, err, "missing source")
			})
		})
	}
}
