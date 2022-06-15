package database

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SignatureSet interface {
	record.Record
	Get() ([]*SignatureEntry, error)
	Add(protocol.Signature) error
	getVersion() (uint64, error)
	putVersion(uint64) error
	putEntries(v []*SignatureEntry) error
}

type SystemSignatureSet struct {
	container *Transaction
	record.Set[*SignatureEntry]
}

func newSystemSignatureSet(container *Transaction, logger log.Logger, store record.Store, key record.Key, _, labelfmt string) *SystemSignatureSet {
	s := new(SystemSignatureSet)
	s.container = container
	new := func() (v *SignatureEntry) { return new(SignatureEntry) }
	cmp := func(u, v *SignatureEntry) int { return u.Compare(v) }
	s.Set = *record.NewSet(logger, store, key, labelfmt, record.NewSlice(new), cmp)
	return s
}

func (s *SystemSignatureSet) Add(signature protocol.Signature) error {
	// Update the signer set
	err := s.container.addSigner(protocol.AcmeUrl())
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	v := new(SignatureEntry)
	v.Type = signature.Type()
	v.SignatureHash = *(*[32]byte)(signature.Hash())
	return s.Set.Add(v)
}

func (s *SystemSignatureSet) putEntries(v []*SignatureEntry) error {
	return s.Set.Put(v)
}

func (s *SystemSignatureSet) getVersion() (uint64, error) {
	return 0, nil
}

func (s *SystemSignatureSet) putVersion(uint64) error {
	return nil
}

type VersionedSignatureSet struct {
	container *Transaction
	set       *record.Set[*SignatureEntry]
	version   *record.Wrapped[uint64]
	signer    protocol.Signer
	err       error
}

func newVersionedSignatureSet(container *Transaction, store record.Store, key record.Key, namefmt string) *VersionedSignatureSet {
	s := new(VersionedSignatureSet)
	s.container = container
	new := func() (v *SignatureEntry) { return new(SignatureEntry) }
	cmp := func(u, v *SignatureEntry) int { return u.Compare(v) }
	s.set = record.NewSet(container.logger.L, store, key, namefmt, record.NewSlice(new), cmp)
	s.version = record.NewWrapped(container.logger.L, store, key.Append("Version"), namefmt+" version", true, record.NewWrapper(record.UintWrapper))

	lastVersion, err := s.version.Get()
	if err != nil {
		return &VersionedSignatureSet{err: errors.Wrap(errors.StatusUnknown, err)}
	}

	err = container.container.Account(s.getSignerUrl()).State().GetAs(&s.signer)
	if err != nil {
		return &VersionedSignatureSet{err: errors.Wrap(errors.StatusUnknown, err)}
	}

	if lastVersion > s.signer.GetVersion() {
		return &VersionedSignatureSet{err: errors.Format(errors.StatusInternalError, "last version > signer version")}
	}

	return s
}

func (s *VersionedSignatureSet) getSignerUrl() *url.URL {
	return s.set.Key(3).(*url.URL)
}

func (s *VersionedSignatureSet) Get() ([]*SignatureEntry, error) {
	if s.err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, s.err)
	}
	return s.set.Get()
}

func (s *VersionedSignatureSet) Add(signature protocol.Signature) error {
	if s.err != nil {
		return errors.Wrap(errors.StatusUnknown, s.err)
	}

	if !s.getSignerUrl().Equal(signature.GetSigner()) {
		return errors.Format(errors.StatusInternalError, "cannot add signature by %v to signature set %v", signature.GetSigner(), s.getSignerUrl())
	}

	// Update the signer set
	err := s.container.addSigner(signature.GetSigner())
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	v := new(SignatureEntry)
	v.Type = signature.Type()
	v.SignatureHash = *(*[32]byte)(signature.Hash())

	switch sig := signature.(type) {
	case protocol.KeySignature:
		i, _, ok := s.signer.EntryByKeyHash(sig.GetPublicKeyHash())
		if !ok {
			s.signer.EntryByKeyHash(sig.GetPublicKeyHash())
			return errors.Format(errors.StatusInternalError, "key hash %X does not belong to signer", sig.GetPublicKeyHash()[:8])
		}
		v.KeyEntryIndex = uint64(i)

	case *protocol.DelegatedSignature:
		i, _, ok := s.signer.EntryByDelegate(sig.Delegate)
		if !ok {
			return errors.Format(errors.StatusInternalError, "delegate %v does not belong to signer", sig.Delegate)
		}
		v.KeyEntryIndex = uint64(i)

	default:
		return errors.Format(errors.StatusInternalError, "invalid signature type %T", signature)
	}

	// Get the signature set version
	lastVersion, err := s.version.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// If the signer version matches, ok
	if lastVersion == s.signer.GetVersion() {
		return s.set.Add(v)
	}

	// Update the signature set version
	err = s.version.Put(s.signer.GetVersion())
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Remove any previous signatures
	err = s.set.Put([]*SignatureEntry{v})
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *VersionedSignatureSet) putEntries(v []*SignatureEntry) error {
	if s.err != nil {
		return errors.Wrap(errors.StatusUnknown, s.err)
	}
	return s.set.Put(v)
}

func (s *VersionedSignatureSet) getVersion() (uint64, error) {
	if s.err != nil {
		return 0, errors.Wrap(errors.StatusUnknown, s.err)
	}
	return s.version.Get()
}

func (s *VersionedSignatureSet) putVersion(v uint64) error {
	if s.err != nil {
		return errors.Wrap(errors.StatusUnknown, s.err)
	}
	return s.version.Put(v)
}

func (s *VersionedSignatureSet) Resolve(key record.Key) (record.Record, record.Key, error) {
	if s.err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, s.err)
	}
	if len(key) == 1 {
		if k, ok := key[0].(string); ok && k == "Version" {
			return s.version, nil, nil
		}
	}

	return s.set.Resolve(key)
}

func (s *VersionedSignatureSet) IsDirty() bool {
	if s.err != nil {
		return false
	}
	return s.version.IsDirty() || s.set.IsDirty()
}

func (s *VersionedSignatureSet) Commit() error {
	if s.err != nil {
		return errors.Wrap(errors.StatusUnknown, s.err)
	}
	err := s.version.Commit()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = s.set.Commit()
	return errors.Wrap(errors.StatusUnknown, err)
}
