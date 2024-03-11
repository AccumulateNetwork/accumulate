// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"fmt"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

func (m *JrpcMethods) GetMethod(name string) jsonrpc2.MethodFunc {
	method := m.methods[name]
	if method == nil {
		panic(fmt.Errorf("method %q not found", name))
	}
	return method
}
