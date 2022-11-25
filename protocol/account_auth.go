// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"io"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type AccountAuthRule interface {
	encoding.UnionValue
	Type() AccountAuthRuleType
	GetID() uint64
	MustAccept(txn *Transaction) bool
}

type AccountAuthRuleType uint64

func (defaultAuthRules) GetID() uint64   { return DefaultAuthRulesID }
func (disabledAuthRules) GetID() uint64  { return DisabledAuthRulesID }
func (r *IncludeSpecific) GetID() uint64 { return r.ID }
func (r *ExcludeSpecific) GetID() uint64 { return r.ID }

func (a *AccountAuth) KeyBook() *url.URL {
	if len(a.Authorities) >= 1 {
		return a.Authorities[0].Url
	}
	return nil
}

func (a *AccountAuth) ManagerKeyBook() *url.URL {
	if len(a.Authorities) >= 2 {
		return a.Authorities[1].Url
	}
	return nil
}

func (a *AccountAuth) GetAuthority(entry *url.URL) (*AuthorityEntry, bool) {
	i, found := sortutil.Search(a.Authorities, func(e AuthorityEntry) int { return e.Url.Compare(entry) })
	if !found {
		return nil, false
	}

	return &a.Authorities[i], true
}

func (a *AccountAuth) AddAuthority(entry *url.URL) (*AuthorityEntry, bool) {
	ptr, new := sortutil.BinaryInsert(&a.Authorities, func(e AuthorityEntry) int { return e.Url.Compare(entry) })
	if new {
		*ptr = AuthorityEntry{Url: entry}
	}
	return ptr, new
}

func (a *AccountAuth) RemoveAuthority(entry *url.URL) bool {
	i, found := sortutil.Search(a.Authorities, func(e AuthorityEntry) int { return e.Url.Compare(entry) })
	if !found {
		return false
	}

	a.Authorities = append(a.Authorities[:i], a.Authorities[i+1:]...)
	return true
}

const DefaultAuthRulesID = 0
const DisabledAuthRulesID = 1

type defaultAuthRules struct{ fixedRules }
type disabledAuthRules struct{ fixedRules }

func (defaultAuthRules) Type() AccountAuthRuleType  { return AccountAuthRuleTypeDefault }
func (disabledAuthRules) Type() AccountAuthRuleType { return AccountAuthRuleTypeDisabled }

func (defaultAuthRules) CopyAsInterface() interface{}  { return defaultAuthRules{} }
func (disabledAuthRules) CopyAsInterface() interface{} { return disabledAuthRules{} }

func (defaultAuthRules) MustAccept(txn *Transaction) bool { return true }

func (disabledAuthRules) MustAccept(txn *Transaction) bool {
	return txn.Body.Type() == TransactionTypeUpdateAccountAuth
}

type fixedRules struct{}

func (fixedRules) MarshalBinary() (data []byte, err error)    { return nil, errors.NotAllowed }
func (fixedRules) UnmarshalBinary(data []byte) error          { return errors.NotAllowed }
func (fixedRules) UnmarshalBinaryFrom(io.Reader) error        { return errors.NotAllowed }
func (fixedRules) UnmarshalFieldsFrom(*encoding.Reader) error { return errors.NotAllowed }

func (a *AuthorityEntry) Disabled() bool { return a.RuleID == disabledAuthRules{}.GetID() }

func (a *AccountAuth) GetRule(id uint64) (AccountAuthRule, bool) {
	if id == DefaultAuthRulesID {
		return defaultAuthRules{}, true
	}
	if id == DisabledAuthRulesID {
		return disabledAuthRules{}, true
	}

	i, ok := sortutil.Search(a.Rules, func(rs AccountAuthRule) int { return int(rs.GetID() - id) })
	if ok {
		return a.Rules[i], true
	}
	return nil, false
}

func (r *IncludeSpecific) MustAccept(txn *Transaction) bool {
	bit, ok := txn.Body.Type().AllowedTransactionBit()
	return ok && r.Transactions.IsSet(bit)
}

func (r *ExcludeSpecific) MustAccept(txn *Transaction) bool {
	bit, ok := txn.Body.Type().AllowedTransactionBit()
	return !ok || !r.Transactions.IsSet(bit)
}
