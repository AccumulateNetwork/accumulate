// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Router routes an account to the partition that holds it. It is the node's
// own router; the join needs it to know which partition's peers to ask for an
// account, because a block names accounts of every partition.
type Router interface {
	RouteAccount(*url.URL) (string, error)
}

// Sources is where the state half gets the peers to pull from. It is an
// interface so a test can supply peers without a network, and so that nothing
// can hand the join a routed client again: a routed client answers from this
// node.
type Sources interface {
	// For returns one source per peer that can answer for the account, and
	// the partition those peers answer for -- never this node. The account is
	// routed to a partition and that partition's query service is looked up.
	// The partition comes back because a receipt proves the state as of a
	// block and block numbers collide across partitions (#4308).
	For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error)

	// Querier reads a partition through whichever of its peers answers,
	// never this node. It is for reads that stand on their own -- the
	// Directory's anchor chain, a peer's BPT pages, a peer's block ledger --
	// where one peer answering this call and another the next is no worse
	// than one peer answering both.
	Querier(partition *url.URL) api.Querier

	// ValidatorsOf is the partition's validators, each reached by name --
	// never this node. The join collects its partition's own anchors from
	// them (executor spec, "Sync", "The algorithm", step 3).
	anchorsrc.Validators
}

// QueryPeers finds the peers serving a partition's query service and addresses
// each one by name.
//
// **A joining node must never pull from itself.** The node's own routed client
// answers locally for any service the node provides -- p2p.DialNetwork installs
// a self-discoverer unconditionally ("Always use self-discovery",
// dial_network.go), and every partition a node serves has its query:<id>
// registered -- so a pull given that client reads the node's own un-executed
// store and is refused by it, forever (#4303). Addressing a named peer is the
// rule APIPeers already states for staging: the join has to know whose state it
// took, and to move on from one that cannot serve it.
type QueryPeers struct {
	// Client reaches the network. FindService is asked of it unaddressed;
	// every read is addressed at one peer.
	Client *message.Client

	// Network is the network this node belongs to. A service is advertised
	// under its network's key, so a search that does not name one finds
	// nothing (#4296).
	Network string

	// Router routes an account to its partition.
	Router Router

	// Self is this node's peer ID. It is dropped from every list.
	Self peer.ID

	// TTL is how long a peer list is reused before it is looked up again.
	// Zero means DefaultPeerTTL.
	TTL time.Duration

	mu     sync.Mutex
	cached map[string]*peerList
}

// DefaultPeerTTL is how long a discovered peer list is reused. A join runs for
// as long as a node takes to catch up and asks for many accounts a second;
// looking the partition up for every one of them would spend the join in
// discovery.
const DefaultPeerTTL = 30 * time.Second

type peerList struct {
	peers []peer.ID
	read  time.Time
}

var _ Sources = (*QueryPeers)(nil)

// For implements [Sources].
func (q *QueryPeers) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	if q.Router == nil {
		return nil, nil, errors.BadRequest.With("no router: the join cannot tell which partition holds an account")
	}
	id, err := q.Router.RouteAccount(account)
	if err != nil {
		// A name nothing can route is not an account any peer has. It is
		// dropped rather than retried forever (#4306).
		return nil, nil, errors.BadRequest.WithFormat("route %v: %w", account, err)
	}
	partition := protocol.PartitionUrl(id)
	srcs, err := q.ForPartition(ctx, partition)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	return srcs, partition, nil
}

// ForPartition returns one source per peer serving the partition's querier.
func (q *QueryPeers) ForPartition(ctx context.Context, partition *url.URL) ([]pull.Source, error) {
	peers, err := q.peersOf(ctx, partition)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	srcs := make([]pull.Source, 0, len(peers))
	for _, p := range peers {
		srcs = append(srcs, peerSource{api.Querier2{Querier: q.clientFor(p, partition)}, p})
	}
	return srcs, nil
}

// ValidatorsOf implements [anchorsrc.Validators]: the peers serving the
// partition's sequencer, minus this node, each addressed by name. A joining
// node collects its partition's own anchors from them (executor spec, "Sync",
// "The algorithm", step 3).
func (q *QueryPeers) ValidatorsOf(ctx context.Context, partition *url.URL) ([]anchorsrc.Validator, error) {
	if q.Client == nil {
		return nil, errors.NotReady.With("no network client")
	}
	id := partitionIDOf(partition)
	addr := private.ServiceTypeSequencer.AddressFor(id)
	found, err := q.Client.FindService(ctx, api.FindServiceOptions{Network: q.Network, Service: addr})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("find the validators of %s: %w", id, err)
	}
	var out []anchorsrc.Validator
	for _, p := range selectPeers(found, q.Self) {
		out = append(out, anchorsrc.Validator{
			Name:      "peer " + p.String(),
			Sequencer: q.Client.ForPeer(p).ForAddress(addr.Multiaddr()).Private(),
		})
	}
	return out, nil
}

var _ anchorsrc.Validators = (*QueryPeers)(nil)

// peerSource is one peer's querier, named by the peer, so that the pull can
// say which peer answered what (#4397 review F3).
type peerSource struct {
	api.Querier2
	peer peer.ID
}

func (p peerSource) String() string { return "peer " + p.peer.String() }

// Querier implements [Sources].
func (q *QueryPeers) Querier(partition *url.URL) api.Querier {
	return &peerQuerier{peers: q, partition: partition}
}

