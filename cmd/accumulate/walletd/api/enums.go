// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import "github.com/AccumulateNetwork/jsonrpc2/v15"

type ErrorCode jsonrpc2.ErrorCode

func (e ErrorCode) Code() jsonrpc2.ErrorCode {
	return jsonrpc2.ErrorCode(e)
}
