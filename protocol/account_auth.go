// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (a *AccountAuth) KeyBook() *url.URL {
	if len(a.Authorities) >= 1 {
		return a.Authorities[0].Url
	}
	return nil
}

func (a *AccountAuth) AllAuthoritiesAreDisabled() bool {
	for _, e := range a.Authorities {
		if !e.Disabled {
			return false
		}
	}
	return true
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
