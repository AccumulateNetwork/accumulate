// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Whitebox testing utilities

type Package struct{}

func (Package) ConstructFaucetTxn(req *protocol.AcmeFaucet) (*TxRequest, []byte, error) {
	return constructFaucetTxn(req)
}

func (Package) ProcessExecuteRequest(req *TxRequest, payload []byte) (*messaging.Envelope, error) {
	return processExecuteRequest(req, payload)
}
