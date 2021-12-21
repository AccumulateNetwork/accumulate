package transactions

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
)

// GenTransaction
// Every transaction that goes through the Accumulate protocol is packaged
// as a GenTransaction.  This means we implement this once, and most of the
// transaction validation and processing is done in one and only one way.
//
// Note we Hash the SigInfo (that makes every transaction unique) and
// we hash the Transaction (which implements the action or data) then
// we hash them together to create the Transaction Hash.
//
// Since all transactions need the SigInfo, no point in implementing it
// over and over.  But a range of transactions are needed, this the
// Transaction.
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
	t.TransactionHash() //                                              make sure both have TransactionHashes computed
	t2.TransactionHash()
	for i, sig := range t.Signature { //                                for every signature
		if !sig.Equal(t2.Signature[i]) { //                             check they are the same
			return false //                                             return false if they are not.
		}
	}
	return t.SigInfo.Equal(t2.SigInfo) && //                            The SigInfo has to be the same
		bytes.Equal(t.TransactionHash(), t2.TransactionHash()) && //    Check the Transaction hash
		bytes.Equal(t.Transaction, t2.Transaction) //                   and the transaction. Mismatch is false
}

// TransactionHash
// compute the transaction hash from the elements of the GenTransaction.
// This is used to populate the TxHash field.
func (t *GenTransaction) TransactionHash() []byte {
	if t.TxHash != nil { //                                Check if I have the hash already
		return t.TxHash //                                  Return it if I do
	} //
	data, err := t.SigInfo.Marshal() //                    Get the SigInfo (all the indexes to signatures)
	if err != nil {                  //                    On an error, return a nil
		return nil //
	} //
	sHash := sha256.Sum256(data)                        // Compute the SigHash
	tHash := sha256.Sum256(t.Transaction)               // Compute the transaction Hash
	txh := sha256.Sum256(append(sHash[:], tHash[:]...)) // Take hash of SigHash on left and hash of sub tx on right
	t.TxHash = txh[:]                                   // cache it
	return txh[:]                                       // And return it
}

// UnMarshal
// Create the binary representation of the GenTransaction
func (t *GenTransaction) Marshal() (data []byte, err error) {
	if err := t.SetRoutingChainID(); err != nil { //                        Make sure routing and chainID are set
		return nil, err //                                                   Not clear if this is necessary
	} //
	sLen := uint64(len(t.Signature)) //                                     Marshal the signatures, where
	if sLen < 1 || sLen > 100 {      //                                     we must have at least one of them.
		return nil, fmt.Errorf("must have 1 to 100 signatures") //          Otherwise we don't have a nonce to
	} //                                                                    make the translation unique
	data = common.Uint64Bytes(sLen) //                                      marshal the length, then each
	for _, v := range t.Signature { //                                      signature struct.
		if sig, err := v.Marshal(); err == nil { //
			data = append(data, sig...) //
		} else { //
			return data, err //
		} //
	} //
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
	var sLen uint64                       //                       Get how many signatures we have
	sLen, data = common.BytesUint64(data) //                       Of course, need it in an int of some sort
	if sLen < 1 || sLen > 100 {           //                       If the count isn't reasonable, die
		return nil, fmt.Errorf("signature length out of range") //
	} //
	for i := uint64(0); i < sLen; i++ { //                  Okay, now cycle for every signature
		sig := new(ED25519Sig)                           // And unmarshal a signature
		if data, err = sig.Unmarshal(data); err != nil { // If bad data is encountered,
			return nil, err //                              complain
		} //
		t.Signature = append(t.Signature, sig) //           Add each signature to list, and repeat until all done
	} //
	t.SigInfo = new(SignatureInfo)        //                Get a SignatureInfo struct
	data, err = t.SigInfo.UnMarshal(data) //                And unmarshal it.
	if err != nil {                       //                Get an error? Complain to caller!
		return nil, err //
	} //
	t.Transaction, data = common.BytesSlice(data) //        Get the Transaction out of the data
	err = t.SetRoutingChainID()                   //        Now compute the Routing and ChainID
	if err != nil {                               //        If an error, then complain
		return nil, err //                                  return no data, and an error
	} //
	return data, nil //                                     Return the data and the fact all is well.
} //

// SetRoutingChainID
// Take the URL in the GenTransaction and compute the Routing number and
// the ChainID
func (t *GenTransaction) SetRoutingChainID() error { //
	if t.Routing > 0 && t.ChainID != nil { //                               Check if there is anything to do
		return nil //                                                       If routing and chainID are set, good
	}
	if t.SigInfo == nil {
		return errors.New("missing signature info")
	}
	u, err := url.Parse(t.SigInfo.URL)
	if err != nil {
		return fmt.Errorf("invalid origin record: %w", err)
	}
	// TODO this should use u.Routing()
	h := u.IdentityChain()                                               // Hash the identity
	t.Routing = uint64(h[0])<<56 | uint64(h[1])<<48 | uint64(h[2])<<40 | // Get the routing out of it
		uint64(h[3])<<32 | uint64(h[4])<<24 | uint64(h[5])<<16 | //         Note Big Endian, 8 bytes
		uint64(h[6])<<8 | uint64(h[7]) //
	t.ChainID = u.ResourceChain() //                                        Compute and assign the ChainID
	return nil                    //                                        Return nil (all is well!)
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

// TransactionType
// Return the type of the Transaction (the first VarInt)
func (t *GenTransaction) TransactionType() types.TxType {
	transType, _ := common.BytesUint64(t.Transaction)
	return types.TxType(transType)
}

// As unmarshals the transaction payload as the given sub transaction type.
func (t *GenTransaction) As(subTx encoding.BinaryUnmarshaler) error {
	return subTx.UnmarshalBinary(t.Transaction)
}

func New(url string, height uint64, signer func(hash []byte) (*ED25519Sig, error), subTx encoding.BinaryMarshaler) (*GenTransaction, error) {
	return NewWith(&SignatureInfo{
		URL:           url,
		KeyPageHeight: height,
	}, signer, subTx)
}

func NewWith(info *SignatureInfo, signer func(hash []byte) (*ED25519Sig, error), subTx encoding.BinaryMarshaler) (*GenTransaction, error) {
	payload, err := subTx.MarshalBinary()
	if err != nil {
		return nil, err
	}

	tx := new(GenTransaction)
	tx.SigInfo = info
	tx.Signature = make([]*ED25519Sig, 1)
	tx.Transaction = payload

	err = tx.SetRoutingChainID()
	if err != nil {
		return nil, err
	}

	hash := tx.TransactionHash()
	tx.Signature[0], err = signer(hash)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
