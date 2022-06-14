package hash_test

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gopkg.in/yaml.v3"
)

func TestMerkleCascade(t *testing.T) {
	b, err := ioutil.ReadFile("../../../testdata/merkle.yaml")
	require.NoError(t, err)

	var cases []*managed.MerkleTestCase
	require.NoError(t, yaml.Unmarshal(b, &cases))

	for _, c := range cases {
		var entries [][]byte
		for _, e := range c.Entries {
			entries = append(entries, e)
		}
		var result [][]byte
		for _, e := range c.Cascade {
			result = append(result, e)
		}
		t.Run(fmt.Sprintf("%X", c.Root[:4]), func(t *testing.T) {
			cascade := hash.MerkleCascade(nil, entries, -1)
			require.Equal(t, result, cascade)
		})
	}
}
