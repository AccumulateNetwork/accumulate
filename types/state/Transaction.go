package state

import (
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/SMT/common"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"

	"github.com/AccumulateNetwork/accumulated/types"
)

// transactionHeader is the structure that stores the basic information needed
type txPendingState struct {
	Signature        []*transactions.ED25519Sig
	TransactionState *txState
	Status           string `json:"status" form:"status" query:"status" validate:"required"`
}

//transaction object will either be on main chain or combined with the header and placed on pending chain.  If this is
// part of the transactionPending, the Transaction can be nil which means the transaction contents are on the main chain
type txState struct {
	SigInfo         *transactions.SignatureInfo
	Transaction     *types.Bytes `json:"tx,omitempty" form:"tx" query:"tx" validate:"optional"`
	transactionHash *types.Bytes32
}

// NewPendingTransaction will create a new pending transaction from a general transaction
func NewPendingTransaction(gtx *transactions.GenTransaction) *PendingTransaction {
	ret := &PendingTransaction{}
	ret.Chain.SetHeader(types.String(gtx.SigInfo.URL), types.ChainTypeTransaction)
	ret.Signature = gtx.Signature
	ret.TransactionState = &txState{}
	ret.TransactionState.SigInfo = gtx.SigInfo
	ret.TransactionState.Transaction = &types.Bytes{}
	*ret.TransactionState.Transaction = gtx.Transaction
	txId := new(types.Bytes32)
	copy(txId[:], gtx.TransactionHash())
	ret.TransactionState.transactionHash = txId
	return ret
}

// NewTransaction will take the transaction from the Pending Transaction, remove it and return a new pending
// transaction state and a new transaction state.  This is used for promtion of a transaction to the main chain
func NewTransaction(pending *PendingTransaction) (*Transaction, *PendingTransaction) {
	txState := &Transaction{}
	txState.Transaction = pending.TransactionState.Transaction
	txState.SigInfo = pending.TransactionState.SigInfo
	txState.transactionHash = pending.TransactionState.transactionHash
	pending.TransactionState.Transaction = nil
	return txState, pending
}

// Transaction can take several modes, the basic is the signature information,
// i.e. transaction header (signature, rcd, transactionid, chainid)
// the body of the transaction can also be stored for pending transactions.
type Transaction struct {
	Entry
	Chain
	txState
}

type PendingTransaction struct {
	Entry
	Chain
	txPendingState
}

func (is *Transaction) TransactionHash() *types.Bytes32 {
	data, _ := is.MarshalBinary()
	txHash := types.Bytes32(sha256.Sum256(data))
	return &txHash
}

func (is *Transaction) GetChainUrl() string {
	return is.Chain.GetChainUrl()
}

func (is *Transaction) GetType() uint64 {
	return is.Chain.GetType()
}

func (is *Transaction) MarshalBinary() (data []byte, err error) {
	defer func() {
		if err := recover(); err != nil {
			err = fmt.Errorf("error marshaling transaction state %v", err)
		}
	}()
	//
	//headerData, err := is.Chain.MarshalBinary()
	//if err != nil {
	//	return nil, fmt.Errorf("unable to marshal chain header for transaction, %v", err)
	//}
	//
	//data = append(data, headerData...)

	if is.SigInfo == nil {
		panic("no SigInfo for state, shouldn't get here")
	}
	var si []byte                  // Someplace to marshal the SigInfo
	si, err = is.SigInfo.Marshal() // Marshal SigInfo
	if err != nil {                // If we have an error, report it.
		return nil, err //
	}
	data = append(data, si...)                                        // Add the SigInfo
	data = append(data, common.SliceBytes(is.Transaction.Bytes())...) // Add the transaction

	return data, nil
}

