// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"golang.org/x/exp/slog"
)

func (p *P2P) start(inst *Instance) error {
	if p == nil {
		p = new(P2P)
	}
	if p.Key == nil {
		p.Key = new(TransientPrivateKey)
	}

	sk, err := getPrivateKey(p.Key, inst)
	if err != nil {
		return err
	}

	node, err := p2p.New(p2p.Options{
		Key:               sk,
		Network:           inst.network,
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
