package encoding

import "errors"

var ErrNotEnoughData = errors.New("not enough data")
var ErrOverflow = errors.New("overflow")

type ErrorCode int

const (
	CodeOk                          ErrorCode = 0
	CodeImproperTxnQueryRequest     ErrorCode = 6
	CodeTxnQueryError               ErrorCode = 7
	CodeImproperTxnHistoryRequest   ErrorCode = 8
	CodeTxnRange                    ErrorCode = 9
	CodeTxnHistory                  ErrorCode = 10
	CodeImproperTypeURLRequest      ErrorCode = 11
	CodeInvalidURL                  ErrorCode = 12
	CodeTypeURL                     ErrorCode = 13
	CodeImproperDirectoryURLRequest ErrorCode = 14
	CodeDirectoryURL                ErrorCode = 15
	CodeImproperChainIdRequest      ErrorCode = 16
	CodeChainIdError                ErrorCode = 17
	CodeDefaultError                ErrorCode = 18
	CodeRoutingChainId              ErrorCode = 19
	CodeCheckTxError                ErrorCode = 20
	CodeUnsupportedTxType           ErrorCode = 21
	CodeImproperTxnRequest          ErrorCode = 22
	CodeTxnCheckFailed              ErrorCode = 23
	CodeDeliverTxError              ErrorCode = 24
	CodeImproperAcceptedTxn         ErrorCode = 25
	CodeImproperPendingTxn          ErrorCode = 26
	CodeTxnStateError               ErrorCode = 27
	CodeRecordTxnError              ErrorCode = 28
	CodeSyntheticTxnError           ErrorCode = 29
	CodeMarshallingError            ErrorCode = 30
)

type Error struct {
	Code    ErrorCode
	Message string
}

var _ error = (*Error)(nil)

func (err *Error) Error() string {
	return err.Message
}
