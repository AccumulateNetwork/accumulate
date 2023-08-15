// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"crypto/sha256"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
)

type Node struct {
	id        int
	network   *accumulated.NodeInit
	partition *Partition
	logger    log.Logger
	eventBus  *events.Bus
	nodeKey   []byte
	peerID    peer.ID
	consensus *consensus.Node
	database  *database.Database
	services  *message.Handler
}

// ConsensusStatus implements [api.ConsensusService].
func (n *Node) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	status, err := n.consensus.Status(&consensus.StatusRequest{})
	if err != nil {
		return nil, err
	}
	info, err := n.consensus.Info(&consensus.InfoRequest{})
	if err != nil {
		return nil, err
	}
	return &api.ConsensusStatus{
		Ok: true,
		LastBlock: &api.LastBlock{
			Height:    int64(status.BlockIndex),
			Time:      status.BlockTime,
			StateRoot: info.LastHash,
			// TODO: chain root, directory height
		},
		NodeKeyHash:      sha256.Sum256(n.nodeKey[32:]),
		ValidatorKeyHash: sha256.Sum256(n.network.PrivValKey[32:]),
		PartitionID:      n.partition.ID,
		PartitionType:    n.partition.Type,
	}, nil
}
func (n *Node) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return n.submit(envelope, false)
}

func (n *Node) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return n.submit(envelope, true)
}

func (n *Node) submit(envelope *messaging.Envelope, pretend bool) ([]*api.Submission, error) {
	st, err := n.partition.Submit(envelope, pretend)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	subs := make([]*api.Submission, len(st))
	for i, st := range st {
		// Create an api.Submission
		subs[i] = new(api.Submission)
		subs[i].Status = st
		subs[i].Success = st.Code.Success()
		if st.Error != nil {
			subs[i].Message = st.Error.Message
		}
	}

	return subs, nil
}
