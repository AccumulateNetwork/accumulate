// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package core

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type globalSigner struct {
	Partition string
	*protocol.NetworkDefinition
}

func ValidatorSigner(g *GlobalValues, partition string) *globalSigner {
	s := new(globalSigner)
	s.Partition = partition
	s.NetworkDefinition = g.Network
	return s
}

func (g *globalSigner) GetUrl() *url.URL   { return protocol.DnUrl().JoinPath(protocol.Network) }
func (g *globalSigner) GetVersion() uint64 { return g.Version }

func (g *globalSigner) GetSignatureThreshold() uint64 { return 1 }

func (g *globalSigner) EntryByDelegate(owner *url.URL) (int, protocol.KeyEntry, bool) {
	return 0, nil, false
}

func (g *globalSigner) EntryByKey(key []byte) (int, protocol.KeyEntry, bool) {
	keyHash := sha256.Sum256(key)
	return g.EntryByKeyHash(keyHash[:])
}

func (g *globalSigner) EntryByKeyHash(keyHash []byte) (int, protocol.KeyEntry, bool) {
	i, v, ok := g.ValidatorByHash(keyHash)
	if !ok || !v.IsActiveOn(g.Partition) {
		return 0, nil, false
	}
	return i, noopEntry{}, true
}

type noopEntry struct{}

func (noopEntry) GetLastUsedOn() uint64 { return 0 }
func (noopEntry) SetLastUsedOn(uint64)  {}
