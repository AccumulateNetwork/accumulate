// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package transport

import (
	"context"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/interfaces"
)

// Transport defines the interface for message transport
type Transport interface {
	RoundTrip(ctx context.Context, requests []interfaces.Message, callback ResponseCallback) error
}

// ResponseCallback is called for each response received
type ResponseCallback func(response interfaces.Message) error
