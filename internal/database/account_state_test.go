package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAccountState verifies that adding an entry to an ADI's directory listing changes the account's BPT entry.
func TestAccountState(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	adiurl := url.MustParse("acc://testadi1.acme")
	bookurl := url.MustParse("acc://testadi1.acme/testbook1")
	testurl := url.MustParse("acc://testurl")
	a := batch.Account(adiurl)
	acc := &protocol.ADI{Url: adiurl, AccountAuth: protocol.AccountAuth{Authorities: []protocol.AuthorityEntry{protocol.AuthorityEntry{Url: bookurl}}}}
	err := a.PutState(acc)
	require.NoError(t, err)
	h1, err := a.hashState()
	require.NoError(t, err)
	err = a.Directory().Add(testurl)
	require.NoError(t, err)
	h2, err := a.hashState()
	require.NoError(t, err)
	require.NotEqual(t, h1.MerkleHash(), h2.MerkleHash())
}
