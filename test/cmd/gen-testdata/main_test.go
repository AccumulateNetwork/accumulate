// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"

	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestDataEntry(t *testing.T) {
	//txnTest(AccountUrl("adi"), &WriteData{Entry: &DoubleHashDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}})
	txnTest(AccountUrl("adi"), &WriteDataTo{Recipient: AccountUrl("lite-data-account"), Entry: &DoubleHashDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar")}}})

}
