// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// nullDispatcher is a [Dispatcher] that discards everything.
type nullDispatcher struct{}

func (nullDispatcher) Submit(context.Context, *url.URL, *messaging.Envelope) error { return nil }

func (nullDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
