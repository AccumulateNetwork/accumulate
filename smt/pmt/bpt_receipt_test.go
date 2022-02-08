package pmt

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestBPT_receipt(t *testing.T) {
	numberEntries := 10
	// Build a BPT
	bpt := NewBPT()

	for i:=0;i<numberEntries; i++{
		chainID := sha256.Sum256([]byte(fmt.Sprint(i)))
		value := sha256.Sum256([]byte(fmt.Sprint(i,"v")))
		bpt.Insert(chainID,value)
	}
}