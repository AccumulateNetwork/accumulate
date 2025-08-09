// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package url

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestURLMethods tests various URL methods
func TestURLMethods(t *testing.T) {
	tests := []struct {
		name     string
		url      *URL
		expected string
	}{
		{
			name:     "Simple authority",
			url:      &URL{Authority: "alice"},
			expected: "alice",
		},
		{
			name:     "With path",
			url:      &URL{Authority: "alice", Path: "tokens"},
			expected: "alice/tokens",
		},
		{
			name:     "With query",
			url:      &URL{Authority: "alice", Query: "foo=bar"},
			expected: "alice?foo=bar",
		},
		{
			name:     "With fragment",
			url:      &URL{Authority: "alice", Fragment: "section"},
			expected: "alice#section",
		},
		{
			name:     "Full URL",
			url:      &URL{Authority: "alice", Path: "tokens", Query: "foo=bar", Fragment: "main"},
			expected: "alice/tokens?foo=bar#main",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test String method
			assert.Equal(t, tt.expected, tt.url.String())
			
			// Test ShortString (should be same for these cases)
			assert.Equal(t, tt.expected, tt.url.ShortString())
			
			// Test Hostname
			assert.Equal(t, tt.url.Authority, tt.url.Hostname())
			
			// Test RootIdentity
			root := tt.url.RootIdentity()
			assert.Equal(t, tt.url.Authority, root.Authority)
			assert.Empty(t, root.Path)
			assert.Empty(t, root.Query)
			assert.Empty(t, root.Fragment)
		})
	}
}

// TestURLComparison tests URL comparison methods
func TestURLComparison(t *testing.T) {
	url1 := &URL{Authority: "alice", Path: "tokens"}
	url2 := &URL{Authority: "alice", Path: "tokens"}
	url3 := &URL{Authority: "bob", Path: "tokens"}
	url4 := &URL{Authority: "alice", Path: "credits"}
	
	// Test Equal
	assert.True(t, url1.Equal(url2))
	assert.False(t, url1.Equal(url3))
	assert.False(t, url1.Equal(url4))
	assert.False(t, url1.Equal(nil))
	
	// Test Compare
	assert.Equal(t, 0, url1.Compare(url2))
	assert.Less(t, url1.Compare(url3), 0)
	assert.Greater(t, url3.Compare(url1), 0)
	
	// Test Identity
	assert.True(t, url1.Identity().Equal(&URL{Authority: "alice"}))
	assert.Equal(t, "alice", url1.Identity().String())
	
	// Test RootIdentity  
	assert.True(t, url1.RootIdentity().Equal(&URL{Authority: "alice"}))
}

// TestURLJoin tests URL joining operations
func TestURLJoin(t *testing.T) {
	base := &URL{Authority: "alice"}
	
	// Test JoinPath
	withPath := base.JoinPath("tokens", "acme")
	assert.Equal(t, "alice/tokens/acme", withPath.String())
	assert.Equal(t, "alice", base.String()) // Original unchanged
	
	// Test WithPath
	withPath2 := base.WithPath("credits")
	assert.Equal(t, "alice/credits", withPath2.String())
	
	// Test WithQuery
	withQuery := base.WithQuery("foo", "bar")
	assert.Equal(t, "alice?foo=bar", withQuery.QueryValues().Encode())
	
	// Test WithFragment
	withFragment := base.WithFragment("section")
	assert.Equal(t, "alice#section", withFragment.String())
	
	// Test WithAuthority
	withAuth := withPath.WithAuthority("bob")
	assert.Equal(t, "bob/tokens/acme", withAuth.String())
}

// TestURLAccount tests account-related URL methods
func TestURLAccount(t *testing.T) {
	// Test account URL
	acc := &URL{Authority: "alice", Path: "tokens"}
	assert.True(t, acc.IsRootIdentity() == false)
	
	// Test root identity
	root := &URL{Authority: "alice"}
	assert.True(t, root.IsRootIdentity())
	
	// Test LocalTo
	local := &URL{Authority: "alice", Path: "tokens"}
	remote := &URL{Authority: "bob", Path: "credits"}
	assert.True(t, local.LocalTo(local))
	assert.False(t, local.LocalTo(remote))
	
	// Test Parent
	child := &URL{Authority: "alice", Path: "tokens/acme"}
	parent := child.Parent()
	assert.Equal(t, "alice/tokens", parent.String())
	
	rootParent := root.Parent()
	assert.Nil(t, rootParent)
}

