package managed_test

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gopkg.in/yaml.v3"
)

func TestDAGRoot(t *testing.T) {
	b, err := ioutil.ReadFile("../../testdata/merkle.yaml")
	require.NoError(t, err)

	var cases []*acctesting.MerkleTestCase
	require.NoError(t, yaml.Unmarshal(b, &cases))

	for _, c := range cases {
		var result managed.SparseHashList
		for _, e := range c.Cascade {
			result = append(result, managed.Hash(e))
		}
		t.Run(fmt.Sprintf("%X", c.Root[:4]), func(t *testing.T) {
			ms := new(managed.MerkleState)
			ms.InitSha256()
			for _, e := range c.Entries {
				ms.AddToMerkleTree(e)
			}
			// Pending may be padded
			for len(ms.Pending) > len(result) {
				result = append(result, nil)
			}
			require.Equal(t, result, ms.Pending)
			require.Equal(t, managed.Hash(c.Root), ms.GetMDRoot())
		})
	}
}
