// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"context"
	"fmt"
	"log/slog"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Every consensus topic has a validator, and the router delivers or relays a
// message only once its topic's validator accepts it (docs/spec/consensus.md,
// "Who may send consensus messages"). Checks run cheapest first: the decode,
// then the committee of the epoch the message names, then the author's
// membership of it, then signatures. A message that is provably bad is
// rejected; one naming an epoch this node holds no committee for is ignored,
// neither delivered nor relayed but not counted as invalid. A validator
// that accepts leaves the decoded message in ValidatorData for the handler.

// committees is what messages are judged by: the current committee and the
// one before it, so messages of the previous epoch are still judged by their
// own committee around a switch.
type committees struct {
	current, previous *types.Committee
}

// SetCommittee sets the committee consensus messages are checked against.
// The committee it replaces is kept for messages of its epoch, unless the
// new one has the same epoch. Until it is called every message that needs a
// committee is ignored.
func (g *GossipLayer) SetCommittee(c *types.Committee) {
	if c == nil {
		return
	}
	g.committeeMu.Lock()
	defer g.committeeMu.Unlock()
	next := &committees{current: c}
	if old := g.committees.Load(); old != nil {
		if old.current.Epoch != c.Epoch {
			next.previous = old.current
		} else {
			next.previous = old.previous
		}
	}
	g.committees.Store(next)
}

// SetHeldCertificate gives the certificate validator a way to find the
// certificate the node holds for a digest, or nil. A copy of a held
// certificate, byte for byte, is then accepted on the verification that
// admitted it.
func (g *GossipLayer) SetHeldCertificate(held func(types.CertificateDigest) *types.Certificate) {
	if held == nil {
		g.held.Store(nil)
		return
	}
	g.held.Store(&held)
}

// heldCertificate returns the certificate the node holds for the digest, or
// nil if none or if SetHeldCertificate was not called.
func (g *GossipLayer) heldCertificate(d types.CertificateDigest) *types.Certificate {
	if held := g.held.Load(); held != nil {
		return (*held)(d)
	}
	return nil
}

// committeeFor returns the committee of the epoch, or nil if this node holds
// none for it.
func (g *GossipLayer) committeeFor(epoch uint64) *types.Committee {
	cs := g.committees.Load()
	switch {
	case cs == nil:
		return nil
	case cs.current.Epoch == epoch:
		return cs.current
	case cs.previous != nil && cs.previous.Epoch == epoch:
		return cs.previous
	}
	return nil
}

// topicValidators maps each topic pattern to its validator.
func (g *GossipLayer) topicValidators() map[string]pubsub.ValidatorEx {
	return map[string]pubsub.ValidatorEx{
		TopicBatches:  g.validateBatch,
		TopicHeaders:  g.validateHeader,
		TopicVotes:    g.validateVote,
		TopicCerts:    g.validateCertificate,
		TopicCertSync: g.validateCertSync,
	}
}

// registerValidators registers every topic's validator with the router.
func (g *GossipLayer) registerValidators() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for pattern, validate := range g.topicValidators() {
		name := g.topics.TopicName(pattern)
		if err := g.pubsub.RegisterTopicValidator(name, validate); err != nil {
			g.unregisterValidatorsLocked()
			return fmt.Errorf("register validator for %s: %w", name, err)
		}
		g.validated = append(g.validated, name)
	}
	return nil
}

// unregisterValidators removes the validators this layer registered, so that
// a later GossipLayer of the same partition on the same router can register
// its own.
func (g *GossipLayer) unregisterValidators() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.unregisterValidatorsLocked()
}

func (g *GossipLayer) unregisterValidatorsLocked() {
	for _, name := range g.validated {
		_ = g.pubsub.UnregisterTopicValidator(name)
	}
	g.validated = nil
}

// validateBatch accepts a batch that decodes and is not empty. A batch names
// no author and carries no signature, so nothing ties it to a committee
// member; what a batch can cost a validator is bounded by the inbound queue
// and the batch store.
func (g *GossipLayer) validateBatch(_ context.Context, _ peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	batch, err := types.UnmarshalBatch(msg.Data)
	if err != nil {
		return g.refuse("batches", pubsub.ValidationReject, err.Error(), nil)
	}
	if len(batch.Transactions) == 0 {
		return g.refuse("batches", pubsub.ValidationReject, "empty batch", nil)
	}
	msg.ValidatorData = batch
	return pubsub.ValidationAccept
}

// validateHeader accepts a header whose author is a member of its epoch's
// committee and whose signature is the author's.
func (g *GossipLayer) validateHeader(_ context.Context, _ peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	header, err := types.UnmarshalHeader(msg.Data)
	if err != nil {
		return g.refuse("headers", pubsub.ValidationReject, err.Error(), nil)
	}
	c := g.committeeFor(header.Epoch)
	switch {
	case c == nil:
		return g.refuse("headers", pubsub.ValidationIgnore, "no committee for its epoch", header.Author)
	case !c.ContainsValidator(header.Author):
		return g.refuse("headers", pubsub.ValidationReject, "author not in committee", header.Author)
	}
	if err := header.Verify(); err != nil {
		return g.refuse("headers", pubsub.ValidationReject, err.Error(), header.Author)
	}
	msg.ValidatorData = header
	return pubsub.ValidationAccept
}

