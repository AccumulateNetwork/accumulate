// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package managed

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

const MSize = int64(200) // Use a Merkle Tree size
const ListLen = 10       // Test lists equal to or less than 10
const Offset = 0         // Offset the lists starting at this index in the Merkle Tree
var rh, rh2 common.RandHash

func loadMerkleTrees(t *testing.T) (manager, manager2 *Chain) {

	rh2.SetSeed([]byte{1, 2, 3})

	store := begin()                    // Create a memory database
	manager = testChain(store, 4, 1)    // Create a test chain
	manager2 = testChain(store, 4, 2)   // and create another
	for i := int64(0); i < MSize; i++ { // Now put MSize elements in the first test chain
		v := rh.NextList()                            // Get a random hash
		require.NoError(t, manager.AddHash(v, false)) //   and put it in the first chain
		ms, err := manager.Head().Get()               // Get the current merkle state
		require.NoError(t, err)                       //

		require.NoError(t, manager2.AddHash(ms.GetMDRoot().Bytes(), false)) // Then anchor the first chain into the second
		for i := 0; i < int(rh2.GetRandInt64())%10; i++ {                   // Mix some non matching hashes in.
			require.NoError(t, manager2.AddHash(rh2.NextList(), false)) // Then anchor the first chain into the second
		}
	}
	MS, _ := manager.Head().Get()
	MS2, _ := manager2.Head().Get()
	_, _ = MS, MS2
	return manager, manager2
}
func TestReceiptList(t *testing.T) {
	var start float64
	var cnt int // Count of hashes validated
	manager, manager2 := loadMerkleTrees(t)
	start = float64(time.Now().UnixMilli()) / 1000

	for startTest := int64(Offset); startTest < MSize; startTest++ { // Test a range
		for endTest := startTest; endTest < startTest+ListLen && endTest < MSize; endTest++ {

			_, err := manager.Get(startTest) // Get the first element of a test of a list
			require.NoError(t, err)

			receiptList, err := GetReceiptList(manager, startTest, endTest) // Get a receipt list
			require.NoError(t, err, startTest)                              // It should work
			require.NotNil(t, receiptList)                                  // It should return a receiptList
			anchor := receiptList.Receipt.Anchor                            // Get the anchor1 that
			cnt++
			for i := startTest; i <= endTest; i++ { //            Test every list we can make for the given range
				element := rh.List[i]                          // Get an element
				cnt++                                          // Count validations
				require.True(t, receiptList.Included(element)) // Check that the element is included in the receiptList
			}

			receipt, err := GetReceipt(manager2, anchor, anchor) // Build a receipt of the anchor in the second chain
			require.NoError(t, err)                              //
			require.NotNil(t, receipt)                           //
			require.True(t, receipt.Validate())                  // Make sure the first receipt validates
			receiptList.ContinuedReceipt = receipt               // Now use the combined receipt in the receiptList
			require.True(t, receiptList.Validate(), startTest)   // Test the modified receiptList

			cnt++
			for i := startTest; i <= endTest; i++ { //            Rerun the test of every list we can make for the given range
				element := rh.List[i]                          // Note we are testing a combined receipt here.  Get an element
				require.True(t, receiptList.Included(element)) // The Element must be included in the ReceiptList
				cnt++
			}

			// Now we check that you can't mess with any of the elements, nor any of
			// the receipts and have the ReceiptList validate.  We flip one bit on
			// each element in the receiptList and confirm that makes the ReceiptList fault
			//
			// Then we check that you can't flip even one bit in the entries in the receipt
			// without making the ReceiptList fault
			for i, e := range receiptList.Elements {
				_ = e
				receiptList.Elements[i][0] ^= 1 // Flip a bit in the Elements
				require.False(t, receiptList.Validate())
				receiptList.Elements[i][0] ^= 1 // Flip a bit in the Elements
			}
			for i := range receiptList.Receipt.Entries {
				receiptList.Receipt.Entries[i].Hash[0] ^= 1
				require.False(t, receiptList.Validate())
				receiptList.Receipt.Entries[i].Hash[0] ^= 1
			}
			for i := range receiptList.ContinuedReceipt.Entries {
				receiptList.ContinuedReceipt.Entries[i].Hash[0] ^= 1
				require.False(t, receiptList.Validate())
				receiptList.ContinuedReceipt.Entries[i].Hash[0] ^= 1
			}

		}
	}
	end := float64(time.Now().UnixMilli())/1000 - start
	fmt.Printf("Cnt %d  Seconds %3.2f\n", cnt, end)
}
