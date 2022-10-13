package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (r *RoutingTable) AddOverride(account *url.URL, partition string) {
	ptr, _ := sortutil.BinaryInsert(&r.Overrides, func(o RouteOverride) int {
		return o.Account.Compare(account)
	})
	ptr.Account = account
	ptr.Partition = partition
}
