// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/test/testdata"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	"gopkg.in/yaml.v3"
)

func TestDAGRoot(t *testing.T) {
	var cases []*acctesting.MerkleTestCase
	require.NoError(t, yaml.Unmarshal([]byte(testdata.Merkle), &cases))

	for _, c := range cases {
		var result database.SparseHashList
		for _, e := range c.Cascade {
			result = append(result, database.Hash(e))
		}
		t.Run(fmt.Sprintf("%X", c.Root[:4]), func(t *testing.T) {
			ms := new(merkle.State)
			for _, e := range c.Entries {
				ms.AddEntry(e)
			}
			// Pending may be padded
			for len(ms.Pending) > len(result) {
				result = append(result, nil)
			}
			require.Equal(t, [][]byte(result), ms.Pending)
			require.Equal(t, []byte(c.Root), ms.Anchor())
		})
	}
}
