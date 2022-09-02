package core

import (
	"crypto/sha256"
	"math"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type globalValueMemos struct {
	threshold map[string]uint64
	active    map[string]int
}

func (g *GlobalValues) memoizeValidators() {
	if g.memoize.active != nil {
		return
	}

	active := make(map[string]int, len(g.Network.Partitions))
	for _, v := range g.Network.Validators {
		for _, p := range v.Partitions {
			if p.Active {
				active[strings.ToLower(p.ID)]++
			}
		}
	}

	threshold := make(map[string]uint64, len(g.Network.Partitions))
	for partition, active := range active {
		threshold[partition] = g.Globals.ValidatorAcceptThreshold.Threshold(active)
	}

	g.memoize.active = active
	g.memoize.threshold = threshold
}

func (g *GlobalValues) ValidatorThreshold(partition string) uint64 {
	g.memoizeValidators()
	v, ok := g.memoize.threshold[strings.ToLower(partition)]
	if !ok {
		return math.MaxUint64
	}
	return v
}

type globalSigner struct {
	Partition string
	*protocol.NetworkDefinition
}

func (g *GlobalValues) AsSigner(partition string) *globalSigner {
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
