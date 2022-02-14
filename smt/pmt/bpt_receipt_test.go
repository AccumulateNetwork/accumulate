package pmt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// returns true if valid, false if not
func chk(t *testing.T, entry Entry) bool {
	node, ok := entry.(*BptNode)
	if ok && node.Left != nil && node.Right != nil {
		c := sha256.Sum256(append(node.Left.GetHash(), node.Right.GetHash()...))
		if node.Hash != c {
			return false
		}
		return chk(t, node.Left) && chk(t, node.Right)
	} else if ok && node.Left != nil {
		if !bytes.Equal(node.Hash[:], node.Left.GetHash()) {
			return false
		}
		return chk(t, node.Left)
	} else if ok && node.Right != nil {
		if !bytes.Equal(node.Hash[:], node.Right.GetHash()) {
			return false
		}
		return chk(t, node.Right)
	}
	return true
}

func TestBPT_receipt(t *testing.T) {
	numberEntries := 10
	// Build a BPT
	bpt := NewBPT()

	for i := 0; i < numberEntries; i++ {
		chainID := sha256.Sum256([]byte(fmt.Sprint(i)))
		value := sha256.Sum256([]byte(fmt.Sprint(i, "v")))
		fmt.Printf("%4d %x:%3d:%08b v=%02x : %3d\n", i, chainID[0], chainID[0], chainID[0], value[0], value[0])
		bpt.Insert(chainID, value)
	}

	require.False(t, chk(t, bpt.Root), "should not work as updates are needed")

	bpt.Update()

	require.True(t, chk(t, bpt.Root), "should work, as updates have been done")

	for i := 0; i < numberEntries; i++ {
		chainID := sha256.Sum256([]byte(fmt.Sprint(i)))
		r := bpt.GetReceipt(chainID)
		require.NotNil(t, r, "Should not be nil")
		fmt.Println(r.String())
		require.Truef(t, r.Validate(), "should validate %d", i)
	}
}
