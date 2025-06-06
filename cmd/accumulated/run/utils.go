// Copyright 2025 The Accumulate Authors
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

	"github.com/libp2p/go-libp2p/core/peer"
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

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func Ptr[T any](v T) *T { return &v }

func setDefaultPtr[V any](ptr **V, def V) V {
	if *ptr == nil {
		*ptr = &def
	}
	return **ptr
}

func setDefaultVal[V any](ptr *V, def V) V {
	if reflect.ValueOf(ptr).Elem().IsZero() {
		*ptr = def
	}
	return *ptr
}

func setDefaultSlice[V any, S ~[]V](ptr *S, def ...V) S {
	return setDefaultVal(ptr, def)
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
	if addr == nil {
		return false
	}

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
	host := multiaddr.StringCast(defaultHost)
	if addr == nil {
		return host
	}
	return host.Encapsulate(addr)
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

func httpListen(ma multiaddr.Multiaddr) (net.Listener, bool, error) {
	proto, addr, port, http, err := decomposeListen(ma)
	if err != nil {
		return nil, false, err
	}
	if proto == "" || port == "" {
		return nil, false, errors.UnknownError.WithFormat("invalid listen address: %v", ma)
	}
	addr += ":" + port

	l, err := net.Listen(proto, addr)
	return l, http == "https", err
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

type ipOffset int

func (i ipOffset) Apply(c multiaddr.Component) ([]multiaddr.Component, bool) {
	switch c.Protocol().Code {
	case multiaddr.P_IP4:
		// Ok
	default:
		return nil, false
	}

	base := net.ParseIP(c.Value())
	if base == nil {
		panic(fmt.Errorf("invalid IP address: %s", c.Value()))
	}

	ip := make(net.IP, len(base))
	copy(ip, base)
	for int(ip[15])+int(i) > 254 {
		i -= 255 - ipOffset(ip[15])
		ip[15] = 1
		ip[14]++
	}
	ip[15] += byte(i)
	d, err := multiaddr.NewComponent(c.Protocol().Name, ip.String())
	if err != nil {
		panic(err)
	}
	return []multiaddr.Component{*d}, true
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

	d, err := multiaddr.NewComponent("http", "")
	if err != nil {
		panic(err)
	}
	return []multiaddr.Component{c, *d}, true
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

func peersForDumbDialer(entries []*HttpPeerMapEntry) map[string][]peer.AddrInfo {
	m := map[string][]peer.AddrInfo{}
	for _, p := range entries {
		for _, part := range p.Partitions {
			part = strings.ToLower(part)
			m[part] = append(m[part], peer.AddrInfo{
				ID:    p.ID,
				Addrs: p.Addresses,
			})
		}
	}
	return m
}

func HaveConfiguration[T any](cfg *Config, predicate func(T) bool, existing *T) bool {
	for _, s := range cfg.Configurations {
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

func AddConfiguration[T Configuration](cfg *Config, s T, predicate func(T) bool) T {
	if !HaveConfiguration(cfg, predicate, &s) {
		cfg.Configurations = append(cfg.Configurations, s)
	}
	return s
}
