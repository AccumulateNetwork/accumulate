// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

func NewDispatcher(network string, router routing.Router, dialer message.Dialer) *dispatcher {
	return newDispatcher(network, router, dialer)
}