// clientFor addresses one peer's query service. The address carries a service
// address, so the transport skips routing and the dialer opens a stream to that
// peer: the self-discoverer is never consulted.
func (q *QueryPeers) clientFor(id peer.ID, partition *url.URL) api.Querier {
	addr := api.ServiceTypeQuery.AddressFor(partitionIDOf(partition)).Multiaddr()
	return q.Client.ForPeer(id).ForAddress(addr)
}

func partitionIDOf(partition *url.URL) string {
	id, ok := protocol.ParsePartitionUrl(partition)
	if !ok {
		return partition.Authority
	}
	return id
}

// peersOf lists the partition's query-service peers, minus this node, reusing
// the last list for TTL.
func (q *QueryPeers) peersOf(ctx context.Context, partition *url.URL) ([]peer.ID, error) {
	if q.Client == nil {
		return nil, errors.NotReady.With("no network client")
	}
	key := partitionIDOf(partition)

	ttl := q.TTL
	if ttl <= 0 {
		ttl = DefaultPeerTTL
	}

	q.mu.Lock()
	cached, ok := q.cached[key]
	q.mu.Unlock()
	if ok && len(cached.peers) > 0 && time.Since(cached.read) < ttl {
		return cached.peers, nil
	}

	found, err := q.Client.FindService(ctx, api.FindServiceOptions{
		Network: q.Network,
		Service: api.ServiceTypeQuery.AddressFor(key),
	})
	if err != nil {
		if ok && len(cached.peers) > 0 {
			return cached.peers, nil // the last list is better than none
		}
		return nil, errors.UnknownError.WithFormat("find the peers serving %s: %w", key, err)
	}

	peers := selectPeers(found, q.Self)
	if len(peers) == 0 {
		return nil, errors.NotReady.WithFormat(
			"no peer other than this node serves %s; this node cannot pull its state", key)
	}

	q.mu.Lock()
	if q.cached == nil {
		q.cached = map[string]*peerList{}
	}
	q.cached[key] = &peerList{peers: peers, read: time.Now()}
	q.mu.Unlock()
	return peers, nil
}

// selectPeers is the list minus this node.
//
// **Never this node.** A joining node's store is the thing the pull exists to
// fill, so reading it back answers every question with what the node already
// does not have -- 18,313 refusals in run 20260918T131713Z, every one of them
// the node's own querier saying it does not have an account (#4303). The
// routed client the join used to be given makes this invisible: it answers
// locally for any service the node provides, without touching the network.
func selectPeers(found []*api.FindServiceResult, self peer.ID) []peer.ID {
	peers := make([]peer.ID, 0, len(found))
	for _, r := range found {
		if r == nil || r.PeerID == "" || r.PeerID == self {
			continue
		}
		peers = append(peers, r.PeerID)
	}
	return peers
}

// peerQuerier reads one partition through its peers, in order, taking the first
// answer. A read it cannot get from any peer is an error and not an empty
// answer: a join that read "nobody answered" as "there is nothing" would
// conclude it had caught up with an empty network.
type peerQuerier struct {
	peers     *QueryPeers
	partition *url.URL

	mu    sync.Mutex
	next  int       // rotates, so one peer does not serve every call
	asked []peer.ID // the peers the last call asked, in order
}

var _ anchorsrc.Peers = (*peerQuerier)(nil)

// PeerCount is how many peers the calls rotate among, so the anchor source
// asks each of them once for an entry it is held at (#4419).
func (p *peerQuerier) PeerCount(ctx context.Context) int {
	ids, err := p.peers.peersOf(ctx, p.partition)
	if err != nil {
		return 0
	}
	return len(ids)
}

// LastAsked names the peers the last call asked, so a stall can say whom it
// asked (#4419).
func (p *peerQuerier) LastAsked() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.asked))
	for i, id := range p.asked {
		out[i] = id.String()
	}
	return out
}

func (p *peerQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	ids, err := p.peers.peersOf(ctx, p.partition)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	p.mu.Lock()
	start := p.next
	p.next++
	p.asked = p.asked[:0]
	p.mu.Unlock()

	var last error
	var notFound error
	var sawNotFound int
	for i := range ids {
		id := ids[(start+i)%len(ids)]
		p.mu.Lock()
		p.asked = append(p.asked, id)
		p.mu.Unlock()
		rec, err := p.peers.clientFor(id, p.partition).Query(ctx, scope, query)
		if err == nil {
			return rec, nil
		}
		if ctx.Err() != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		// NotFound is the peer's answer about the record, and it is the
		// network's answer only once every peer has given it. A peer that is
		// itself joining holds an empty store and answers NotFound for
		// everything it has not pulled yet, so taking the first one as the
		// network's answer ends the read while a peer that holds the record is
		// standing right there. That matters because the callers read NotFound
		// as a fact about the network and not about the peer: the block-ledger
		// walk reads it as an empty block, and the anchor read as a directory
		// that has executed no anchors.
		if errors.Is(err, errors.NotFound) {
			notFound = err
			sawNotFound++
			continue
		}
		last = err
	}
	// Only every peer agreeing makes NotFound the network's answer. A mixture
	// of NotFound and failures says nothing, so it is an error and the caller
	// asks again rather than concluding there is nothing there.
	if sawNotFound == len(ids) && len(ids) > 0 {
		return nil, notFound
	}
	if last == nil {
		last = notFound
	}
	return nil, errors.UnknownError.WithFormat("no peer of %v answered: %w", p.partition, last)
}
