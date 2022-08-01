package managed

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

func TestReceiptList(t *testing.T) {
	const MSize = int64(20)                                     // Use a Merkle Tree size
	for startTest := int64(0); startTest < MSize; startTest++ { // Test a range
		for endTest := startTest; endTest < startTest+10 && endTest < MSize; endTest++ {

			var rb common.RandHash

			store := begin()                    // Create a memory database
			manager := testChain(store, 4)      // Create a test chain
			manager2 := testChain2(store, 4)    // and create another
			for i := int64(0); i < MSize; i++ { // Now put MSize elements in the first test chain
				v := rb.NextList()                                                  // Get a random hash
				require.NoError(t, manager.AddHash(v, false))                       //   and put it in the first chain
				ms, err := manager.Head().Get()                                     // Get the current merkle state
				require.NoError(t, err)                                             //
				require.NoError(t, manager2.AddHash(ms.GetMDRoot().Bytes(), false)) // Then anchor the first chain into the second
			}

			element, err := manager.Get(startTest) // Get the first element of a test of a list
			require.NoError(t, err)

			receiptList, err := GetReceiptList(manager, element, startTest, endTest) // Get a receipt list
			require.NoError(t, err)                                                  // It should work
			require.NotNil(t, receiptList)                                           // It should return a receiptList
			require.True(t, receiptList.Validate())                                  // And the receiptList should validate
			anchor1 := receiptList.Receipt.Anchor                                    // Get the anchor1 that

			for i := startTest; i <= endTest; i++ { //     Test every list we can make for the given range
				element, err := manager.Get(i)          // Get an element
				require.NoError(t, err)                 //
				receiptList.Element = element           // Stuff it into the receiptList
				require.NoError(t, err)                 // This should work, because we look for the element in the list
				require.NotNil(t, receiptList)          // and the list Receipt is proof of all its elements
				require.True(t, receiptList.Validate()) //
			}

			receipt, err := GetReceipt(manager2, anchor1, anchor1)         // Build a receipt of the anchor in the second chain
			require.NoError(t, err)                                        //
			require.NotNil(t, receipt)                                     //
			receipt2, err := CombineReceipts(receiptList.Receipt, receipt) // Combine the receipts
			require.NoError(t, err)                                        //
			require.NotNil(t, receipt2)                                    //
			receiptList.Receipt = receipt2                                 // Now use the combined receipt in the receiptList

			require.True(t, receipt2.Validate()) //                            Test the receipt for grins.

			for i := startTest; i <= endTest; i++ { //     Rerun the test of every list we can make for the given range
				element, err := manager.Get(i)          // Note we are testing a combined receipt here.  Get an element
				require.NoError(t, err)                 //
				receiptList.Element = element           // Stuff the element into the receiptList
				require.NoError(t, err)                 //
				require.NotNil(t, receiptList)          //
				require.True(t, receiptList.Validate()) // Check that the receipt continues to validate.  This goes to the
			}

		}
	}
}
