package accumulated

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
			bvnInit.Nodes = append(bvnInit.Nodes, nodeInit)

			host, listen := opts.HostName(i, j)
			nodeInit.Advertise = protocol.NewInternetAddress("http", host, opts.BasePort)
			nodeInit.ListenIP = listen

			if opts.GenerateKeys != nil {
				privVal, node := opts.GenerateKeys()
				nodeInit.PrivValKey = privVal
				nodeInit.NodeKey = node
			}

		}
	}

	return netInit
}
