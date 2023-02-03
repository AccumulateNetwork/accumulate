// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sdktest

import (
	"encoding/json"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TestSuite struct {
	Transactions []*TestCaseGroup `json:"transactions,omitempty"`
	Accounts     []*TestCaseGroup `json:"accounts,omitempty"`
}

type TestCaseGroup struct {
	Name  string      `json:"name,omitempty"`
	Cases []*TestCase `json:"cases,omitempty"`
}

type TestCase struct {
	Binary []byte          `json:"binary,omitempty"`
	JSON   json.RawMessage `json:"json,omitempty"`
}

func Load(file string) (*TestSuite, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	ts := new(TestSuite)
	err = json.Unmarshal(b, ts)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

func (ts *TestSuite) Store(file string) error {
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}

	return os.WriteFile(file, b, 0755)
}

func NewTxnTest(env *messaging.Envelope) *TestCase {
	binary, err := env.MarshalBinary()
	if err != nil {
		panic(err)
	}

	jsonb, err := json.Marshal(env)
	if err != nil {
		panic(err)
	}

	return &TestCase{
		Binary: binary,
		JSON:   jsonb,
	}
}

func NewAcntTest(account protocol.Account) *TestCase {
	binary, err := account.MarshalBinary()
	if err != nil {
		panic(err)
	}

	jsonb, err := json.Marshal(account)
	if err != nil {
		panic(err)
	}

	return &TestCase{
		Binary: binary,
		JSON:   jsonb,
	}
}
