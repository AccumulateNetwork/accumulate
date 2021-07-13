// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package fat107

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/AdamSLevy/jsonrpc2/v14"
	"github.com/AdamSLevy/retry"
	"github.com/Factom-Asset-Tokens/factom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	RunTestCreate bool
	c             = factom.NewClient()
	ecEs          = func() ECEsAddress {
		var ecEs ECEsAddress
		ecEs.Set("EC3emTZegtoGuPz3MRA4uC8SU6Up52abqQUEKqW44TppGGAc4Vrq")
		return ecEs
	}()
)

type ECEsAddress struct {
	EC factom.ECAddress
	Es factom.EsAddress
}

func (e *ECEsAddress) Set(adrStr string) error {
	if err := e.EC.Set(adrStr); err != nil {
		if err := e.Es.Set(adrStr); err != nil {
			return err
		}
		e.EC = e.Es.ECAddress()
	}
	return nil
}

func (e ECEsAddress) String() string {
	return e.EC.String()
}

func init() {
	rand.Seed(time.Now().Unix())
	flag.BoolVar(&RunTestCreate, "create", false, "Run the Create test")
	flag.StringVar(&c.FactomdServer, "factomd", c.FactomdServer,
		"factomd API endpoint")
	flag.Var(&ecEs, "ecadr", "Es or EC address to query from factom-walletd")
	flag.StringVar(&c.WalletdServer, "walletd", c.WalletdServer,
		"factom-walletd API endpoint")
	c.Factomd.Timeout = 10 * time.Second
	c.Walletd.Timeout = 10 * time.Second
}

func TestMain(m *testing.M) {
	flag.Parse()
	m.Run()
}

func TestGenerate(t *testing.T) {
	require := require.New(t)

	// Get Es Address
	if factom.Bytes32(ecEs.Es).IsZero() {
		es, err := ecEs.EC.GetEsAddress(nil, c)
		require.NoError(err)
		ecEs.Es = es
	}

	size := factom.EntryMaxDataSize * 10
	data := make([]byte, size)
	rand.Read(data)

	dataHash := factom.Bytes32(sha256.Sum256(data))
	dataHash = sha256.Sum256(dataHash[:])

	dataBuf := bytes.NewBuffer(data)
	cDataBuf := bytes.NewBuffer(make([]byte, 0, len(data)))

	gz := gzip.NewWriter(cDataBuf)
	_, err := dataBuf.WriteTo(gz)
	require.NoError(err)
	err = gz.Close()
	require.NoError(err)

	compression := Compression{Format: "gzip", Size: uint64(cDataBuf.Len())}

	fmt.Println("Generating data store:")
	fmt.Println("\tdataHash:", dataHash)
	fmt.Println("\tsize:", size)
	fmt.Println("\tcompression:", compression)
	chainID, txIDs, eHashes, commits, reveals, totalCost, err :=
		Generate(nil, ecEs.Es, cDataBuf, &compression, uint64(size),
			&dataHash, nil)
	require.NoError(err)
	fmt.Println("\tChainID:", chainID)
	fmt.Println("\tEntry Count:", len(reveals))
	fmt.Println("\tTotal Cost:", totalCost)
	assert := assert.New(t)
	assert.Len(commits, 13)
	assert.Len(reveals, 13)
	assert.EqualValues(113, int(totalCost))
	assert.False(chainID.IsZero())

	if !RunTestCreate {
		t.SkipNow()
	}

	fmt.Println("Creating data store...")

	ecBal, err := ecEs.EC.GetBalance(nil, c)
	require.NoError(err)

	require.LessOrEqual(uint64(totalCost), ecBal, "insufficient balance")

	policy := retry.Randomize{
		Factor: .25,
		Policy: retry.LimitTotal{
			Limit: 15 * time.Minute,
			Policy: retry.Max{
				Cap: 5 * time.Second,
				Policy: retry.Exponential{
					Initial:    5 * time.Millisecond,
					Multiplier: 1.25,
				},
			},
		},
	}
	notify := func(hash *factom.Bytes32, s string) func(error, uint, time.Duration) {
		return func(_ error, r uint, d time.Duration) {
			fmt.Println(hash.String()[:6], "waiting for", s, "ack...", r, d)
		}
	}
	_ = notify
	start := time.Now()
	for i, commit := range commits {
		fmt.Printf("%v %v committing ... ", i+1, eHashes[i].String()[:6])
		start := time.Now()
		require.NoError(Submit(policy,
			func() error { return c.Commit(nil, commit) },
			&txIDs[i], nil))
		fmt.Printf("%v\n", time.Since(start))

		fmt.Printf("%v %v revealing ... ", i+1, eHashes[i].String()[:6])
		start = time.Now()
		require.NoError(Submit(policy,
			func() error { return c.Reveal(nil, reveals[i]) },
			&eHashes[i], &chainID))
		fmt.Printf("%v\n", time.Since(start))
	}
	fmt.Println("All entries submitted.", time.Since(start))

	newECBal, err := ecEs.EC.GetBalance(nil, c)
	require.NoError(err)
	require.EqualValues(int(ecBal-uint64(totalCost)), int(newECBal))
}

func Submit(policy retry.Policy, submit func() error,
	hash, chainID *factom.Bytes32) error {
	return retry.Run(nil, policy, nil, func(err error, r uint, _ time.Duration) {
		if r > 10 {
			fmt.Printf("%v submit %v attempts, error: %v\n",
				hash.String()[:6], r, err)
		}
	}, func() error {
		if err := submit(); err != nil {
			jErr, ok := err.(jsonrpc2.Error)
			if !ok {
				return retry.ErrorStop(err)
			}
			if jErr.Message != "Repeated Commit" {
				return retry.ErrorStop(err)
			}
			fmt.Println("Repeated Commit")
		}
		time.Sleep(500 * time.Millisecond)
		if err := retry.Run(nil, policy, nil, func(err error, r uint, _ time.Duration) {
			if r > 10 {
				fmt.Printf("%v check %v attempts, error: %v\n",
					hash.String()[:6], r, err)
			}
		}, func() error {
			status, err := txStatus(nil, c, hash, chainID)
			if err != nil {
				return err
			}
			switch status {
			case "TransactionACK", "DBlockConfirmed":
				return nil
			case "NotConfirmed":
				return fmt.Errorf("re-check status")
			case "Unknown":
				return retry.ErrorStop(
					fmt.Errorf("retry submission"))
			default:
				panic(fmt.Errorf("invalid status: %v", status))
			}
		}); err != nil {
			return err
		}
		return nil
	})
}

func txStatus(ctx context.Context, c *factom.Client,
	txID *factom.Bytes32, chainID *factom.Bytes32) (string, error) {
	params := struct {
		Hash    *factom.Bytes32 `json:"hash"`
		ChainID string
	}{Hash: txID, ChainID: "c"}
	if chainID != nil {
		params.ChainID = chainID.String()
	}

	type Status struct {
		Status string
	}
	var res struct {
		Commit Status `json:"commitdata"`
		Reveal Status `json:"entrydata"`
	}

	if err := c.FactomdRequest(ctx, "ack", params, &res); err != nil {
		return "", err
	}

	if chainID == nil {
		return res.Commit.Status, nil
	}
	return res.Reveal.Status, nil
}
