package multipleDB

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test Updating Key Slices
func TestUpdate(t *testing.T) {
	ks := NewKeySlice(KeySliceDir)
	bFile, err := Open(Directory,Type,Partition,0)
	assert.NoError(t,err,"failed to add DBBlocks")
	ks.Update(bFile)
}