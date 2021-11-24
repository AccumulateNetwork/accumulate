package protocol

import "github.com/AccumulateNetwork/accumulate/internal/encoding"

var ErrNotEnoughData = encoding.ErrNotEnoughData
var ErrOverflow = encoding.ErrOverflow

type ErrorCode int

const (
	//CodeTxnQueryError is returned when txn is not found
	CodeTxnQueryError ErrorCode = 5
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
	//CodeMarshallingError is returned when marshaling  object or binary fails
	CodeMarshallingError ErrorCode = 17
	//CodeUnMarshallingError is returned when unmarshaling  object or binary fails
	CodeUnMarshallingError ErrorCode = 18
	//CodeInvalidQueryType is returned when query type in request is not matched with the available ones
	CodeInvalidQueryType ErrorCode = 19
	//CodeInvalidTxnType is returned when txn type passed is not available
	CodeInvalidTxnType ErrorCode = 20
	//CodeValidateTxnError is returned when execution validation of txn fails
	CodeValidateTxnError ErrorCode = 21
	//CodeInvalidTxnError is returned when txn doesn't contains proper data
	CodeInvalidTxnError ErrorCode = 22
	//CodeAddTxnError is returned when adding txn to state db fails
	CodeAddTxnError ErrorCode = 23
)

type Error struct {
	Code    ErrorCode
	Message error
}

var _ error = (*Error)(nil)

func (err *Error) Error() string {
	return err.Message.Error()
}

func (err *Error) Unwrap() error {
	return err.Message
}
