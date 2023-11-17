// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

func (p *P2P) start(inst *Instance) error {
	if p == nil {
		p = new(P2P)
	}
	if p.Key == nil {
		p.Key = new(TransientPrivateKey)
	}

	addr := p.Key.get()
	if addr.GetType() != protocol.SignatureTypeED25519 {
		return errors.BadRequest.WithFormat("key type %v not supported", addr.GetType())
	}
	sk, ok := addr.GetPrivateKey()
	if !ok {
		return errors.BadRequest.WithFormat("missing private key")
	}

	node, err := p2p.New(p2p.Options{
		Key:               sk,
		Network:           p.Network,
		Listen:            p.Listen,
		BootstrapPeers:    p.BootstrapPeers,
		PeerDatabase:      p.PeerDB,
		EnablePeerTracker: p.EnablePeerTracking,
	})
	if err != nil {
		return err
	}
	inst.p2p = node

	slog.InfoCtx(inst.context, "We are", "id", node.ID(), "module", "p2p")

	inst.cleanup(func() {
		err := node.Close()
		if err != nil {
			slog.ErrorCtx(inst.context, "Error while stopping node", "module", "p2p", "id", node.ID(), "error", err)
		} else {
			slog.InfoCtx(inst.context, "Stopped", "id", node.ID(), "module", "p2p")
		}
	})
	return nil
}
