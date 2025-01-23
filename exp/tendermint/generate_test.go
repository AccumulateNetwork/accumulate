// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cometbft/cometbft/rpc/client"
)

func TestGenerateClientMethods(t *testing.T) {
	typ := reflect.TypeOf(new(client.Client)).Elem()
	for i, n := 0, typ.NumMethod(); i < n; i++ {
		m := typ.Method(i)
		if m.PkgPath != "" {
			continue
		}

		var in, out, args []string
		for i, n := 0, m.Type.NumIn(); i < n; i++ {
			arg := fmt.Sprintf("a%d", i)
			in = append(in, fmt.Sprintf("%s %s", arg, m.Type.In(i)))
			args = append(args, arg)
		}
		for i, n := 0, m.Type.NumOut(); i < n; i++ {
			out = append(out, m.Type.Out(i).String())
		}

		var ret string
		if len(out) > 0 {
			ret = "return "
		}

		fmt.Printf(
			`
			func (d *DeferredClient) %s(%s) (%s) {
				%s d.get().%s(%s)
			}
			`,
			m.Name,
			strings.Join(in, ", "),
			strings.Join(out, ", "),
			ret,
			m.Name,
			strings.Join(args, ", "))
	}
}
