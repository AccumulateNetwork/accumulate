// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"fmt"
	"strconv"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

var (
	portDir = portOffset(config.PortOffsetDirectory)
	portBVN = portOffset(config.PortOffsetBlockValidator)

	portAccAPI = portOffset(config.PortOffsetAccumulateApi)
	portAccP2P = portOffset(config.PortOffsetAccumulateP2P)
)

func ptr[T any](v T) *T { return &v }

func addrHasOneOf(addr multiaddr.Multiaddr, components ...string) bool {
	var found bool
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		for _, component := range components {
			if c.Protocol().Name == component {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func ensureHost(addr multiaddr.Multiaddr, defaultHost string) multiaddr.Multiaddr {
	if addrHasOneOf(addr, "ip4", "ip6", "dns", "dns4", "dns6") {
		return addr
	}
	return multiaddr.StringCast(defaultHost).Encapsulate(addr)
}

type addrTransform interface {
	Apply(multiaddr.Component) ([]multiaddr.Component, bool)
}

func applyAddrTransforms(addr multiaddr.Multiaddr, transforms ...addrTransform) multiaddr.Multiaddr {
	for _, tr := range transforms {
		var result []multiaddr.Multiaddr
		multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
			d, ok := tr.Apply(c)
			if !ok {
				result = append(result, &c)
				return true
			}
			for _, c := range d {
				c := c
				result = append(result, &c)
			}
			return true
		})
		addr = multiaddr.Join(result...)
	}
	return addr
}

type portOffset uint64

func (p portOffset) Apply(c multiaddr.Component) ([]multiaddr.Component, bool) {
	switch c.Protocol().Code {
	case multiaddr.P_TCP,
		multiaddr.P_UDP:
		// Ok
	default:
		return nil, false
	}

	port, err := strconv.ParseUint(c.Value(), 10, 64)
	if err != nil {
		panic(err)
	}
	d, err := multiaddr.NewComponent(c.Protocol().Name, fmt.Sprint(port+uint64(p)))
	if err != nil {
		panic(err)
	}
	return []multiaddr.Component{*d}, true
}

type useTCP struct{}

func (useTCP) Apply(c multiaddr.Component) ([]multiaddr.Component, bool) {
	switch c.Protocol().Code {
	case multiaddr.P_TCP,
		multiaddr.P_UDP:
		// Ok
	default:
		return nil, false
	}

	d, err := multiaddr.NewComponent("tcp", c.Value())
	if err != nil {
		panic(err)
	}
	return []multiaddr.Component{*d}, true
}

type useQUIC struct{}

func (useQUIC) Apply(c multiaddr.Component) ([]multiaddr.Component, bool) {
	switch c.Protocol().Code {
	case multiaddr.P_TCP,
		multiaddr.P_UDP:
		// Ok
	default:
		return nil, false
	}

	d1, err := multiaddr.NewComponent("udp", c.Value())
	if err != nil {
		panic(err)
	}
	d2, err := multiaddr.NewComponent("quic", "")
	if err != nil {
		panic(err)
	}
	return []multiaddr.Component{*d1, *d2}, true
}
