package protocol

import "github.com/AccumulateNetwork/accumulate/internal/encoding"

var ErrNotEnoughData = encoding.ErrNotEnoughData
var ErrOverflow = encoding.ErrOverflow

type ErrorCode int

const (
	// CodeOK is returned when the operation succeeded
	CodeOK ErrorCode = 0

	// CodeEncodingError is returned when un/marshalling fails
	CodeEncodingError ErrorCode = 1

	// CodeBadNonce is returned when the transaction's nonce is invalid
	CodeBadNonce ErrorCode = 2

	// CodeInvalidRequest is returned when the request parameters are invalid
	CodeInvalidRequest ErrorCode = 3

	// CodeInternalError is returned when an internal error occurs
	CodeInternalError ErrorCode = 4

	// CodeNotFound is returned when the record is not found
	CodeNotFound ErrorCode = 5

	// CodeInvalidRecord is returned when a database record is invalid
	CodeInvalidRecord ErrorCode = 17

	//CodeTxnRange is returned when txn range query fails
	CodeTxnRange ErrorCode = 6
	//CodeTxnHistory is returned when txn history query fails
	CodeTxnHistory ErrorCode = 7
	//CodeInvalidURL is returned when invalid URL is passed in query
	CodeInvalidURL ErrorCode = 8
	//CodeDirectoryURL is returned when invalid directory URL is passed in query
	CodeDirectoryURL ErrorCode = 9
	//CodeChainIdError is returned when query by in id fails
	CodeChainIdError ErrorCode = 10
	//CodeRoutingChainId is returned when setting routing chain id fails
	CodeRoutingChainId ErrorCode = 11
	//CodeCheckTxError is returned when txn validation check fails
	CodeCheckTxError ErrorCode = 12
	//CodeDeliverTxError is returned when txn deliver method fails
	CodeDeliverTxError ErrorCode = 13
	//CodeTxnStateError is returned when adding txn to state fails
	CodeTxnStateError ErrorCode = 14
	//CodeRecordTxnError is returned when storing pending state updates fail
	CodeRecordTxnError ErrorCode = 15
	//CodeSyntheticTxnError is returned when submit synthetic txn fails
	CodeSyntheticTxnError ErrorCode = 16

	CodeInvalidQueryType ErrorCode = 19
	//CodeInvalidTxnType is returned when txn type passed is not available
	CodeInvalidTxnType ErrorCode = 20
	//CodeValidateTxnError is returned when execution validation of txn fails
	CodeValidateTxnError ErrorCode = 21
	//CodeInvalidTx is returned when txn doesn't contains proper data
	CodeInvalidTx ErrorCode = 22
	//CodeAddTxnError is returned when adding txn to state db fails
	CodeAddTxnError ErrorCode = 23
)

type Error struct {
	Code    ErrorCode
	Message string
}

var _ error = (*Error)(nil)

func (err *Error) Error() string {
	return err.Message
}
