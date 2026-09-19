// Copyright 2026 The Accumulate Authors
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

var (
	portDir = portOffset(config.PortOffsetDirectory)
	portBVN = portOffset(config.PortOffsetBlockValidator)

	portCmtP2P = portOffset(config.PortOffsetTendermintP2P)
	portAccAPI = portOffset(config.PortOffsetAccumulateApi)
	portAccP2P = portOffset(config.PortOffsetAccumulateP2P)
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
	registerRpcServiceIf(inst, addr, service, nil)
}

// registerRpcServiceIf registers a service whose OFFER — advertisement,
// NodeInfo listing, and this node's own dialer resolving to itself — is
// conditional. The handler is installed either way, so a peer holding a
// stale provider record gets the service's own answer. See
// [p2p.Node.RegisterServiceIf].
func registerRpcServiceIf(inst *Instance, addr *api.ServiceAddress, service message.Service, offer func() bool) {
	handler, err := message.NewHandler(service)
	if err != nil {
		panic(err)
	}
	inst.p2p.RegisterServiceIf(addr, handler.Handle, offer)
}

// offersService decides whether this node offers a service of the given
// type, given whether it is in the partition's current committee.
//
// Submit and validate go by committee membership: a node in no committee
// cannot get a submission into a block, so being found as a provider of
// either is what strands the traffic (#4366, executor.md Sync step 5 and step
// 6, "COMPLETE serves the services its committee membership gives it"). Every
// other service is a read, which a follower serves.
//
// The readiness half — a JOINING node advertising anything at all — is #4336
// and is not decided here: its latch has to defer the advertisement until the
// node can serve rather than skip it, because there is no un-advertise and
// util.Advertise republishes on its own schedule.
func offersService(typ api.ServiceType, inCommittee func() bool) bool {
	switch typ {
	case api.ServiceTypeSubmit, api.ServiceTypeValidate:
		return inCommittee == nil || inCommittee()
	default:
		return true
	}
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

// registerSubmitServices registers the two services a node offers only as a
// member of a partition's committee: submit and validate.
//
// The handlers are installed whatever the answer, so a peer that still holds
// this node in a DHT provider record — which lingers to its TTL, whatever
// the node does — gets the service's NotReady naming the reason, rather than
// a dial failure. What membership decides is whether the node is FOUND: the
// advertisement, the NodeInfo listing, and whether this node's own dialer
// resolves the service to itself (#4366).
func registerSubmitServices(inst *Instance, partition string, inCommittee func() bool, submitter api.Submitter, validator api.Validator) {
	offer := func(typ api.ServiceType) func() bool {
		return func() bool { return offersService(typ, inCommittee) }
	}
	registerRpcServiceIf(inst, api.ServiceTypeSubmit.AddressFor(partition),
		message.Submitter{Submitter: submitter}, offer(api.ServiceTypeSubmit))
	registerRpcServiceIf(inst, api.ServiceTypeValidate.AddressFor(partition),
		message.Validator{Validator: validator}, offer(api.ServiceTypeValidate))
}
