package refactor

import "github.com/AccumulateNetwork/SMT/common"

// TokenSend
// Send Tokens from the SourceURL to one or more DestinationURL.  Note that
// we don't have to define the tokens sent because the fees are implied and
// separate from the sends.
type TokenSend struct {
	AccountURL string     // URL of the token chain
	Outputs    []struct { // The set of outputs for distribution of tokens
		Amount         int64  // The amount sent
		DestinationURL string // The destination URL
	}
}

// Marshal
// Marshal a transaction
func (t TokenSend) Marshal() []byte {
	data := common.Uint64Bytes(TokenTX)                                //      The Transaction type
	data = append(data, common.SliceBytes([]byte(t.AccountURL))...)    //      The source URL
	data = append(data, common.Uint64Bytes(uint64(len(t.Outputs)))...) //      The number of outputs
	for _, v := range t.Outputs {                                      //      For each output
		data = append(data, common.Uint64Bytes(uint64(v.Amount))...)        //   the amount sent
		data = append(data, common.SliceBytes([]byte(v.DestinationURL))...) //   the destination for the amount
	}
	return data
}
