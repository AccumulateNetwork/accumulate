// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	sdktest "gitlab.com/accumulatenetwork/accumulate/test/sdk"
)

const defaultSdkTestData = "../../.testdata/sdk.json"

var sdkTestData = flag.String("sdk-test-data", defaultSdkTestData, "SDK test data")

func TestSDK(t *testing.T) {
	ts, err := sdktest.Load(*sdkTestData)
	if err != nil && errors.Is(err, fs.ErrNotExist) && *sdkTestData == defaultSdkTestData {
		t.Skip("Test data has not been created")
	}
	require.NoError(t, err)

	// For the Unmarshal tests, we're JSON marshalling and comparing the result.
	// It doesn't actually matter if the JSON marshalling is identical, but Go's
	// JSON marshalling is deterministic and comparing the marshalled JSON is
	// easier than comparing structs and less potentially error prone than using
	// Equal.

	t.Run("Transaction", func(t *testing.T) {
		for _, tcg := range ts.Transactions {
			t.Run(tcg.Name, func(t *testing.T) {
				for i, tc := range tcg.Cases {
					t.Run(fmt.Sprintf("Case %d", i+1), func(t *testing.T) {
						t.Run("Marshal", func(t *testing.T) {
							// Unmarshal the envelope from the TC
							env := new(messaging.Envelope)
							require.NoError(t, json.Unmarshal(tc.JSON, env))
							flattenRawJson(t, reflect.ValueOf(env))

							// TEST Binary marshal the envelope
							bin, err := env.MarshalBinary()
							require.NoError(t, err)

							// Compare the result to the TC
							require.Equal(t, tc.Binary, bin)
						})

						t.Run("Unmarshal", func(t *testing.T) {
							// TEST Binary unmarshal the envelope from the TC
							env := new(messaging.Envelope)
							require.NoError(t, env.UnmarshalBinary(tc.Binary))

							// Marshal the envelope
							json, err := json.Marshal(env)
							require.NoError(t, err)

							// Compare the result to the TC
							require.Equal(t, tokenize(t, tc.JSON), tokenize(t, json))
						})
					})
				}
			})
		}
	})

	t.Run("Account", func(t *testing.T) {
		for _, tcg := range ts.Accounts {
			t.Run(tcg.Name, func(t *testing.T) {
				for i, tc := range tcg.Cases {
					t.Run(fmt.Sprintf("Case %d", i+1), func(t *testing.T) {
						t.Run("Marshal", func(t *testing.T) {
							// Unmarshal the account from the TC
							acnt, err := protocol.UnmarshalAccountJSON(tc.JSON)
							require.NoError(t, err)
							flattenRawJson(t, reflect.ValueOf(acnt))

							// TEST Binary marshal the account
							bin, err := acnt.MarshalBinary()
							require.NoError(t, err)

							// Compare the result to the TC
							require.Equal(t, tc.Binary, bin)
						})

						t.Run("Unmarshal", func(t *testing.T) {
							// TEST Binary unmarshal the account from the TC
							acnt, err := protocol.UnmarshalAccount(tc.Binary)
							require.NoError(t, err)

							// Marshal the account
							json, err := json.Marshal(acnt)
							require.NoError(t, err)

							// Compare the result to the TC
							require.Equal(t, tokenize(t, tc.JSON), tokenize(t, json))
						})
					})
				}
			})
		}
	})
}

func tokenize(t *testing.T, in []byte) []json.Token {
	var tokens []json.Token
	dec := json.NewDecoder(bytes.NewReader(in))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return tokens
		}
		require.NoError(t, err)
		tokens = append(tokens, tok)
	}
}

func flattenRawJson(t *testing.T, v reflect.Value) {
	switch v.Kind() {
	case reflect.Ptr:
		flattenRawJson(t, v.Elem())
	case reflect.Struct:
		typ := v.Type()
		for i, nf := 0, v.NumField(); i < nf; i++ {
			if !typ.Field(i).IsExported() {
				continue
			}
			flattenRawJson(t, v.Field(i))
		}
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return
		}

		switch vv := v.Interface().(type) {
		case json.RawMessage:
			var u interface{}
			require.NoError(t, json.Unmarshal(vv, &u))
			vv, err := json.Marshal(u)
			require.NoError(t, err)
			v.Set(reflect.ValueOf(vv))
		default:
			if v.Type().Elem().Kind() == reflect.Uint8 {
				return
			}
			for i, n := 0, v.Len(); i < n; i++ {
				flattenRawJson(t, v.Index(i))
			}
		}
	case reflect.Interface:
		raw, ok := v.Interface().(json.RawMessage)
		if !ok {
			flattenRawJson(t, v.Elem())
			return
		}
		if len(raw) == 0 {
			return
		}
		var u interface{}
		require.NoError(t, json.Unmarshal(raw, &u))
		raw, err := json.Marshal(u)
		require.NoError(t, err)
		v.Set(reflect.ValueOf(raw))
	}
}
