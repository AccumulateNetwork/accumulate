// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package internal

import (
	"fmt"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type MultiaddrFlag struct {
	Value *multiaddr.Multiaddr
}

func (m MultiaddrFlag) Type() string { return "multiaddr" }

func (m MultiaddrFlag) String() string {
	if *m.Value == nil {
		return "nil"
	}
	return (*m.Value).String()
}

func (m MultiaddrFlag) Set(s string) error {
	a, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		return err
	}
	*m.Value = a
	return nil
}

type MultiaddrSliceFlag []multiaddr.Multiaddr

func (m MultiaddrSliceFlag) Type() string { return "multiaddr-slice" }

func (m MultiaddrSliceFlag) String() string {
	var s []string
	for _, m := range m {
		s = append(s, m.String())
	}
	return strings.Join(s, ",")
}

func (m *MultiaddrSliceFlag) Set(s string) error {
	a, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		return err
	}
	*m = append(*m, a)
	return nil
}

type UrlFlag struct {
	V **url.URL
}

func (f UrlFlag) Type() string   { return "acc-url" }
func (f UrlFlag) String() string { return fmt.Sprint(*f.V) }
func (f UrlFlag) Set(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	*f.V = u
	return nil
}
