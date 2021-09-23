package transactions

import (
	"fmt"

	"github.com/AccumulateNetwork/SMT/common"
)

// SignatureInfo
// The struct holds the URL, the nonce, and the signature indexes that are
// used to validate the signatures of a transaction.
//
// The transaction hash is what is signed.  This must be unique, or
// transactions could be replayed (breaking the security of the protocol)
//
// The following defines the roles each field plays to ensure transactions
// cannot be replayed.
//
// The URL forces the transaction onto the chain identified by the URL.
//
// The ChainID derived from the URL allows the chain to provide the
// SigSpecGroup (signature specification group) that applies to this chain.
//
// The SigSpecHt specifies the number of elements in the SigSpecGroup when
// the signature was generated.  Any change to the keys of the SigSpecGroup
// will increment the height, and if the transaction has not been promoted,
// the transaction will be invalided.
//
// The Priority will specify exactly which signature specification submitted
// the transaction
//
// The PriorityIdx will specify which of possibly multiple signatures signed
// the transaction submission.
//
// The nonce of the transaction submission must be equal to the nonce of
// the signature that submitted the transaction.
type SignatureInfo struct {
	// The following elements are all part of the Transaction that goes onto
	// the main chain.  But the only thing that varies from one transaction
	// to another is the transaction itself.
	URL         string // URL for the transaction
	SigSpecHt   uint64 // Height of the SigSpec Chain
	Priority    uint64 // Priority for the signature in the SigSpec
	PriorityIdx uint64 // Index within the Priority of the signature used
	Nonce       uint64 // Nonce for the signature to prevent replays
}

// Equal
// Largely used in testing, but allows the testing that one SignatureInfo
// is equal to another.  In testing we marshal and unmarshal a SignatureInfo
// and test that the information in the SignatureInfo is preserved
func (t *SignatureInfo) Equal(t2 *SignatureInfo) bool {
	return t.URL == t2.URL && //               URL equal?
		t.Nonce == t2.Nonce && //              Nonce equal?
		t.SigSpecHt == t2.SigSpecHt && //      SigSpecHt equal?
		t.Priority == t2.Priority && //        Priority equal?
		t.PriorityIdx == t2.PriorityIdx //     PriorityIdx equal?  If any fails, this returns false
}

// UnMarshal
// Create the binary representation of the GenTransaction
func (t *SignatureInfo) Marshal() (data []byte, err error) { //            Serialize the Signature Info
	defer func() { //                                                      If any problems are encountered, then
		if err := recover(); err != nil { //                               Complain
			err = fmt.Errorf("error marshaling GenTransaction %v", err) //
		} //
	}()

	data = common.SliceBytes([]byte(t.URL))                   //           URL =>
	data = append(data, common.Uint64Bytes(t.Nonce)...)       //           Nonce =>
	data = append(data, common.Uint64Bytes(t.SigSpecHt)...)   //           SigSpecHt =>
	data = append(data, common.Uint64Bytes(t.Priority)...)    //           Priority =>
	data = append(data, common.Uint64Bytes(t.PriorityIdx)...) //           PriorityIdx =>
	return data, nil                                          //           All good, return data and nil error
}

// UnMarshal
// Take a bunch of bytes in data a []byte and pull out all the values for
// the GenTransaction
func (t *SignatureInfo) UnMarshal(data []byte) (nextData []byte, err error) { // Get the data from the input stream
	defer func() { //                                                            and set the values.  Any problem
		if err := recover(); err != nil { //                                     will be reported
			err = fmt.Errorf("error unmarshaling GenTransaction %v", err) //
		} //
	}() //

	URL, data := common.BytesSlice(data)           //                      => URL
	t.URL = string(URL)                            //                       (url must be a string)
	t.Nonce, data = common.BytesUint64(data)       //                      => Nonce
	t.SigSpecHt, data = common.BytesUint64(data)   //                      => SigSpecHt
	t.Priority, data = common.BytesUint64(data)    //                      => Priority
	t.PriorityIdx, data = common.BytesUint64(data) //                      => PriorityIdx
	return data, nil                               //
} //