// validateVote accepts a vote whose author is a member of its epoch's
// committee and whose signature is the author's.
func (g *GossipLayer) validateVote(_ context.Context, _ peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	vote, err := types.UnmarshalVote(msg.Data)
	if err != nil {
		return g.refuse("votes", pubsub.ValidationReject, err.Error(), nil)
	}
	c := g.committeeFor(vote.Epoch)
	switch {
	case c == nil:
		return g.refuse("votes", pubsub.ValidationIgnore, "no committee for its epoch", vote.Author)
	case !c.ContainsValidator(vote.Author):
		return g.refuse("votes", pubsub.ValidationReject, "author not in committee", vote.Author)
	}
	if err := vote.Verify(); err != nil {
		return g.refuse("votes", pubsub.ValidationReject, err.Error(), vote.Author)
	}
	msg.ValidatorData = vote
	return pubsub.ValidationAccept
}

// validateCertificate accepts a certificate that verifies against its
// epoch's committee: the author is a member, the header is signed by it, and
// the votes are a quorum of distinct members' signatures. A copy, byte for
// byte, of a certificate the node holds is accepted on the verification
// that admitted it: the primary's duplicate rule (receiveCertificate),
// applied before the signatures so that a copy costs a lookup, not a check
// per signature. A stalled node re-shares every certificate of its round
// and the one before as new messages, to every peer (#4057).
func (g *GossipLayer) validateCertificate(_ context.Context, _ peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	cert, err := types.UnmarshalCertificate(msg.Data)
	if err != nil {
		// Info, not Debug: an undecodable certificate (e.g. wire-format skew
		// between builds) stalls consensus with no visible cause (#4054).
		slog.Info("Invalid certificate message",
			"error", err,
			"partition", g.partition)
		return g.refuse("certs", pubsub.ValidationReject, err.Error(), nil)
	}
	c := g.committeeFor(cert.Header.Epoch)
	switch {
	case c == nil:
		return g.refuse("certs", pubsub.ValidationIgnore, "no committee for its epoch", cert.Header.Author)
	case !c.ContainsValidator(cert.Header.Author):
		return g.refuse("certs", pubsub.ValidationReject, "author not in committee", cert.Header.Author)
	}
	if held := g.heldCertificate(cert.Digest()); held != nil && held.SameWire(cert) {
		msg.ValidatorData = cert
		return pubsub.ValidationAccept
	}
	if err := cert.Verify(c); err != nil {
		return g.refuse("certs", pubsub.ValidationReject, err.Error(), cert.Header.Author)
	}
	msg.ValidatorData = cert
	return pubsub.ValidationAccept
}

// validateCertSync accepts a request that decodes, and a response that
// decodes and whose every certificate is authored by a member of its epoch's
// committee. Requests carry no signature and come from joining nodes that
// are in no committee. A response's certificates are not verified here:
// a response carries up to MaxSyncCertificates of them and every node
// receives every response, so each certificate is verified by the receiver
// before it is used or counts as an answer.
func (g *GossipLayer) validateCertSync(_ context.Context, _ peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	if len(msg.Data) < 1 {
		return g.refuse("cert-sync", pubsub.ValidationReject, "empty message", nil)
	}
	switch msg.Data[0] {
	case 0x01:
		req, err := UnmarshalCertSyncRequest(msg.Data[1:])
		if err != nil {
			return g.refuse("cert-sync", pubsub.ValidationReject, err.Error(), nil)
		}
		msg.ValidatorData = req
		return pubsub.ValidationAccept

	case 0x02:
		resp, err := UnmarshalCertSyncResponse(msg.Data[1:])
		if err != nil {
			return g.refuse("cert-sync", pubsub.ValidationReject, err.Error(), nil)
		}
		for _, cert := range resp.Certificates {
			c := g.committeeFor(cert.Header.Epoch)
			switch {
			case c == nil:
				return g.refuse("cert-sync", pubsub.ValidationIgnore, "no committee for a certificate's epoch", cert.Header.Author)
			case !c.ContainsValidator(cert.Header.Author):
				return g.refuse("cert-sync", pubsub.ValidationReject, "certificate author not in committee", cert.Header.Author)
			}
		}
		msg.ValidatorData = resp
		return pubsub.ValidationAccept
	}
	return g.refuse("cert-sync", pubsub.ValidationReject, "unknown message type", nil)
}

// refuse counts and reports a message the validator does not pass on. The
// line is Debug: anyone can send messages that are refused.
func (g *GossipLayer) refuse(topic string, result pubsub.ValidationResult, reason string, author []byte) pubsub.ValidationResult {
	outcome := "reject"
	if result == pubsub.ValidationIgnore {
		outcome = "ignore"
	}
	metrics.GossipRefusedTotal.WithLabelValues(g.partition, topic, outcome).Inc()
	if debugEnabled() {
		slog.Debug("Consensus message refused",
			"partition", g.partition,
			"topic", topic,
			"outcome", outcome,
			"reason", reason,
			"author", fmt.Sprintf("%x", author))
	}
	return result
}
