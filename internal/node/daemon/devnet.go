// Copyright 2023 The Accumulate Authors
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
	BsnCount       int
	BasePort       int
	GenerateKeys   func() (privVal, dnn, bvnn, bsnn []byte)
	HostName       func(bvnNum, nodeNum int) (host, listen string)
}

func NewDevnet(opts DevnetOptions) *NetworkInit {
	netInit := new(NetworkInit)
	netInit.Id = "DevNet"

	bootstrap := new(NodeInit)
	netInit.Bootstrap = bootstrap
	bootstrap.BasePort = uint64(opts.BasePort)

	if opts.GenerateKeys != nil {
		key, _, _, _ := opts.GenerateKeys()
		bootstrap.PrivValKey = key
		bootstrap.DnNodeKey = key
		bootstrap.BvnNodeKey = key
		bootstrap.BsnNodeKey = key
	}

	if opts.HostName != nil {
		host, listen := opts.HostName(-1, -1)
		bootstrap.AdvertizeAddress = host
		bootstrap.ListenAddress = listen
	}

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
				privVal, dnn, bvnn, _ := opts.GenerateKeys()
				nodeInit.PrivValKey = privVal
				nodeInit.DnNodeKey = dnn
				nodeInit.BvnNodeKey = bvnn
			}

			if opts.HostName != nil {
				host, listen := opts.HostName(i, j)
				nodeInit.AdvertizeAddress = host
				nodeInit.ListenAddress = listen
			}
		}
	}

	if opts.BsnCount == 0 {
		return netInit
	}

	netInit.Bsn = new(BvnInit)
	netInit.Bsn.Id = "BSN"
	for i := 0; i < opts.BsnCount; i++ {
		nodeInit := new(NodeInit)
		nodeInit.BsnnType = config.Validator
		nodeInit.BasePort = uint64(opts.BasePort)
		netInit.Bsn.Nodes = append(netInit.Bsn.Nodes, nodeInit)

		if opts.GenerateKeys != nil {
			privVal, _, _, bsnn := opts.GenerateKeys()
			nodeInit.PrivValKey = privVal
			nodeInit.BsnNodeKey = bsnn
		}

		if opts.HostName != nil {
			host, listen := opts.HostName(-1, i)
			nodeInit.AdvertizeAddress = host
			nodeInit.ListenAddress = listen
		}
	}

	return netInit
}
