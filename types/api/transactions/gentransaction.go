package transactions

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"

	"github.com/AccumulateNetwork/SMT/common"
)

// GenTransaction
// Every transaction that goes through the Accumulate protocol is packaged
// as a GenTransaction.  This means we implement this once, and most of the
// transaction validation and processing is done in one and only one way.
type GenTransaction struct {
	Routing uint64 //            first 8 bytes of hash of identity [NOT marshaled]
	ChainID []byte //            hash of chain URL [NOT marshaled]

	Signature   []*ED25519Sig  // Signature(s) of the transaction
	TxHash      []byte         // Hash of the Transaction
	SigInfo     *SignatureInfo // Information that is included with the Transaction
	Transaction []byte         // The transaction that follows
}

// Equal
// Largely used in testing, but allows the testing that one GenTransaction
// is equal to another.  In testing we marshal and unmarshal a GenTransaction
// and test that the information in the GenTransaction is preserved
func (t *GenTransaction) Equal(t2 *GenTransaction) bool {
	isEqual := true
	for i, sig := range t.Signature {
		isEqual = isEqual && sig.Equal(t2.Signature[i])
	}
	return isEqual &&
		bytes.Equal(t.TxHash, t2.TxHash) &&
		bytes.Equal(t.Transaction, t2.Transaction)
}

// TransactionHash
// compute the transaction hash from the elements of the GenTransaction.
// This is used to populate the TxHash field.
func (t *GenTransaction) TransactionHash() []byte {
	if t.TxHash != nil { //                  Check if I have the hash already
		return t.TxHash //                   Return it if I do
	} //
	data, err := t.SigInfo.Marshal() //      Get the SigInfo (all the indexes to signatures)
	if err != nil {                  //      On an error, return a nil
		return nil //
	} //
	data = append(data, t.Transaction...) // Add the transaction to the SigInfo
	txh := sha256.Sum256(data)            // Take the hash
	t.TxHash = txh[:]                     // cache it
	return txh[:]                         // And return it
}

// UnMarshal
// Create the binary representation of the GenTransaction
func (t *GenTransaction) Marshal() (data []byte, err error) {
	defer func() {
		if err := recover(); err != nil {
			err = fmt.Errorf("error marshaling GenTransaction %v", err)
		}
	}()
	sLen := uint64(len(t.Signature))
	if sLen == 0 || sLen > 100 {
		panic("must have 1 to 100 signatures")
	}
	data = common.Uint64Bytes(sLen)
	for _, v := range t.Signature {
		if sig, err := v.Marshal(); err == nil {
			data = append(data, sig...)
		} else {
			return data, err
		}
	}
	var si []byte                 // Someplace to marshal the SigInfo
	si, err = t.SigInfo.Marshal() // Marshal SigInfo
	if err != nil {               // If we have an error, report it.
		return nil, err //
	}
	data = append(data, si...)                               // Add the SigInfo
	data = append(data, common.SliceBytes(t.Transaction)...) // Add the transaction
	return data, nil
}

// UnMarshal
// Take a bunch of bytes in data a []byte and pull out all the values for
// the GenTransaction
func (t *GenTransaction) UnMarshal(data []byte) (nextData []byte, err error) {
	defer func() { //
		if err := recover(); err != nil { //
			err = fmt.Errorf("error unmarshaling GenTransaction %v", err) //
		} //
	}() //
	var sLen uint64                       //                Get how many signatures we have
	sLen, data = common.BytesUint64(data) //                Of course, need it in an int of some sort
	if sLen < 1 || sLen > 100 {           //                If the count isn't reasonable, die
		panic("signature length out of range") //           With a panic
	} //
	for i := uint64(0); i < sLen; i++ { //                  Okay, now cycle for every signature
		sig := new(ED25519Sig)                           // And unmarshal a signature
		if data, err = sig.Unmarshal(data); err != nil { // If bad data is encountered,
			return nil, err //                              complain
		} //
		t.Signature = append(t.Signature, sig) //           Add each signature to list, and repeat until all done
	} //
	si := new(SignatureInfo)       //                       Get a SignatureInfo struct
	data, err = si.UnMarshal(data) //                       And unmarshal it.
	if err != nil {                //                       Get an error? Complain to caller!
		return nil, err //
	} //
	t.Transaction, data = common.BytesSlice(data) // Get the Transaction out of the data
	err = t.SetRoutingChainID()                   // Now compute the Routing and ChainID
	if err != nil {                               // If an error, then complain
		return nil, err //                           return no data, and an error
	} //
	return data, nil //                              Return the data and the fact all is well.
} //

// SetRoutingChainID
// Take the URL in the GenTransaction and compute the Routing number and
// the ChainID
func (t *GenTransaction) SetRoutingChainID() error { //
	if t.Routing > 0 && t.ChainID != nil { //                               Check if there is anything to do
		return nil //                                                       If routing and chainID are set, good
	}
	adi, chainPath, err := types.ParseIdentityChainPath(&t.SigInfo.URL) //  Parse the URL.  Hope it is good.
	if err != nil {                                                     //  If hope is squashed, return err
		return err //
	} //
	h := sha256.Sum256([]byte(adi))                                      // Hash the identity
	t.Routing = uint64(h[0])<<56 | uint64(h[1])<<48 | uint64(h[2])<<40 | // Get the routing out of it
		uint64(h[3])<<32 | uint64(h[4])<<24 | uint64(h[5])<<16 | //           Note Big Endian, 8 bytes
		uint64(h[6])<<8 | uint64(h[7])
	h = sha256.Sum256([]byte(chainPath)) //                                 Compute the chainID from the URL
	t.ChainID = h[:]                     //                                 Assign the ChainID
	return nil                           //                                 Return nil (all is well!)
} //

// ValidateSig
// We validate the signature of the transaction.
func (t *GenTransaction) ValidateSig() bool { // Validate the signatures on the GenTransaction
	th := t.TransactionHash()       //           Get the transaction hash (computes it if it must)
	for _, v := range t.Signature { //           All signatures must verify the TxHash
		if !v.Verify(th) { //                    Verify each signature
			return false //                      One bad signature fails the lot
		}
	}
	return true //                               Return all is well.
}
