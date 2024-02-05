// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const defaultHost = "/ip4/0.0.0.0"

var (
	portDir = portOffset(config.PortOffsetDirectory)
	portBVN = portOffset(config.PortOffsetBlockValidator)

	portCmtP2P  = portOffset(config.PortOffsetTendermintP2P)
	portCmtRPC  = portOffset(config.PortOffsetTendermintRpc)
	portMetrics = portOffset(config.PortOffsetPrometheus)
	portAccAPI  = portOffset(config.PortOffsetAccumulateApi)
	portAccP2P  = portOffset(config.PortOffsetAccumulateP2P)
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

func registerRpcService(inst *Instance, addr *api.ServiceAddress, service message.Service) {
	handler, err := message.NewHandler(service)
	if err != nil {
		panic(err)
	}
	inst.p2p.RegisterService(addr, handler.Handle)
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

func listenUrl(addr multiaddr.Multiaddr, defaultHost string, transform ...addrTransform) string {
	addr = listen(addr, defaultHost, transform...)
	scheme, host, port, _, err := decomposeListen(addr)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port)
}

func listenHostPort(addr multiaddr.Multiaddr, defaultHost string, transform ...addrTransform) string {
	addr = listen(addr, defaultHost, transform...)
	_, host, port, _, err := decomposeListen(addr)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s:%s", host, port)
}

func decomposeListen(addr multiaddr.Multiaddr) (proto, host, port, http string, err error) {
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4,
			multiaddr.P_IP6,
			multiaddr.P_DNS,
			multiaddr.P_DNS4,
			multiaddr.P_DNS6:
			host = c.Value()
		case multiaddr.P_TCP,
			multiaddr.P_UDP:
			proto = c.Protocol().Name
			port = c.Value()
		case multiaddr.P_HTTP,
			multiaddr.P_HTTPS:
			http = c.Protocol().Name
		default:
			err = errors.UnknownError.WithFormat("invalid listen address: %v", addr)
			return false
		}
		return true
	})
	return
}

func isPrivate(addr multiaddr.Multiaddr) bool {
	var private bool
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4,
			multiaddr.P_IP6:
			ip := net.ParseIP(c.Value())
			private = ip != nil && (ip.IsLoopback() || ip.IsPrivate())
			return false
		}
		return true
	})
	return private
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
	for _, s := range cfg.Services {
		t, ok := s.(T)
		if ok && (predicate == nil || predicate(t)) {
			if existing != nil {
				*existing = t
			}
			return true
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
