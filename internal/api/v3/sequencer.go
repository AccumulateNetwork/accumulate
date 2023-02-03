// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"sync/atomic"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Sequencer struct {
	logger      logging.OptionalLogger
	db          database.Viewer
	partitionID string
	partition   config.NetworkUrl
	valKey      []byte
	globals     atomic.Value
}

var _ private.Sequencer = (*Sequencer)(nil)

type SequencerParams struct {
	Logger       log.Logger
	Database     database.Viewer
	EventBus     *events.Bus
	Globals      *core.GlobalValues
	Partition    string
	ValidatorKey []byte
}

func NewSequencer(params SequencerParams) *Sequencer {
	s := new(Sequencer)
	s.logger.L = params.Logger
	s.db = params.Database
	s.partitionID = params.Partition
	s.partition.URL = protocol.PartitionUrl(params.Partition)
	s.valKey = params.ValidatorKey
	s.globals.Store(params.Globals)
	events.SubscribeAsync(params.EventBus, func(e events.WillChangeGlobals) {
		s.globals.Store(e.New)
	})
	return s
}

func (s *Sequencer) Sequence(ctx context.Context, src, dst *url.URL, num uint64) (*api.TransactionRecord, error) {
	if !s.partition.URL.ParentOf(src) {
		return nil, errors.BadRequest.WithFormat("requested source is %s but this partition is %s", src.RootIdentity(), s.partitionID)
	}

	globals := s.globals.Load().(*core.GlobalValues)
	if globals == nil {
		return nil, errors.NotReady
	}

	// Starting a batch would not be safe if the ABCI were updated to commit in
	// the middle of a block

	var r *api.TransactionRecord
	var err error
	switch {
	case s.partition.Synthetic().Equal(src):
		return r, s.db.View(func(batch *database.Batch) error {
			r, err = s.getSynth(batch, globals, dst, num)
			return err
		})

	case s.partition.AnchorPool().Equal(src):
		return r, s.db.View(func(batch *database.Batch) error {
			r, err = s.getAnchor(batch, globals, dst, num)
			return err
		})
	}

	return nil, errors.BadRequest.WithFormat("invalid source: %s", src)
}

func (s *Sequencer) getAnchor(batch *database.Batch, globals *core.GlobalValues, dst *url.URL, num uint64) (*api.TransactionRecord, error) {
	chain, err := batch.Account(s.partition.AnchorPool()).AnchorSequenceChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}
	hash, err := chain.Entry(int64(num) - 1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", num-1, err)
	}

	var msg messaging.MessageWithTransaction
	err = batch.Message2(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = dst.JoinPath(protocol.AnchorPool)
	txn.Body = msg.GetTransaction().Body

	var signatures []protocol.Signature
	if globals.ExecutorVersion.V2() {
		txn.Header.Source = s.partition.URL
		txn.Header.Destination = dst
		txn.Header.SequenceNumber = num

	} else {
		// Create a partition signature
		partSig, err := new(signing.Builder).
			SetUrl(s.partition.URL).
			SetVersion(num).
			InitiateSynthetic(txn, dst)
		if err != nil {
			return nil, errors.InternalError.Wrap(err)
		}
		signatures = append(signatures, partSig)
	}

	// Create a key signature
	signer := &protocol.UnknownSigner{Url: s.partition.JoinPath(protocol.Network)}
	keySig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(s.valKey).
		SetUrl(signer.Url).
		SetVersion(globals.Network.Version).
		SetTimestamp(1).
		Sign(txn.GetHash())
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	signatures = append(signatures, keySig)

	r := new(api.TransactionRecord)
	r.Transaction = txn
	r.TxID = txn.ID()
	r.Signatures = new(api.RecordRange[*api.SignatureRecord])
	r.Signatures.Total = 1
	r.Signatures.Records = []*api.SignatureRecord{
		{Signature: &protocol.SignatureSet{
			Vote:            protocol.VoteTypeAccept,
			Signer:          signer.Url,
			Authority:       signer.Url,
			TransactionHash: txn.ID().Hash(),
			Signatures:      signatures,
		}, TxID: txn.ID(), Signer: signer},
	}
	return r, nil
}

func (s *Sequencer) getSynth(batch *database.Batch, globals *core.GlobalValues, dst *url.URL, num uint64) (*api.TransactionRecord, error) {
	partition, ok := protocol.ParsePartitionUrl(dst)
	if !ok {
		return nil, errors.UnknownError.WithFormat("destination is not a partition")
	}
	ledger := batch.Account(s.partition.Synthetic())
	chain, err := ledger.SyntheticSequenceChain(partition).Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic sequence chain: %w", err)
	}
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(int64(num)-1, entry)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic sequence chain entry %d: %w", num-1, err)
	}
	chain, err = ledger.MainChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic main chain: %w", err)
	}
	hash, err := chain.Entry(int64(entry.Source))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic chain entry %d: %w", entry.Source, err)
	}

	var msg messaging.MessageWithTransaction
	err = batch.Message2(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	status, err := batch.Transaction(hash).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Proof == nil {
		return nil, errors.NotReady.WithFormat("no proof yet")
	}

	var signatures []protocol.Signature

	if !globals.ExecutorVersion.V2() {
		// Add the partition signature
		partSig := new(protocol.PartitionSignature)
		partSig.SourceNetwork = status.SourceNetwork
		partSig.DestinationNetwork = status.DestinationNetwork
		partSig.SequenceNumber = status.SequenceNumber
		partSig.TransactionHash = *(*[32]byte)(hash)
		signatures = append(signatures, partSig)

		// Add the receipt signature
		receiptSig := new(protocol.ReceiptSignature)
		receiptSig.SourceNetwork = status.SourceNetwork
		if status.Proof != nil {
			receiptSig.Proof = *status.Proof
		}
		receiptSig.TransactionHash = *(*[32]byte)(hash)
		signatures = append(signatures, receiptSig)
	}

	// Add the key signature
	signer := &protocol.UnknownSigner{Url: s.partition.JoinPath(protocol.Network)}
	keySig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(s.valKey).
		SetUrl(signer.Url).
		SetVersion(globals.Network.Version).
		SetTimestamp(1).
		Sign(hash)
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	signatures = append(signatures, keySig)

	r := new(api.TransactionRecord)
	r.Transaction = msg.GetTransaction()
	r.TxID = msg.GetTransaction().ID()
	r.Signatures = new(api.RecordRange[*api.SignatureRecord])
	r.Signatures.Total = 1
	r.Signatures.Records = []*api.SignatureRecord{
		{Signature: &protocol.SignatureSet{
			Vote:            protocol.VoteTypeAccept,
			Signer:          signer.Url,
			Authority:       signer.Url,
			TransactionHash: msg.GetTransaction().ID().Hash(),
			Signatures:      signatures,
		}, TxID: msg.GetTransaction().ID(), Signer: signer},
	}
	r.Status = status
	return r, nil
}
