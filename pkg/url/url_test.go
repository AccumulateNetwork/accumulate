// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package url

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	cases := []struct {
		s      string
		expect interface{}
	}{
		{"foo", "acc://foo"},
		{"acc://foo", "acc://foo"},
		{"xxx://foo", wrongScheme("xxx://foo")},
		{"/foo", missingHost("/foo")},
		{"acc://:123", missingHost("acc://:123")},
		{"acc://foo?", "acc://foo"},
		{"acc://foo#", "acc://foo"},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			u, err := Parse(c.s)
			if _, ok := c.expect.(error); ok {
				require.Error(t, err)
				require.Equal(t, c.expect, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expect, u.String())
			}
		})
	}
}

func TestURL_Equal(t *testing.T) {
	cases := []struct {
		u, v *URL
		eq   bool
	}{
		{&URL{Authority: "foo"}, &URL{Authority: "FOO"}, true},
		{&URL{Authority: "foo"}, &URL{Authority: "foo", Query: "bar"}, false},
		{&URL{Authority: "foo"}, &URL{Authority: "foo", Fragment: "bar"}, false},
		{&URL{Authority: "foo"}, &URL{Authority: "foo", Path: "bar"}, false},
		{&URL{Authority: "foo"}, &URL{Authority: "foo", UserInfo: "bar"}, false},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			if c.eq {
				assert.Truef(t, c.u.Equal(c.v), "%v != %v", c.u, c.v)
			} else {
				assert.Falsef(t, c.u.Equal(c.v), "%v == %v", c.u, c.v)
			}
		})
	}
}

func TestURL_IdentityChain(t *testing.T) {
	cases := []struct {
		url    string
		expect string
	}{
		{"FOO", "foo"},
		{"foo/bar", "foo"},
		{"foo?bar", "foo"},
		{"foo#bar", "foo"},
		{"bar@foo", "foo"},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			u, err := Parse(c.url)
			require.NoError(t, err)
			h := sha256.Sum256([]byte(c.expect))
			require.Equal(t, h[:], u.IdentityAccountID())
		})
	}
}

func TestURL_ResourceChain(t *testing.T) {
	cases := []struct {
		url    string
		expect string
	}{
		{"FOO", "foo"},
		{"foo/bar", "foo/bar"},
		{"foo?bar", "foo"},
		{"foo#bar", "foo"},
		{"bar@foo", "foo"},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			u, err := Parse(c.url)
			require.NoError(t, err)
			h := sha256.Sum256([]byte(c.expect))
			require.Equal(t, h[:], u.AccountID())
		})
	}
}
