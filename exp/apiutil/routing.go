// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package apiutil

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func RouteAccount(table *protocol.RoutingTable, account *url.URL) (string, error) {
	return routing.NewRouter(routing.RouterOptions{
		Initial: table,
	}).RouteAccount(account)
}
