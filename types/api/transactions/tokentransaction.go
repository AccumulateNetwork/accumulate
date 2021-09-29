package transactions

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"net/url"

	"github.com/AccumulateNetwork/accumulated/smt/common"
)

// TokenSend
// Send Tokens from the SourceURL to one or more DestinationURL.  Note that
// we don't have to define the tokens sent because the fees are implied and
// separate from the sends.
type TokenSend struct {
	AccountURL string    // URL of the token chain
	Outputs    []*Output // The set of outputs for distribution of tokens
}

type Output struct { // Transaction output
	Amount uint64 //    number of tokens
	Dest   string //    url of the destination of the tokens
}

// Equal
// returns true if t == t2, otherwise return false.  Comparing t with t2, if
// any runtime error occurs we return false
func (t *TokenSend) Equal(t2 *TokenSend) (ret bool) {
	defer func() { //                      ret will default to false, so if any error occurs
	}() //                                 we will return false as long as we catch any errors
	if t.AccountURL != t2.AccountURL { //  Make sure accountURLs are the same
		return false
	}
	tLen := len(t.Outputs)                                 // Get our len
	if tLen != len(t2.Outputs) || tLen < 1 || tLen > 100 { // Make sure len is in range and same as t2
		return false //                                       If anything is different, function is false.
	}
	return true //                                           Only at the very end after all is done we return true
}

// Marshal
// Marshal a transaction
func (t *TokenSend) Marshal() []byte {
	data := common.Uint64Bytes(uint64(types.TxTypeTokenTx))            //      The Transaction type
	data = append(data, common.SliceBytes([]byte(t.AccountURL))...)    //      The source URL
	data = append(data, common.Uint64Bytes(uint64(len(t.Outputs)))...) //      The number of outputs
	for _, v := range t.Outputs {                                      //      For each output
		data = append(data, common.Uint64Bytes(uint64(v.Amount))...) //           the amount sent
		data = append(data, common.SliceBytes([]byte(v.Dest))...)    //           the destination for the amount
	}
	return data
}

// Unmarshal
// Unmarshal a transaction
func (t *TokenSend) Unmarshal(data []byte) ([]byte, error) { //                       Pull the state out of the data into the tx
	defer func() {
		if err := recover(); err != nil {
			err = fmt.Errorf("error marshaling Pending Transaction State %v", err)
		}
	}()

	tag, data := common.BytesUint64(data)      //       Get the tag
	if tag != types.TxTypeTokenTx.AsUint64() { //       Tag better be right
		panic(fmt.Sprintf("not a token transaction, expected %d got %d", // Blow up if not, and we don't have to save
			types.TxTypeTokenTx, tag)) //              the tag.
	} //
	var url []byte                       //                                 Working variable for computing urls
	var num uint64                       //                                 Working variable for getting output count
	url, data = common.BytesSlice(data)  //                                 Get the url
	t.AccountURL = string(url)           //                                 Get the balance
	num, data = common.BytesUint64(data) //                                 Get the count of outputs
	for i := uint64(0); i < num; i++ {   //                                 For all outputs
		output := new(Output)                          //                      Create an output
		output.Amount, data = common.BytesUint64(data) //                      with the amount
		url, data = common.BytesSlice(data)            //                      and a destination URL
		output.Dest = string(url)                      //
		t.Outputs = append(t.Outputs, output)          //                      Add the output
	} //
	return data, nil //                                                          Return the data
}

/*
func Sprint(a ...interface{}) string {
	p := newPrinter()
	p.doPrint(a)
	s := string(p.buf)
	p.free()
	return s
}
*/

// NewTokenSend
// Create a GenTransaction to send tokens
func NewTokenSend(sourceURL string, outputs ...interface{}) (tx *TokenSend) {
	defer func() { //                                       On an error, we return a nil
		if recover() != nil { //                            Check for any error
			tx = nil //
		} //
	}() //
	//
	tx = new(TokenSend)                             //      Create a TokenSend transaction.  be optimistic
	if _, err := url.Parse(sourceURL); err != nil { //      Make sure the source URL parses
		return nil //
	} //
	tx.AccountURL = sourceURL   //                          Set the source URL
	for _, o := range outputs { //                          Go through the outputs
		out := (o).(Output)                            //   Cast each output to an Output strct
		if _, err := url.Parse(out.Dest); err != nil { //   Parse the destination to make sure the URL is good
			return nil //                                   Return nil if it isn't.
		} //
		if out.Amount > 500000000*100000000 { // Sanity check the range of outputs.
			return nil //                                   1/2 billion not allowed to exist, much less in one address
		} //
		//
		tx.Outputs = append(tx.Outputs, &out) //          // Add all the good outputs
	} //
	return tx //                                            All that works, so return tx
}
