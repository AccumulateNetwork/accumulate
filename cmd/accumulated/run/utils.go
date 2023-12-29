// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	portDir = portOffset(config.PortOffsetDirectory)
	portBVN = portOffset(config.PortOffsetBlockValidator)

	portAccAPI = portOffset(config.PortOffsetAccumulateApi)
	portAccP2P = portOffset(config.PortOffsetAccumulateP2P)
)

func ptr[T any](v T) *T { return &v }

func setDefaultPtr[V any](ptr **V, def V) {
	if *ptr == nil {
		*ptr = &def
	}
}

func setDefaultVal[V any](ptr *V, def V) {
	if reflect.ValueOf(ptr).Elem().IsZero() {
		*ptr = def
	}
}

func mustParsePeer(s string) peer.ID {
	id, err := peer.Decode(s)
	if err != nil {
		panic(err)
	}
	return id
}

func mustParseMulti(s string) multiaddr.Multiaddr {
	addr, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		panic(err)
	}
	return addr
}

func getPrivateKey(key PrivateKey, inst *Instance) (ed25519.PrivateKey, error) {
	addr, err := key.get(inst)
	if err != nil {
		return nil, err
	}
	if addr.GetType() != protocol.SignatureTypeED25519 {
		return nil, errors.BadRequest.WithFormat("key type %v not supported", addr.GetType())
	}
	sk, ok := addr.GetPrivateKey()
	if !ok {
		return nil, errors.BadRequest.WithFormat("missing private key")
	}
	return sk, nil
}

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

func listen(addr multiaddr.Multiaddr, defaultHost string, transform ...addrTransform) multiaddr.Multiaddr {
	if defaultHost != "" {
		addr = ensureHost(addr, defaultHost)
	}
	return applyAddrTransforms(addr, transform...)
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

type useHTTP struct{}

func (useHTTP) Apply(c multiaddr.Component) ([]multiaddr.Component, bool) {
	switch c.Protocol().Code {
	case multiaddr.P_TCP,
		multiaddr.P_UDP,
		multiaddr.P_HTTP,
		multiaddr.P_HTTPS:
		// Ok
	default:
		return nil, false
	}

	d, err := multiaddr.NewComponent("http", c.Value())
	if err != nil {
		panic(err)
	}
	return []multiaddr.Component{*d}, true
}

func haveService[T any](cfg *Config, predicate func(T) bool, existing *T) bool {
	for _, s := range [][]Service{cfg.Apps, cfg.Services} {
		for _, s := range s {
			t, ok := s.(T)
			if ok && (predicate == nil || predicate(t)) {
				if existing != nil {
					*existing = t
				}
				return true
			}
		}
	}
	return false
}

func haveService2[T any](cfg *Config, wantID string, getID func(T) string, existing *T) bool {
	return haveService(cfg, func(s T) bool {
		return strings.EqualFold(wantID, getID(s))
	}, existing)
}

func addService[T Service](cfg *Config, s T, getID func(T) string) T {
	if !haveService2(cfg, getID(s), getID, &s) {
		cfg.Services = append(cfg.Services, s)
	}
	return s
}
