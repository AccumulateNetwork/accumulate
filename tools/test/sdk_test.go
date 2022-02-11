package test

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/testdata"
)

const defaultSdkTestData = "../../.testdata/sdk.json"

var sdkTestData = flag.String("sdk-test-data", defaultSdkTestData, "SDK test data")

func TestSDK(t *testing.T) {
	ts, err := testdata.Load(*sdkTestData)
	if err != nil && errors.Is(err, fs.ErrNotExist) && *sdkTestData == defaultSdkTestData {
		t.Skip("Test data has not been created")
	}

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
							env := new(protocol.Envelope)
							require.NoError(t, json.Unmarshal(tc.JSON, env))

							// TEST Binary marshal the envelope
							bin, err := env.MarshalBinary()
							require.NoError(t, err)

							// Compare the result to the TC
							require.Equal(t, tc.Binary, bin)
						})

						t.Run("Unmarshal", func(t *testing.T) {
							// TEST Binary unmarshal the envelope from the TC
							env := new(protocol.Envelope)
							require.NoError(t, env.UnmarshalBinary(tc.Binary))

							// Marshal the envelope
							json, err := json.Marshal(env)
							require.NoError(t, err)

							// Compare the result to the TC
							require.Equal(t, []byte(tc.JSON), json)
						})

						t.Run("Marshal Body", func(t *testing.T) {
							t.Skip("TODO Enable once Type is included when marshalling a transaction")

							// Unmarshal the body from the TC
							body, err := protocol.UnmarshalTransactionJSON(tc.Inner)
							require.NoError(t, err)

							// TEST Binary marshal the body
							bin, err := body.MarshalBinary()
							require.NoError(t, err)

							// Unmarshal the envelope from the TC
							env := new(protocol.Envelope)
							require.NoError(t, json.Unmarshal(tc.JSON, env))

							// Compare the result to the envelope
							require.Equal(t, env.Transaction.Body, bin)
						})

						t.Run("Unmarshal Body", func(t *testing.T) {
							// Unmarshal the envelope from the TC
							env := new(protocol.Envelope)
							require.NoError(t, json.Unmarshal(tc.JSON, env))

							// TEST Binary unmarshal the body from the envelope
							body, err := protocol.UnmarshalTransaction(env.Transaction.Body)
							require.NoError(t, err)

							// Marshal the body
							json, err := json.Marshal(body)
							require.NoError(t, err)

							// Compare the result to the TC
							require.Equal(t, []byte(tc.Inner), json)
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
							require.Equal(t, []byte(tc.JSON), json)
						})
					})
				}
			})
		}
	})
}
