package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func (a *AccountAuth) KeyBook() *url.URL {
	if len(a.Authorities) >= 1 {
		return a.Authorities[0].Url
	}
	return nil
}

func (a *AccountAuth) AuthDisabled() bool {
	for _, e := range a.Authorities {
		if e.Disabled {
			return true
		}
	}
	return false
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

func (a *AccountAuth) AddAuthority(entry *url.URL) *AuthorityEntry {
	ptr, new := sortutil.BinaryInsert(&a.Authorities, func(e AuthorityEntry) int { return e.Url.Compare(entry) })
	if new {
		*ptr = AuthorityEntry{Url: entry}
	}
	return ptr
}

func (a *AccountAuth) RemoveAuthority(entry *url.URL) bool {
	i, found := sortutil.Search(a.Authorities, func(e AuthorityEntry) int { return e.Url.Compare(entry) })
	if !found {
		return false
	}

	a.Authorities = append(a.Authorities[:i], a.Authorities[i+1:]...)
	return true
}