func (is *Transaction) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if recover(); err != nil {
			err = fmt.Errorf("error unmarshaling transaction state %v", err)
		}
	}()
	i := 0
	//
	//err := is.Chain.UnmarshalBinary(data)
	//if err != nil {
	//	return fmt.Errorf("unable to unmarshal data for transaction, %v", err)
	//}
	//
	//i += is.Chain.GetHeaderSize()

	if len(data) < i {
		return fmt.Errorf("unable to unmarshal raw transaction data")
	}

	is.SigInfo = new(transactions.SignatureInfo) //                Get a SignatureInfo struct
	data, err = is.SigInfo.UnMarshal(data[i:])   //                And unmarshal it.
	if err != nil {                              //                Get an error? Complain to caller!
		return err //
	} //

	if len(data) != 0 {
		is.Transaction = &types.Bytes{}
		*is.Transaction, data = common.BytesSlice(data) //        Get the Transaction out of the data
	}

	return nil
}

func (t *PendingTransaction) MarshalBinary() (data []byte, err error) {
	defer func() {
		if err := recover(); err != nil {
			err = fmt.Errorf("error marshaling Pending Transaction State %v", err)
		}
	}()

	headerData, err := t.Chain.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chain header for transaction, %v", err)
	}

	data = append(data, headerData...)

	sLen := uint64(len(t.Signature))
	if sLen < 1 || sLen > 100 {
		panic("must have 1 to 100 signatures")
	}
	data = append(data, common.Uint64Bytes(sLen)...)
	for _, v := range t.Signature {
		if sig, err := v.Marshal(); err == nil {
			data = append(data, sig...)
		} else {
			return data, err
		}
	}

	if t.TransactionState.SigInfo == nil {
		panic("no SigInfo for state, shouldn't get here")
	}
	var si []byte                                  // Someplace to marshal the SigInfo
	si, err = t.TransactionState.SigInfo.Marshal() // Marshal SigInfo
	if err != nil {                                // If we have an error, report it.
		return nil, err //
	}
	data = append(data, si...) // Add the SigInfo
	if t.TransactionState.Transaction != nil {
		data = append(data, common.SliceBytes(t.TransactionState.Transaction.Bytes())...) // Add the transaction
	}

	return data, nil
}

func (t *PendingTransaction) UnmarshalBinary(data []byte) (err error) {
	defer func() { //
		if recover() != nil { //
			err = fmt.Errorf("error unmarshaling GenTransaction %v", err) //
		} //
	}() //

	err = t.Chain.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("cannot unmarshal chain header for transaction state,%v", err)
	}

	data = data[t.Chain.GetHeaderSize():]
	var sLen uint64                       //                Get how many signatures we have
	sLen, data = common.BytesUint64(data) //                Of course, need it in an int of some sort
	if sLen < 1 || sLen > 100 {           //                If the count isn't reasonable, die
		panic("signature length out of range") //           With a panic
	} //
	for i := uint64(0); i < sLen; i++ { //                  Okay, now cycle for every signature
		sig := new(transactions.ED25519Sig)              // And unmarshal a signature
		if data, err = sig.Unmarshal(data); err != nil { // If bad data is encountered,
			return err //                              complain
		} //
		t.Signature = append(t.Signature, sig) //           Add each signature to list, and repeat until all done
	} //
	t.TransactionState = &txState{}
	t.TransactionState.SigInfo = new(transactions.SignatureInfo) //                Get a SignatureInfo struct
	data, err = t.TransactionState.SigInfo.UnMarshal(data)       //                And unmarshal it.
	if err != nil {                                              //                Get an error? Complain to caller!
		return err //
	} //

	if len(data) != 0 {
		t.TransactionState.Transaction = &types.Bytes{}
		*t.TransactionState.Transaction, data = common.BytesSlice(data) //        Get the Transaction out of the data
	}

	return nil
}

func (t *PendingTransaction) GetChainUrl() string {
	return t.Chain.GetChainUrl()
}

func (t *PendingTransaction) GetType() uint64 {
	return t.Chain.GetType()
}
