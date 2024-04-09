// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func ExampleQueryAccount() {
	client := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint("mainnet", "v3"))

	resp, err := client.Query(context.Background(), url.MustParse("accumulate.acme"), &api.DefaultQuery{})
	if err != nil {
		panic(err)
	}

	acct, ok := resp.(*api.AccountRecord)
	if !ok {
		panic(fmt.Errorf("want %T, got %T", new(api.AccountRecord), resp))
	}

	// Output:
	// {"type":"identity","url":"acc://accumulate.acme","authorities":[{"url":"acc://accumulate.acme/book"}]}
	b, err := json.Marshal(acct.Account)
	fmt.Println(string(b))
}
