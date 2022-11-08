// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

type DevnetOptions struct {
	BvnCount       int
	ValidatorCount int
	FollowerCount  int
	BasePort       int
	GenerateKeys   func() (privVal, node []byte)
	HostName       func(bvnNum, nodeNum int) (host, listen string)
}

func NewDevnet(opts DevnetOptions) *NetworkInit {
	netInit := new(NetworkInit)
	netInit.Id = "DevNet"
	count := opts.ValidatorCount + opts.FollowerCount
	for i := 0; i < opts.BvnCount; i++ {
		bvnInit := new(BvnInit)
		bvnInit.Id = fmt.Sprintf("BVN%d", i+1)
		netInit.Bvns = append(netInit.Bvns, bvnInit)
		for j := 0; j < count; j++ {
			nodeType := config.Validator
			if j >= opts.ValidatorCount {
				nodeType = config.Follower
			}

			nodeInit := new(NodeInit)
			nodeInit.DnnType = nodeType
			nodeInit.BvnnType = nodeType
			nodeInit.BasePort = uint64(opts.BasePort)
			bvnInit.Nodes = append(bvnInit.Nodes, nodeInit)

			if opts.GenerateKeys != nil {
				privVal, node := opts.GenerateKeys()
				nodeInit.PrivValKey = privVal
				nodeInit.NodeKey = node
			}

			if opts.HostName != nil {
				host, listen := opts.HostName(i, j)
				nodeInit.AdvertizeAddress = host
				nodeInit.ListenAddress = listen
			}
		}
	}

	return netInit
}
