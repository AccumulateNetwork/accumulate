// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// What a joining node refused to take in, by the call it refused. A restarted
// validator that keeps accepting user traffic rejects every transaction of it
// against a store it has not filled, and nothing says so (#4307).
var mNotSubmitting = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "node",
	Name:      "not_submitting_total",
	Help:      "Submissions refused because this node is joining and cannot validate against state it has not executed",
}, []string{"partition", "call"})

// ConsensusAPIService implements api.ConsensusService for DAG-BFT.
type ConsensusAPIService struct {
	logger        logging.OptionalLogger
	service       *Service
	db            database.Viewer
	partitionID   string
	partitionType protocol.PartitionType
	partition     config.NetworkUrl
	nodeKeyHash   [32]byte
	valKeyHash    [32]byte
	heals         *crosschain.HealCounters
	nodeState     nodestate.Serving
	validatorKey  ed25519.PrivateKey
	peerID        peer.ID
}

var _ api.ConsensusService = (*ConsensusAPIService)(nil)

// ConsensusAPIServiceParams holds the parameters for creating a ConsensusAPIService.
type ConsensusAPIServiceParams struct {
	Logger           logging.Logger
	Service          *Service
	Database         database.Viewer
	PartitionID      string
	PartitionType    protocol.PartitionType
	EventBus         *events.Bus
	NodeKeyHash      [32]byte
	ValidatorKeyHash [32]byte

	// NodeState is this node's join state, reported as CatchingUp. It is
	// what tells a RELAY that this node, committee key and all, cannot
	// propose yet and would only relay again -- the one fact that bounds a
	// relay to a single hop (#4366; see relay.go).
	NodeState nodestate.Serving

	// ValidatorKey is this node's author key, used ONLY to answer a relay's
	// challenge: a caller that means to hand this node a submission asks it
	// to sign a nonce with the key whose hash it reports, so that naming a
	// validator's hash is not enough to be handed traffic (#4366 F1). Empty
	// means this node cannot answer a challenge, and no relay will choose
	// it.
	ValidatorKey ed25519.PrivateKey

	// PeerID is this node's own libp2p identity. It goes into what the
	// challenge signs, and this node refuses to sign for any other ID: a
	// signature that says only "a validator holds this key" can be got by
	// forwarding the nonce to that validator, and the peer that forwarded
	// it is then handed the submission (#4366, note_3869991754).
	PeerID peer.ID

	// Heals is shared with the conductor so recoveries are reportable, not
	// only loggable (#4075, #4105) — the soak monitor reads these fields.
	Heals *crosschain.HealCounters
}

// NewConsensusAPIService creates a new ConsensusAPIService.
func NewConsensusAPIService(params ConsensusAPIServiceParams) *ConsensusAPIService {
	s := new(ConsensusAPIService)
	s.logger.L = params.Logger
	s.service = params.Service
	s.db = params.Database
	s.partitionID = params.PartitionID
	s.partitionType = params.PartitionType
	s.partition.URL = protocol.PartitionUrl(params.PartitionID)
	s.nodeKeyHash = params.NodeKeyHash
	s.valKeyHash = params.ValidatorKeyHash
	s.heals = params.Heals
	s.nodeState = params.NodeState
	s.validatorKey = params.ValidatorKey
	s.peerID = params.PeerID
	return s
}

// Type returns the service type.
func (s *ConsensusAPIService) Type() api.ServiceType { return api.ServiceTypeConsensus }

