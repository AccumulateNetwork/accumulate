package router

import "github.com/AccumulateNetwork/jsonrpc2/v15"

var (
	// 32800-32899 — API request struct validation
	ErrorInvalidRequest = jsonrpc2.NewError(-32801, "Parsing Error", "unable to parse request params")
	// 32802 — Validation Error

	// 32900-32999 — Signer, signature, timestamp validation
	ErrorSignerInvalid    = jsonrpc2.NewError(-32901, "Invalid Signer", "signer adi url is invalid")
	ErrorSignerNotExist   = jsonrpc2.NewError(-32902, "Signer Does Not Exist", "signer adi url does not exist")
	ErrorInvalidTimestamp = jsonrpc2.NewError(-32903, "Invalid Timestamp", "invalid timestamp")
	ErrorInvalidSignature = jsonrpc2.NewError(-32904, "Invalid Signature", "invalid signature")

	// 33000-33999 — Data structures validation
	ErrorADIInvalid           = jsonrpc2.NewError(-33001, "Invalid ADI", "adi url is invalid")
	ErrorADINotExist          = jsonrpc2.NewError(-33002, "ADI Does Not Exist", "adi url does not exist")
	ErrorTokenInvalid         = jsonrpc2.NewError(-33003, "Invalid Token", "token url is invalid")
	ErrorTokenNotExist        = jsonrpc2.NewError(-33004, "Token Does Not Exist", "token url does not exist")
	ErrorTokenAccountInvalid  = jsonrpc2.NewError(-33005, "Invalid Token Account", "token account url is invalid")
	ErrorTokenAccountNotExist = jsonrpc2.NewError(-33006, "Token Account Does Not Exist", "token account url does not exist")
	ErrorTokenTxInvalid       = jsonrpc2.NewError(-33007, "Invalid Token Transaction", "token tx hash is invalid")
	ErrorTokenTxNotExist      = jsonrpc2.NewError(-33008, "Token Transaction Does Not Exist", "token tx hash does not exist")
)

func NewValidatorError(err error) jsonrpc2.Error {
	return jsonrpc2.NewError(-32802, "Validation Error", err)
}
