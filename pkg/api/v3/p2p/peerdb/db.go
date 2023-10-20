// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerdb

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

func New() *DB {
	return &DB{Peers: new(AtomicSlice[*PeerStatus, PeerStatus])}
}

func LoadFile(file string) (*DB, error) {
	f, err := os.Open(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return New(), nil
		}
		return nil, err
	}
	defer f.Close()
	db := New()
	return db, db.Load(f)
}

func (db *DB) Load(rd io.Reader) error {
	dec := json.NewDecoder(rd)
	dec.DisallowUnknownFields()
	err := dec.Decode(db)
	return err
}

func (db *DB) Store(wr io.Writer) error {
	enc := json.NewEncoder(wr)
	enc.SetIndent("", "  ")
	return enc.Encode(db)
}

func (db *DB) StoreFile(file string) error {
	f, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return db.Store(f)
}

func (db *DB) Peer(id peer.ID) *PeerStatus {
	return db.Peers.Insert(&PeerStatus{
		ID:        id,
		Networks:  &AtomicSlice[*PeerNetworkStatus, PeerNetworkStatus]{},
		Addresses: &AtomicSlice[*PeerAddressStatus, PeerAddressStatus]{},
	})
}

func (p *PeerStatus) Address(addr multiaddr.Multiaddr) *PeerAddressStatus {
	return p.Addresses.Insert(&PeerAddressStatus{
		Address: addr,
	})
}

func (p *PeerStatus) Network(name string) *PeerNetworkStatus {
	return p.Networks.Insert(&PeerNetworkStatus{
		Name:     name,
		Services: &AtomicSlice[*PeerServiceStatus, PeerServiceStatus]{},
	})
}

func (p *PeerNetworkStatus) Service(addr *api.ServiceAddress) *PeerServiceStatus {
	return p.Services.Insert(&PeerServiceStatus{
		Address: addr,
	})
}
