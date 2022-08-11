package api

import "github.com/AccumulateNetwork/jsonrpc2/v15"

type ErrorCode jsonrpc2.ErrorCode

func (e ErrorCode) Code() jsonrpc2.ErrorCode {
	return jsonrpc2.ErrorCode(e)
}
