package main

import (
	"testing"

	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestDataEntry(t *testing.T) {
	//txnTest(AccountUrl("adi"), &WriteData{Entry: &DoubleHashDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}})
	txnTest(AccountUrl("adi"), &WriteDataTo{Recipient: AccountUrl("lite-data-account"), Entry: &DoubleHashDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar")}}})

}
