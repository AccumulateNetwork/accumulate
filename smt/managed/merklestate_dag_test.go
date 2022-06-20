package managed

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDAGRoot(t *testing.T) {
	b, err := ioutil.ReadFile("../../testdata/merkle.yaml")
	require.NoError(t, err)

	var cases []*MerkleTestCase
	require.NoError(t, yaml.Unmarshal(b, &cases))

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