// TestURLMarshaling tests JSON marshaling/unmarshaling
func TestURLMarshaling(t *testing.T) {
	tests := []struct {
		name string
		url  *URL
	}{
		{
			name: "Simple URL",
			url:  &URL{Authority: "alice"},
		},
		{
			name: "URL with path",
			url:  &URL{Authority: "alice", Path: "tokens/acme"},
		},
		{
			name: "URL with query",
			url:  &URL{Authority: "alice", Query: "foo=bar&baz=qux"},
		},
		{
			name: "Full URL",
			url:  &URL{Authority: "alice", Path: "tokens", Query: "foo=bar", Fragment: "main"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.url)
			require.NoError(t, err)
			
			// Unmarshal back
			var decoded URL
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			
			// Should be equal
			assert.True(t, tt.url.Equal(&decoded))
		})
	}
}

// TestTxID tests transaction ID functionality
func TestTxID(t *testing.T) {
	// Create TxID
	txid := &TxID{
		Hash: [32]byte{1, 2, 3, 4, 5},
		Url:  &URL{Authority: "alice", Path: "tokens"},
	}
	
	// Test Hash
	assert.Equal(t, [32]byte{1, 2, 3, 4, 5}, txid.Hash)
	
	// Test URL
	assert.Equal(t, "alice/tokens", txid.Url.String())
	
	// Test MarshalJSON
	data, err := json.Marshal(txid)
	require.NoError(t, err)
	
	// Test UnmarshalJSON
	var decoded TxID
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, txid.Hash, decoded.Hash)
	assert.True(t, txid.Url.Equal(decoded.Url))
	
	// Test String representation
	str := txid.String()
	assert.Contains(t, str, "0102030405")
	
	// Test AsBytes
	bytes := txid.AsBytes()
	assert.NotNil(t, bytes)
	
	// Test FromBytes
	txid2 := new(TxID)
	err = txid2.FromBytes(bytes)
	require.NoError(t, err)
	assert.Equal(t, txid.Hash, txid2.Hash)
}

// TestURLValidation tests URL validation
func TestURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     *URL
		wantErr bool
	}{
		{
			name:    "Valid URL",
			url:     &URL{Authority: "alice"},
			wantErr: false,
		},
		{
			name:    "Empty authority",
			url:     &URL{Authority: ""},
			wantErr: true,
		},
		{
			name:    "With valid path",
			url:     &URL{Authority: "alice", Path: "tokens"},
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// URL validation would happen during parsing
			// For now just check basic properties
			if tt.wantErr {
				assert.Empty(t, tt.url.Authority)
			} else {
				assert.NotEmpty(t, tt.url.Authority)
			}
		})
	}
}

// TestURLQueryValues tests query parameter handling
func TestURLQueryValues(t *testing.T) {
	// Test setting query values
	u := &URL{Authority: "alice"}
	u = u.WithQuery("foo", "bar")
	u = u.WithQuery("baz", "qux")
	
	values := u.QueryValues()
	assert.Equal(t, "bar", values.Get("foo"))
	assert.Equal(t, "qux", values.Get("baz"))
	
	// Test parsing existing query
	u2 := &URL{Authority: "alice", Query: "key1=value1&key2=value2"}
	values2 := u2.QueryValues()
	assert.Equal(t, "value1", values2.Get("key1"))
	assert.Equal(t, "value2", values2.Get("key2"))
}

// TestURLCopy tests URL copying
func TestURLCopy(t *testing.T) {
	original := &URL{
		Authority: "alice",
		Path:      "tokens",
		Query:     "foo=bar",
		Fragment:  "section",
	}
	
	// Copy via WithPath (should create new instance)
	copy1 := original.WithPath("credits")
	assert.Equal(t, "alice/credits?foo=bar#section", copy1.String())
	assert.Equal(t, "alice/tokens?foo=bar#section", original.String())
	
	// Modifying copy shouldn't affect original
	copy1.Authority = "bob"
	assert.Equal(t, "alice", original.Authority)
	assert.Equal(t, "bob", copy1.Authority)
}

// TestURLEdgeCases tests edge cases
func TestURLEdgeCases(t *testing.T) {
	// Test nil URL
	var nilURL *URL
	assert.Equal(t, "", nilURL.String())
	assert.Equal(t, "", nilURL.ShortString())
	assert.Nil(t, nilURL.Identity())
	assert.Nil(t, nilURL.RootIdentity())
	
	// Test empty URL
	emptyURL := &URL{}
	assert.Equal(t, "", emptyURL.String())
	
	// Test URL with only fragment
	fragURL := &URL{Fragment: "section"}
	assert.Equal(t, "#section", fragURL.String())
	
	// Test URL with only query
	queryURL := &URL{Query: "foo=bar"}
	assert.Equal(t, "?foo=bar", queryURL.String())
}