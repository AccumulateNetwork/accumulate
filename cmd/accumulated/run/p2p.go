// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
)

func (p *P2P) start(inst *Instance) error {
	sk, err := getPrivateKey(p.Key, inst)
	if err != nil {
		return err
	}

	setDefaultPtr(&p.PeerDB, "")
	node, err := p2p.New(p2p.Options{
		Key:               sk,
		Network:           inst.config.Network,
		Listen:            p.Listen,
		BootstrapPeers:    p.BootstrapPeers,
		PeerDatabase:      *p.PeerDB,
		EnablePeerTracker: p.EnablePeerTracking,
	})
	if err != nil {
		return err
	}
	inst.p2p = node

	slog.InfoContext(inst.context, "We are", "node-id", node.ID(), "instance-id", inst.id, "module", "run")

	inst.cleanup("p2p node", func(context.Context) error {
		err := node.Close()
		if err != nil {
			return err
		}
		slog.InfoContext(inst.context, "Stopped", "id", node.ID(), "module", "run")
		return nil
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
