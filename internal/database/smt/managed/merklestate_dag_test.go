// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package managed

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/test/testdata"
	"gopkg.in/yaml.v3"
)

func TestDAGRoot(t *testing.T) {
	var cases []*MerkleTestCase
	require.NoError(t, yaml.Unmarshal([]byte(testdata.Merkle), &cases))

	for _, c := range cases {
		var result SparseHashList
		for _, e := range c.Cascade {
			result = append(result, Hash(e))
		}
		t.Run(fmt.Sprintf("%X", c.Root[:4]), func(t *testing.T) {
			ms := new(MerkleState)
			ms.InitSha256()
			for _, e := range c.Entries {
				ms.AddToMerkleTree(e)
			}
			// Pending may be padded
			for len(ms.Pending) > len(result) {
				result = append(result, nil)
			}
			require.Equal(t, result, ms.Pending)
			require.Equal(t, Hash(c.Root), ms.GetMDRoot())
		})
	}
}
