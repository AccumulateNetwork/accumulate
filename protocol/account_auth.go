package protocol

import (
	"sort"

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

type listSearchResult int

const (
	listSearchFound listSearchResult = iota
	listSearchAppend
	listSearchInsert
)

func (a *AccountAuth) search(entry *url.URL) (int, listSearchResult) {
	// Find the matching entry
	i := sort.Search(len(a.Authorities), func(i int) bool {
		return a.Authorities[i].Url.Compare(entry) >= 0
	})

	// Entry belongs past the end of the list
	if i >= len(a.Authorities) {
		return i, listSearchAppend
	}

	// A matching entry exists
	if a.Authorities[i].Url.Equal(entry) {
		return i, listSearchFound
	}

	return i, listSearchInsert
}

func (a *AccountAuth) GetAuthority(entry *url.URL) (*AuthorityEntry, bool) {
	i, r := a.search(entry)
	if r != listSearchFound {
		return nil, false
	}

	return &a.Authorities[i], true
}

func (a *AccountAuth) AddAuthority(entry *url.URL) *AuthorityEntry {
	i, r := a.search(entry)
	switch r {
	case listSearchFound:
		// Ok

	case listSearchAppend:
		a.Authorities = append(a.Authorities, AuthorityEntry{Url: entry})

	case listSearchInsert:
		a.Authorities = append(a.Authorities, AuthorityEntry{})
		copy(a.Authorities[i+1:], a.Authorities[i:])
		a.Authorities[i] = AuthorityEntry{Url: entry}

	default:
		panic("unreachable")
	}
	return &a.Authorities[i]
}

func (a *AccountAuth) RemoveAuthority(entry *url.URL) bool {
	i, r := a.search(entry)
	if r != listSearchFound {
		return false
	}

	a.Authorities = append(a.Authorities[:i], a.Authorities[i+1:]...)
	return true
}
