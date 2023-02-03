// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"crypto/sha256"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// simService implements API v3 for a simulator.
type simService Simulator

// Services returns the simulator's API v3 implementation.
func (s *Simulator) Services() *simService { return (*simService)(s) }

// Private returns the service, because it implements [private.Sequencer]
// directly.
func (s *simService) Private() private.Sequencer { return s }

// NodeStatus finds the specified node and returns its NodeStatus.
func (s *simService) NodeStatus(ctx context.Context, opts api.NodeStatusOptions) (*api.NodeStatus, error) {
	if opts.NodeID == "" {
		return nil, errors.BadRequest.WithFormat("node ID is missing")
	}
	id, err := peer.Decode(opts.NodeID)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid peer ID: %w", err)
	}
	for _, n := range s.Executors {
		sk, err := crypto.UnmarshalEd25519PrivateKey(n.nodeKey)
		if err != nil {
			continue
		}
		if id.MatchesPrivateKey(sk) {
			return n.service.NodeStatus(ctx, opts)
		}
	}
	return nil, errors.NotFound.WithFormat("node %s not found", id)
}

// NetworkStatus implements pkg/api/v3.NetworkService.
func (s *simService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	p, ok := s.Executors[opts.Partition]
	if !ok {
		return nil, errors.NotFound.WithFormat("%q is not a partition", opts.Partition)
	}
	return p.service.NetworkStatus(ctx, opts)
}

// Metrics implements pkg/api/v3.MetricsService.
func (s *simService) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	p, ok := s.Executors[opts.Partition]
	if !ok {
		return nil, errors.NotFound.WithFormat("%q is not a partition", opts.Partition)
	}
	return p.service.Metrics(ctx, opts)
}

// Query routes the scope to a partition and calls Query on the first node of
// that partition, returning the result.
func (s *simService) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	part, err := s.router.RouteAccount(scope)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return s.Executors[part].service.Query(ctx, scope, query)
}

// Submit routes the envelope to a partition and calls Submit on the first node
// of that partition, returning the result.
func (s *simService) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	part, err := s.router.Route(envelope)
	if err != nil {
		return nil, err
	}
	return s.Executors[part].service.Submit(ctx, envelope, opts)
}

// Validate routes the envelope to a partition and calls Validate on the first
// node of that partition, returning the result.
func (s *simService) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	part, err := s.router.Route(envelope)
	if err != nil {
		return nil, err
	}
	return s.Executors[part].service.Validate(ctx, envelope, opts)
}

// Sequence routes the source to a partition and calls Sequence on the first
// node of that partition, returning the result.
func (s *simService) Sequence(ctx context.Context, src, dst *url.URL, num uint64) (*api.TransactionRecord, error) {
	part, err := s.router.RouteAccount(src)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return s.Executors[part].service.Private().Sequence(ctx, src, dst, num)
}

// partService implements API v3 for a partition.
type partService struct {
	x       *ExecEntry
	query   api.Querier
	private private.Sequencer
}

// newExecService returns a new partService for the given partition.
func newExecService(x *ExecEntry, logger log.Logger) *partService {
	s := new(partService)
	s.x = x
	s.query = apiimpl.NewQuerier(apiimpl.QuerierParams{
		Logger:    logger.With("module", "acc-rpc"),
		Database:  x,
		Partition: x.Partition.Id,
	})
	s.private = apiimpl.NewSequencer(apiimpl.SequencerParams{
		Logger:       logger.With("module", "acc-rpc"),
		Database:     x,
		EventBus:     x.Executor.EventBus,
		Partition:    x.Partition.Id,
		ValidatorKey: x.Executor.Key,
	})
	return s
}

// Private returns the service, because it implements [private.Sequencer]
// directly.
func (s *partService) Private() private.Sequencer { return s.private }

// NodeStatus returns an incomplete node status. Some of the missing
// functionality can be implemented if there is need.
func (s *partService) NodeStatus(ctx context.Context, opts api.NodeStatusOptions) (*api.NodeStatus, error) {
	return &api.NodeStatus{
		Ok: true,
		LastBlock: &api.LastBlock{
			Height: int64(s.x.BlockIndex),
			Time:   s.x.blockTime,
			// TODO: chain root, state root
		},
		NodeKeyHash:      sha256.Sum256(s.x.nodeKey[32:]),
		ValidatorKeyHash: sha256.Sum256(s.x.Executor.Key[32:]),
		PartitionID:      s.x.Partition.Id,
		PartitionType:    s.x.Partition.Type,
	}, nil
}

// NetworkStatus returns the active globals.
func (s *partService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return &api.NetworkStatus{
		Oracle:  s.x.Executor.ActiveGlobals_TESTONLY().Oracle,
		Network: s.x.Executor.ActiveGlobals_TESTONLY().Network,
		Globals: s.x.Executor.ActiveGlobals_TESTONLY().Globals,
		Routing: s.x.Executor.ActiveGlobals_TESTONLY().Routing,
	}, nil
}

// Metrics returns a "not implemented" error.
func (s *partService) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return nil, errors.InternalError.With("not implemented")
}

// Query calls the partition's query service.
func (s *partService) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	r, err := s.query.Query(ctx, scope, query)
	if err != nil {
		return nil, err
	}
	// Force despecialization of generic types
	b, err := r.MarshalBinary()
	require.NoError(s.x, err)
	r, err = api.UnmarshalRecord(b)
	require.NoError(s.x, err)
	return r, nil
}

// Submit submits the envelope.
func (s *partService) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	var r []*api.Submission
	for _, d := range s.x.Submit(false, envelope) {
		r = append(r, &api.Submission{
			Success: true,
			Status:  &protocol.TransactionStatus{TxID: d.Transaction.ID()},
		})
	}
	return r, nil
}

// Validate validates the envelope.
func (s *partService) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	var r []*api.Submission
	for _, d := range s.x.Submit(true, envelope) {
		r = append(r, &api.Submission{
			Success: true,
			Status:  &protocol.TransactionStatus{TxID: d.Transaction.ID()},
		})
	}
	return r, nil
}
