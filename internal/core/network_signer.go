package core

import (
	"crypto/sha256"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type globalValueMemos struct {
	active map[string]int
}

func (g *GlobalValues) countActive(partition string) int {
	partition = strings.ToLower(partition)
	if g.memoize.active != nil {
		return g.memoize.active[partition]
	}

	active := make(map[string]int, len(g.Network.Partitions))
	for _, v := range g.Network.Validators {
		for _, p := range v.Partitions {
			if p.Active {
				active[strings.ToLower(p.ID)]++
			}
		}
	}

	g.memoize.active = active
	return active[partition]
}

type globalSigner struct {
	Partition string
	Threshold uint64
	*protocol.NetworkDefinition
}

func (g *GlobalValues) AsSigner(partition string) *globalSigner {
	s := new(globalSigner)
	s.Partition = partition
	s.Threshold = g.Globals.OperatorAcceptThreshold.Threshold(g.countActive(partition))
	s.NetworkDefinition = g.Network
	return s
}

func (g *globalSigner) GetUrl() *url.URL   { return protocol.DnUrl().JoinPath(protocol.Network) }
func (g *globalSigner) GetVersion() uint64 { return g.Version }

// func (g *globalSigner) GetSignatureThreshold() uint64 { return g.Threshold }
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