// ConsensusStatus returns the current consensus status.
func (s *ConsensusAPIService) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	// Basic data
	res := new(api.ConsensusStatus)
	res.Ok = true
	if s.heals != nil {
		res.SyntheticHeals = s.heals.Synthetic.Load()
		res.AnchorHeals = s.heals.Anchor.Load()
	}
	res.Version = accumulate.Version
	res.Commit = accumulate.Commit
	res.NodeKeyHash = s.nodeKeyHash
	res.ValidatorKeyHash = s.valKeyHash
	res.PartitionID = s.partitionID
	res.PartitionType = s.partitionType

	// "Still catching up to the network" -- and therefore, for this
	// partition, still unable to propose. A relay reads this to avoid
	// handing a submission to a node that would only relay it again
	// (#4366).
	res.CatchingUp = s.nodeState != nil && !s.nodeState.CanServeCurrent()

	// A relay's challenge, answered with the key whose hash is above, as
	// THIS node and for nobody else. Only when one is asked: a node signs
	// nothing it was not asked to sign, and what it signs cannot be a
	// consensus message (relay_challenge.go).
	res.ChallengeSignature = signRelayChallenge(
		s.validatorKey, s.partitionID, s.peerID, opts.NodeID, opts.Challenge)

	// Load values from the database
	res.LastBlock = new(api.LastBlock)
	if s.db != nil && boolOpt(opts.IncludeAccumulate, true) {
		err := s.db.View(func(batch *database.Batch) error {
			c, err := batch.Account(s.partition.Ledger()).RootChain().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load root chain: %w", err)
			}
			res.LastBlock.ChainRoot = *(*[32]byte)(c.Anchor())

			c, err = batch.Account(s.partition.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load root anchor chain for the DN: %w", err)
			}
			res.LastBlock.DirectoryAnchorHeight = uint64(c.Height())
			return nil
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Get status from DAG-BFT service
	if s.service != nil {
		status := s.service.Status()
		res.LastBlock.Height = int64(status.LastBlockIndex)
		res.LastBlock.Time = status.LastBlockTime
		res.LastBlock.StateRoot = s.service.adapter.StateHash()
	}

	if !boolOpt(opts.IncludePeers, true) {
		return res, nil
	}

	// DAG-BFT doesn't have the same peer concept as CometBFT
	// Return empty peer list for now
	res.Peers = []*api.ConsensusPeerInfo{}

	return res, nil
}

func boolOpt(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// SubmitterService implements api.Submitter for DAG-BFT.
type SubmitterService struct {
	logger     logging.OptionalLogger
	service    *Service
	nodeState  nodestate.Serving
	membership *Membership
	relay      *Relay
}

var _ api.Submitter = (*SubmitterService)(nil)

// SubmitterServiceParams holds the parameters for creating a SubmitterService.
type SubmitterServiceParams struct {
	Logger  logging.Logger
	Service *Service

	// NodeState is this node's join state. A joining node refuses
	// submissions: it validates against a store the pull has half filled, so
	// every user transaction fails on an account it does not have yet, and the
	// caller is told its transaction is invalid when it is not (#4307 -- 15,035
	// of those in run 20260918T131713Z). Nil means the node never joined.
	//
	// A joining node does not DROP what it cannot propose, it relays it: a
	// relay reads no account and verifies no signature, so nothing about
	// the half-filled store reaches it (executor.md, "Sync" step 6).
	NodeState nodestate.Serving

	// Membership is this node's standing in the partition's current
	// committee. A node in no committee cannot get a submission into a
	// block: its header is dropped before any vote. Nil means the caller
	// applies no committee gate.
	Membership *Membership

	// Relay hands on what this node cannot propose. Nil means there is
	// nowhere to hand it to, and a node that cannot propose then answers
	// NotReady rather than taking what it would strand.
	Relay *Relay
}

// NewSubmitterService creates a new SubmitterService.
func NewSubmitterService(params SubmitterServiceParams) *SubmitterService {
	s := new(SubmitterService)
	s.logger.L = params.Logger
	s.service = params.Service
	s.nodeState = params.NodeState
	s.membership = params.Membership
	s.relay = params.Relay
	return s
}

// canPropose reports whether this node can get a submission for its partition
// into a block by proposing it itself.
//
// Two things stop it, and they are different facts with the same consequence:
// its author key is in no current committee of the partition, so every header
// it writes is dropped before any vote; or it is still joining, so it would
// validate against a store its pull has half filled and tell the sender its
// transaction is bad when it is not (#4307).
//
// A node that cannot propose does not refuse and does not keep it. It relays
// it (executor.md, "Sync" step 6; Paul, 2026-09-19: "Followers can relay txs.
// And should.").
func (s *SubmitterService) canPropose() bool {
	if !s.membership.CanPropose() {
		return false
	}
	return s.nodeState == nil || s.nodeState.CanServeCurrent()
}

// relayIt hands a submission this node cannot propose to one that can, and
// answers the caller with the target's answer.
//
// Synchronous on purpose: accept-and-forward would tell the sender its
// transaction is in hand while this node still has to find somewhere to put
// it, which is accept-then-drop under another name — the failure #4366 is.
func (s *SubmitterService) relayIt(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	partition := s.service.config.Partition.ID

	if s.relay == nil {
		// Nowhere to hand it to. Refusing is still better than taking it:
		// what this node takes for a partition it cannot propose for never
		// reaches a block at all. The reason is the one that will still be
		// true when the join finishes (consensus.md, invariant 10: a
		// refusal says why).
		mNotSubmitting.WithLabelValues(strings.ToLower(partition), "Submit").Inc()
		s.submitted("rejected")
		if !s.membership.CanPropose() {
			return nil, errors.NotReady.WithFormat(
				"this node is not in the current committee of %s and has no relay", partition)
		}
		return nil, errors.NotReady.WithFormat(
			"%s is joining and cannot validate against state it has not executed", partition)
	}

	res, outcome, err := s.relay.Submit(ctx, envelope, opts)

	// Once per submission, at its final answer, and one accepted with it
	// (#4366 note_3869838619).
	//
	// Every submission that enters the relay is accepted: this node took
	// responsibility for it, however it then discharged it. That is what
	// makes the harness's arithmetic hold — relayed_total <= accepted per
	// node and partition, and "accepted, neither certified here nor taken
	// on relay" = accepted - certified - relayed{taken}, which is then
	// exactly the relays that did not reach a proposer, "where a drop
	// belongs" (soakmon.py, the relay block). Counting only taken as
	// accepted instead makes sum(relayed) > accepted fire on a correct
	// build the first time one relay ends unreachable, and hides a node
	// whose relays never land. The contract also says accepted is "Submit
	// returned success to the caller", which for a refused or unreachable
	// relay it was not; the two readings cannot both hold and the
	// arithmetic is the one the harness computes (#4366, builder's note).
	metrics.RelayedTotal.WithLabelValues(partition, outcome).Inc()
	s.submitted("accepted")
	return res, err
}

// submitted records what this node did with a submission, as the caller saw
// it: accepted means Submit returned success, rejected means it did not.
// Read against the certified-own and relayed counters, the difference is what
// this node took and neither proposed nor handed on (#4366, #4369).
func (s *SubmitterService) submitted(outcome string) {
	// The partition ID verbatim -- "Directory", "BVN3" -- as the harness
	// that reads this family spells it: it takes the label as a key and
	// joins the three families on it, and every container runs two nodes, a
	// DN node and a BVN node, whose queues are separate.
	metrics.SubmissionsTotal.WithLabelValues(s.service.config.Partition.ID, outcome).Inc()
}

// Type returns the service type.
func (s *SubmitterService) Type() api.ServiceType { return api.ServiceTypeSubmit }

// signerOf returns a stable routing key for an envelope: the URL of the signer
// of its first signature.
//
// Everything signed by one key must be handled by one worker, or it is batched
// in parallel, committed out of order, and rejected by replay protection —
// which requires a signer's timestamps to be strictly increasing in EXECUTION
// order (#4132).
//
// Falls back to the legacy Signatures field, then to empty (round-robin) if
// there is nothing to key on.
func signerOf(envelope *messaging.Envelope) string {
	for _, m := range envelope.Messages {
		if sm, ok := m.(*messaging.SignatureMessage); ok && sm.Signature != nil {
			if u := sm.Signature.GetSigner(); u != nil {
				return u.String()
			}
		}
	}
	for _, sig := range envelope.Signatures {
		if u := sig.GetSigner(); u != nil {
			return u.String()
		}
	}
	return ""
}

// Submit submits an envelope to the DAG-BFT consensus.
func (s *SubmitterService) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	// Identify what is being submitted: the contentless version of this trace
	// made it impossible to follow a specific lost message (#4111) through
	// accept → batch → commit → execute.
	var msgIDs []string
	for _, m := range envelope.Messages {
		msgIDs = append(msgIDs, fmt.Sprintf("%v:%v", m.Type(), m.ID()))
	}
	s.logger.Debug("TRACE-SUBMIT: SubmitterService.Submit() called (DAG-BFT)",
		"messages", strings.Join(msgIDs, ","),
		"partition", s.service.config.Partition.ID)

	// Verify the envelope is well-formed -- BEFORE the relay, and on the
	// same terms as the local path.
	//
	// This is the relaying node's own admission, and it is not validation:
	// Normalize decodes and checks shape, reads no account and touches no
	// store, so nothing about a half-filled state reaches it (executor.md,
	// "Sync" step 6, "not validated, decoded only to route"). Without it a
	// follower turns B bytes of garbage into B bytes at every validator of
	// the partition, which is the amplification F3 names; with it garbage
	// costs one node one decode and never leaves it. A caller that asks for
	// no verification gets none, here as before.
	if opts.Verify == nil || *opts.Verify {
		_, err := envelope.Normalize()
		if err != nil {
			s.logger.Error("TRACE-SUBMIT: envelope normalization failed", "error", err)
			s.submitted("rejected")
			return nil, errors.BadRequest.WithFormat("verify: %w", err)
		}
	}

	// What this node cannot propose it relays: judging a partition it is in
	// no committee of, or validating against a store its pull has half
	// filled, answers a question it is not the one to answer (executor.md,
	// "Sync" step 6).
	if !s.canPropose() {
		return s.relayIt(ctx, envelope, opts)
	}

	// Marshal envelope
	b, err := envelope.MarshalBinary()
	if err != nil {
		s.logger.Error("TRACE-SUBMIT: envelope marshaling failed", "error", err)
		s.submitted("rejected")
		return nil, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	// Route by signer. Everything signed by one key must be handled by one
	// worker, or it is batched in parallel, committed out of order, and
	// rejected by replay protection (#4132).
	// Routing key, for diagnostics only — it is NOT used to pick a worker.
	//
	// Keying the shard on the signer was tried and reverted. It serialises a
	// hot signer onto one worker, which is the opposite of what sharding is
	// for, and it does not even work: in run 20260822T071949Z all 100 of the
	// treasury's transactions landed in worker 12 exactly as designed, that
	// worker produced SIX batches, and 81 of the 100 were still rejected —
	// because batches commit in DAG order, so order survives inside a batch
	// and is lost across batches. The ordering constraint lives in the
	// executor's replay protection; it cannot be fixed by routing (#4132).
	routeKey := signerOf(envelope)

	s.logger.Debug("TRACE-SUBMIT: submitting to DAG-BFT service.SubmitTransaction",
		"routeKey", routeKey)

	// Submit to consensus (includes pre-batch validation)
	// User traffic is bounded by the store's budget; system traffic --
	// synthetics, anchors, the healer's re-submissions -- is what drains it
	// and is never refused (consensus spec, invariant 4; #4165).
	submit := s.service.SubmitTransaction
	if isUserEnvelope(envelope) {
		submit = s.service.SubmitUserTransaction
	}
	if err := submit(b); err != nil {
		// Every road out of here is a rejection: the envelope did not enter
		// this node's worker and Submit does not return success.
		s.submitted("rejected")
		if stderrors.Is(err, worker.ErrStoreFull) || stderrors.Is(err, worker.ErrExecutionLagging) {
			// Retry later: the answer every internal client already handles.
			// The reason travels in the error (consensus spec, invariant 10).
			return nil, errors.NotReady.WithFormat("submit: %w", err)
		}
		// Check if this is a validation error
		if stderrors.Is(err, worker.ErrValidationFailed) {
			s.logger.Error("TRACE-SUBMIT: validation failed, returning error submission", "error", err)
			return []*api.Submission{{
				Success: false,
				Message: fmt.Sprintf("Transaction validation failed: %v", err),
				Status: &protocol.TransactionStatus{
					TxID: nil,
					Code: errors.Pending,
				},
			}}, nil
		}
		if stderrors.Is(err, worker.ErrBackpressure) {
			s.logger.Error("TRACE-SUBMIT: backpressure error", "error", err)
			return nil, errors.TooManyRequests.WithFormat("submit: %w", err)
		}
		// An oversized transaction is the CALLER's fault, permanently — no
		// batch will ever fit it. BadRequest, not InternalError, so the
		// submitter stops rather than retrying or blaming the node (#4151).
		if stderrors.Is(err, worker.ErrTransactionTooLarge) {
			return nil, errors.BadRequest.WithFormat("submit: %w", err)
		}
		s.logger.Error("TRACE-SUBMIT: internal error", "error", err)
		return nil, errors.InternalError.WithFormat("submit: %w", err)
	}

	// Accepted: it is in this node's worker. Whether it ever reaches a
	// certified header is the other half of the measurement.
	s.submitted("accepted")

	s.logger.Debug("TRACE-SUBMIT: submission successful, creating result WITHOUT Status field (BUG!)")

	// Return success - DAG-BFT doesn't have synchronous result like CometBFT
	result := []*api.Submission{{
		Success: true,
		Message: "Transaction submitted to DAG-BFT consensus",
		Status: &protocol.TransactionStatus{
			TxID: nil,
			Code: errors.Pending,
		},
	}}

	s.logger.Debug("TRACE-SUBMIT: returning result", "submission_count", len(result), "status_is_nil", result[0].Status == nil)

	return result, nil
}

// ValidatorService implements api.Validator for DAG-BFT.
type ValidatorService struct {
	logger    logging.OptionalLogger
	service   *Service
	nodeState nodestate.Serving
}

var _ api.Validator = (*ValidatorService)(nil)

// ValidatorServiceParams holds the parameters for creating a ValidatorService.
type ValidatorServiceParams struct {
	Logger  logging.Logger
	Service *Service

	// NodeState is this node's join state; see SubmitterServiceParams.
	NodeState nodestate.Serving
}

// NewValidatorService creates a new ValidatorService.
func NewValidatorService(params ValidatorServiceParams) *ValidatorService {
	s := new(ValidatorService)
	s.logger.L = params.Logger
	s.service = params.Service
	s.nodeState = params.NodeState
	return s
}

// Type returns the service type.
func (s *ValidatorService) Type() api.ServiceType { return api.ServiceTypeValidate }

// Validate validates an envelope without submitting it.
//
// Validate is a READ, and reads divide from relays on whether local state is
// needed: a joining node refuses it, because it would judge against a store
// its pull has half filled (#4307), and a node that is synced answers it
// from its own state WHATEVER ITS COMMITTEE, because a validation judges
// against the latest committed state and promises nothing about proposal
// (executor.md, "Sync" step 6). So there is no committee gate here, and
// nothing to relay: the answer needs no proposer.
func (s *ValidatorService) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	if s.nodeState != nil && !s.nodeState.CanServeCurrent() {
		mNotSubmitting.WithLabelValues(strings.ToLower(s.service.config.Partition.ID), "Validate").Inc()
		return nil, errors.NotReady.WithFormat(
			"%s is joining and cannot validate against state it has not executed", s.service.config.Partition.ID)
	}

	// Marshal envelope
	b, err := envelope.MarshalBinary()
	if err != nil {
		return nil, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	// Validate using adapter
	if err := s.service.adapter.ValidateTransaction(b); err != nil {
		return []*api.Submission{{
			Success: false,
			Message: fmt.Sprintf("Validation failed: %v", err),
		}}, nil
	}

	return []*api.Submission{{
		Success: true,
		Message: "Transaction is valid",
	}}, nil
}

// isUserEnvelope reports whether an envelope carries user traffic rather than
// the system's own: a synthetic, sequenced or anchor message anywhere in it
// makes it system traffic.
func isUserEnvelope(env *messaging.Envelope) bool {
	for _, m := range env.Messages {
		switch m.Type() {
		case messaging.MessageTypeSynthetic, messaging.MessageTypeSequenced, messaging.MessageTypeBlockAnchor,
			messaging.MessageTypeBadSynthetic, messaging.MessageTypeNetworkUpdate, messaging.MessageTypeSyntheticProof:
			return false
		}
	}
	return true
}
