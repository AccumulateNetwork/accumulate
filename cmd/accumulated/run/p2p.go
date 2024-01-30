// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"strconv"

	dht "github.com/libp2p/go-libp2p-kad-dht"
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

	setDefaultPtr(&p.PeerDB, "")

	node, err := p2p.New(p2p.Options{
		Key:               sk,
		Network:           inst.network,
		Listen:            p.Listen,
		BootstrapPeers:    p.BootstrapPeers,
		PeerDatabase:      *p.PeerDB,
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

type DhtMode dht.ModeOpt

func (d DhtMode) String() string {
	switch dht.ModeOpt(d) {
	case dht.ModeAuto:
		return "auto"
	case dht.ModeClient:
		return "client"
	case dht.ModeServer:
		return "server"
	case dht.ModeAutoServer:
		return "auto-server"
	}
	return strconv.FormatInt(int64(d), 10)
}

func (d DhtMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *DhtMode) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	switch s {
	case "auto":
		*d = DhtMode(dht.ModeAuto)
		return nil
	case "client":
		*d = DhtMode(dht.ModeClient)
		return nil
	case "server":
		*d = DhtMode(dht.ModeServer)
		return nil
	case "auto-server":
		*d = DhtMode(dht.ModeAutoServer)
		return nil
	}

	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*d = DhtMode(dht.ModeOpt(i))
	return nil
}
