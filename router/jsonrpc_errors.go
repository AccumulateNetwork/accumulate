package router

import "github.com/AccumulateNetwork/jsonrpc2/v15"

var (
	ErrorSponsorInvalid   = jsonrpc2.NewError(-32801, "Invalid Sponsor Identity", "sponsor identity url is invalid")
	ErrorInvalidSignature = jsonrpc2.NewError(-32802, "Invalid Signature", "invalid signature")
	ErrorInvalidTimestamp = jsonrpc2.NewError(-32803, "Invalid Timestamp", "invalid timestamp")

	ErrorIdentityInvalid  = jsonrpc2.NewError(-32901, "Invalid Identity", "identity url is invalid")
	ErrorIdentityNotExist = jsonrpc2.NewError(-32902, "Identity Does Not Exist", "identity url does not exist")

	ErrorTokenInvalid  = jsonrpc2.NewError(-33001, "Invalid Token", "token url is invalid")
	ErrorTokenNotExist = jsonrpc2.NewError(-33002, "Token Does Not Exist", "token url does not exist")

	ErrorTokenAddressInvalid  = jsonrpc2.NewError(-34001, "Invalid Token Address", "token address url is invalid")
	ErrorTokenAddressNotExist = jsonrpc2.NewError(-34002, "Token Address Does Not Exist", "token address url does not exist")
)
