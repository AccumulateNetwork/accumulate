// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"io"
	"strconv"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type GenesisAddressAndBalances struct {
	Address *url.URL
	Balance int64
}

func LoadFactomAddressesAndBalances(file io.Reader) ([]*GenesisAddressAndBalances, error) {
	factomData, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	// Tolerate CRLF. This is a text file, so it may arrive with either ending
	// depending on how it was checked out, and a trailing carriage return makes
	// ParseInt reject the balance with a syntax error that says nothing about
	// line endings.
	datas := strings.Split(strings.ReplaceAll(string(factomData), "\r\n", "\n"), "\n")
	var genesisFactomData []*GenesisAddressAndBalances
	for _, combinedData := range datas {
		if strings.Contains(combinedData, ":") {
			addressAndBalance := strings.Split(combinedData, ":")
			genesisData := &GenesisAddressAndBalances{}
			address, err := protocol.GetLiteAccountFromFactoidAddress(addressAndBalance[0])
			if err != nil {
				return nil, err
			}
			genesisData.Address = address
			balance, err := strconv.ParseInt(strings.TrimSpace(addressAndBalance[1]), 10, 64)
			if err != nil {
				return nil, err
			}
			genesisData.Balance = balance
			genesisFactomData = append(genesisFactomData, genesisData)
		}
	}
	return genesisFactomData, nil
}
