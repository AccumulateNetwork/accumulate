package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SignatureSet interface {
	record
	Get() ([]*SignatureEntry, error)
	Add(protocol.Signature) error
	getVersion() (uint64, error)
	putVersion(uint64) error
	putEntries(v []*SignatureEntry) error
}

type SystemSignatureSet struct {
	Set[*SignatureEntry]
}

func newSystemSignatureSet(store recordStore, key recordKey, _, labelfmt string) *SystemSignatureSet {
	s := new(SystemSignatureSet)
	new := func() (v *SignatureEntry) { return new(SignatureEntry) }
	cmp := func(u, v *SignatureEntry) int { return u.Compare(v) }
	s.Set = *newSet(store, key, labelfmt, newSlice(new), cmp)
	return s
}

func (s *SystemSignatureSet) Add(signature protocol.Signature) error {
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
	set     *Set[*SignatureEntry]
	version *Wrapped[uint64]
	signer  protocol.Signer
	err     error
}

func newVersionedSignatureSet(cs *ChangeSet, store recordStore, key recordKey, signerUrl *url.URL) *VersionedSignatureSet {
	s := new(VersionedSignatureSet)
	key = key.Append("Signatures", signerUrl)
	new := func() (v *SignatureEntry) { return new(SignatureEntry) }
	cmp := func(u, v *SignatureEntry) int { return u.Compare(v) }
	s.set = newSet(store, key, "transaction %[2]x signatures %[4]v", newSlice(new), cmp)
	s.version = newWrapped(store, key.Append("Version"), "transaction %[2]x signatures %[4]v version", true, newWrapper(uintWrapper))

	lastVersion, err := s.version.Get()
	if err != nil {
		return &VersionedSignatureSet{err: errors.Wrap(errors.StatusUnknown, err)}
	}

	err = cs.Account(signerUrl).State().GetAs(&s.signer)
	if err != nil {
		return &VersionedSignatureSet{err: errors.Wrap(errors.StatusUnknown, err)}
	}

	if lastVersion > s.signer.GetVersion() {
		return &VersionedSignatureSet{err: errors.Format(errors.StatusInternalError, "last version > signer version")}
	}

	return s
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

	v := new(SignatureEntry)
	v.Type = signature.Type()
	v.SignatureHash = *(*[32]byte)(signature.Hash())

	switch sig := signature.(type) {
	case protocol.KeySignature:
		i, _, ok := s.signer.EntryByKeyHash(sig.GetPublicKeyHash())
		if !ok {
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
	return s.set.Put(v)
}

func (s *VersionedSignatureSet) getVersion() (uint64, error) {
	return s.version.Get()
}

func (s *VersionedSignatureSet) putVersion(v uint64) error {
	return s.version.Put(v)
}

func (s *VersionedSignatureSet) resolve(key recordKey) (record, recordKey, error) {
	if len(key) == 1 {
		if k, ok := key[0].(string); ok && k == "Version" {
			return s.version, nil, nil
		}
	}

	return s.set.resolve(key)
}

func (s *VersionedSignatureSet) isDirty() bool {
	return s.version.isDirty() || s.set.isDirty()
}

func (s *VersionedSignatureSet) commit() error {
	err := s.version.commit()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = s.set.commit()
	return errors.Wrap(errors.StatusUnknown, err)
}
