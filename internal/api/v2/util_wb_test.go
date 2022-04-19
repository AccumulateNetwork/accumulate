package api

import "gitlab.com/accumulatenetwork/accumulate/protocol"

// Whitebox testing utilities

type Package struct{}

func (Package) ConstructFaucetTxn(req *protocol.AcmeFaucet) (*TxRequest, []byte, error) {
	return constructFaucetTxn(req)
}

func (Package) ProcessExecuteRequest(req *TxRequest, payload []byte) (*protocol.Envelope, error) {
	return processExecuteRequest(req, payload)
}
