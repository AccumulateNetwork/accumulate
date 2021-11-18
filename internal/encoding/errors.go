package encoding

import "errors"

const (
	//Executor codes

	CodeOk                          = 0
	CodeImproperTxnQueryRequest     = 6
	CodeTxnQueryError               = 7
	CodeImproperTxnHistoryRequest   = 8
	CodeTxnRange                    = 9
	CodeTxnHistory                  = 10
	CodeImproperTypeURLRequest      = 11
	CodeInvalidURL                  = 12
	CodeTypeURL                     = 13
	CodeImproperDirectoryURLRequest = 14
	CodeDirectoryURL                = 15
	CodeImproperChainIdRequest      = 16
	CodeChainIdError                = 17
	CodeDefaultError                = 18
	CodeRoutingChainId              = 19
	CodeCheckTxError                = 20
	CodeUnsupportedTxType           = 21
	CodeImproperTxnRequest          = 22
	CodeTxnCheckFailed              = 23
	CodeDeliverTxError              = 24
	CodeImproperAcceptedTxn         = 25
	CodeImproperPendingTxn          = 26
	CodeTxnStateError               = 27
	CodeRecordTxnError              = 28
	CodeSyntheticTxnError           = 29
	CodeMarshallingError            = 30
)

var ErrNotEnoughData = errors.New("not enough data")
var ErrOverflow = errors.New("overflow")
var ErrImproperTxnQueryRequest = "improper txn query request"
var ErrImproperTxnHistoryRequest = "improper txn history request"
var ErrTxnRange = "error obtaining txid range %v"
var ErrTxnHistoryPayload = "error marshalling payload for transaction history"
var ErrImproperTypeURL = "improper type url request"
var ErrInvalidURL = "invalid URL in query %s"
var ErrTypeURL = "type URL error %s"
var ErrImproperDirectoryURL = "improper directory url request"
var ErrDirectoryURL = "type directory error %s"
var ErrImproperChainId = "improper chain id request"
var ErrCheckTx = "Check Tx Error : %s"
var ErrImproperTxnRequest = "improper txn request"
var ErrTxnCheckFailed = "Txn Check Failed : %s"
var ErrDeliverTx = "Deliver Tx Error : %s"
var ErrImproperAcceptedTxn = "improper accepted txn : %s"
var ErrImproperPendingTxn = "improper pending txn : %s"
var ErrTxnState = "store txn state error : %s"
var ErrRecordTxn = "store pending state error : %s"
var ErrSyntheticTxn = "synthetic txn error : %s"
var ErrMarshallingObject = "marshalling object error"
