// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (r *RoutingTable) AddOverride(account *url.URL, partition string) {
	ptr, _ := sortutil.BinaryInsert(&r.Overrides, func(o RouteOverride) int {
		return o.Account.Compare(account)
	})
	ptr.Account = account
	ptr.Partition = partition
}
