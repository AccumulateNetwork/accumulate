package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func (m *Executor) loadDirectoryMetadata(chainId []byte) (*protocol.DirectoryIndexMetadata, error) {
	b, err := m.DB.GetIndex(state.DirectoryIndex, chainId, "Metadata")
	if err != nil {
		return nil, err
	}

	md := new(protocol.DirectoryIndexMetadata)
	err = md.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	return md, nil
}

func (m *Executor) loadDirectoryEntry(chainId []byte, index uint64) (string, error) {
	b, err := m.DB.GetIndex(state.DirectoryIndex, chainId, index)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (m *Executor) mirrorADIs(urls ...*url.URL) (*protocol.SyntheticMirror, error) {
	mirror := new(protocol.SyntheticMirror)

	for _, u := range urls {
		chainId := u.ResourceChain()
		obj, _, err := m.dbTx.LoadChain(chainId)
		if err != nil {
			return nil, fmt.Errorf("failed to load %q: %v", u, err)
		}
		mirror.Objects = append(mirror.Objects, obj)

		md, err := m.loadDirectoryMetadata(chainId)
		if err != nil {
			return nil, fmt.Errorf("failed to load directory for %q: %v", u, err)
		}

		for i := uint64(0); i < md.Count; i++ {
			s, err := m.loadDirectoryEntry(chainId, i)
			if err != nil {
				return nil, fmt.Errorf("failed to load directory entry %d", i)
			}

			u, err := url.Parse(s)
			if err != nil {
				return nil, fmt.Errorf("invalid url %q: %v", s, err)
			}

			obj, _, err := m.dbTx.LoadChain(u.ResourceChain())
			if err != nil {
				return nil, fmt.Errorf("failed to load %q: %v", u, err)
			}
			mirror.Objects = append(mirror.Objects, obj)
		}
	}

	return mirror, nil
}
