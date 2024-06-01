// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NewGlobals returns GlobalValues with uninitialized values set to the default.
func NewGlobals(g *GlobalValues) *GlobalValues {
	// TODO: This should be part of genesis but that causes an import loop

	if g == nil {
		g = new(GlobalValues)
	}

	if g.Oracle == nil {
		g.Oracle = new(protocol.AcmeOracle)
		if g.Oracle.Price == 0 {
			g.Oracle.Price = uint64(protocol.InitialAcmeOracleValue)
		}
	}

	// Set the initial threshold to 2/3 & MajorBlockSchedule
	if g.Globals == nil {
		g.Globals = new(protocol.NetworkGlobals)
	}
	if g.Globals.OperatorAcceptThreshold.Numerator == 0 {
		g.Globals.OperatorAcceptThreshold.Set(2, 3)
	}
	if g.Globals.ValidatorAcceptThreshold.Numerator == 0 {
		g.Globals.ValidatorAcceptThreshold.Set(2, 3)
	}
	if g.Globals.MajorBlockSchedule == "" {
		g.Globals.MajorBlockSchedule = protocol.DefaultMajorBlockSchedule
	}
	if g.Globals.FeeSchedule == nil {
		g.Globals.FeeSchedule = new(protocol.FeeSchedule)
		g.Globals.FeeSchedule.CreateIdentitySliding = []protocol.Fee{
			protocol.FeeCreateIdentity << 12,
			protocol.FeeCreateIdentity << 11,
			protocol.FeeCreateIdentity << 10,
			protocol.FeeCreateIdentity << 9,
			protocol.FeeCreateIdentity << 8,
			protocol.FeeCreateIdentity << 7,
			protocol.FeeCreateIdentity << 6,
			protocol.FeeCreateIdentity << 5,
			protocol.FeeCreateIdentity << 4,
			protocol.FeeCreateIdentity << 3,
			protocol.FeeCreateIdentity << 2,
			protocol.FeeCreateIdentity << 1,
		}
	}
	if g.Globals.Limits == nil {
		g.Globals.Limits = new(protocol.NetworkLimits)
	}
	if g.Globals.Limits.DataEntryParts == 0 {
		g.Globals.Limits.DataEntryParts = 100
	}
	if g.Globals.Limits.AccountAuthorities == 0 {
		g.Globals.Limits.AccountAuthorities = 20
	}
	if g.Globals.Limits.BookPages == 0 {
		g.Globals.Limits.BookPages = 20
	}
	if g.Globals.Limits.PageEntries == 0 {
		g.Globals.Limits.PageEntries = 100
	}
	if g.Globals.Limits.IdentityAccounts == 0 {
		g.Globals.Limits.IdentityAccounts = 100
	}
	return g
}

type getStateFunc = func(accountUrl *url.URL, target interface{}) error
type putStateFunc = func(account protocol.Account) error

const labelOracle = "oracle"
const labelGlobals = "network globals"
const labelNetwork = "network definition"
const labelRouting = "routing table"

func (g *GlobalValues) Load(net *url.URL, getState getStateFunc) error {
	// Load the oracle
	e1 := g.LoadOracle(net, getState)

	// Load network globals
	e2 := g.LoadGlobals(net, getState)

	// Load the network definition
	e3 := g.LoadNetwork(net, getState)

	// Load the routing table
	e4 := g.LoadRouting(net, getState)

	// Load the executor version
	e5 := g.loadFromLedger(net, getState)

	switch {
	case e1 != nil:
		return e1
	case e2 != nil:
		return e2
	case e3 != nil:
		return e3
	case e4 != nil:
		return e4
	}
	return e5
}

func (g *GlobalValues) LoadOracle(net *url.URL, getState getStateFunc) error {
	return loadAccount(net.JoinPath(protocol.Oracle), labelOracle, getState, new(protocol.AcmeOracle), &g.Oracle)
}

func (g *GlobalValues) LoadGlobals(net *url.URL, getState getStateFunc) error {
	return loadAccount(net.JoinPath(protocol.Globals), labelGlobals, getState, new(protocol.NetworkGlobals), &g.Globals)
}

func (g *GlobalValues) LoadNetwork(net *url.URL, getState getStateFunc) error {
	return loadAccount(net.JoinPath(protocol.Network), labelNetwork, getState, new(protocol.NetworkDefinition), &g.Network)
}

func (g *GlobalValues) LoadRouting(net *url.URL, getState getStateFunc) error {
	return loadAccount(net.JoinPath(protocol.Routing), labelRouting, getState, new(protocol.RoutingTable), &g.Routing)
}

func (g *GlobalValues) loadFromLedger(net *url.URL, getState getStateFunc) error {
	var ledger *protocol.SystemLedger
	if err := getState(net.JoinPath(protocol.Ledger), &ledger); err != nil {
		return errors.BadRequest.WithFormat("load system ledger: %w", err)
	} else {
		g.ExecutorVersion = ledger.ExecutorVersion
		g.BvnExecutorVersions = ledger.BvnExecutorVersions
	}
	return nil
}

// InitializeDataAccounts sets the initial state of the network data accounts
// for genesis.
func (g *GlobalValues) InitializeDataAccounts(net *url.URL, getState getStateFunc, putState putStateFunc) error {
	if err := g.storeAccount(net.JoinPath(protocol.Oracle), labelOracle, getState, putState, g.Oracle); err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if err := g.storeAccount(net.JoinPath(protocol.Globals), labelGlobals, getState, putState, g.Globals); err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if g.Network != nil {
		// TODO Make this unconditional once the corresponding part of genesis
		// is unconditional
		if err := g.storeAccount(net.JoinPath(protocol.Network), labelNetwork, getState, putState, g.Network); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	if err := g.storeAccount(net.JoinPath(protocol.Routing), labelRouting, getState, putState, g.Routing); err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

func (g *GlobalValues) ParseOracle(entry protocol.DataEntry) error {
	return parseEntryAs(labelOracle, entry, new(protocol.AcmeOracle), &g.Oracle)
}

func (g *GlobalValues) FormatOracle() protocol.DataEntry {
	return g.formatEntry(g.Oracle)
}

func (g *GlobalValues) ParseGlobals(entry protocol.DataEntry) error {
	return parseEntryAs(labelGlobals, entry, new(protocol.NetworkGlobals), &g.Globals)
}

func (g *GlobalValues) FormatGlobals() protocol.DataEntry {
	return g.formatEntry(g.Globals)
}

func (g *GlobalValues) ParseNetwork(entry protocol.DataEntry) error {
	version := g.Network.Version
	err := parseEntryAs(labelNetwork, entry, new(protocol.NetworkDefinition), &g.Network)
	if err != nil {
		return err
	}

	if g.Network.Version <= version {
		return errors.BadRequest.WithFormat("version must increase: %d <= %d", g.Network.Version, version)
	}
	return nil
}

func (g *GlobalValues) FormatNetwork() protocol.DataEntry {
	return g.formatEntry(g.Network)
}

func (g *GlobalValues) ParseRouting(entry protocol.DataEntry) error {
	return parseEntryAs(labelRouting, entry, new(protocol.RoutingTable), &g.Routing)
}

func (g *GlobalValues) FormatRouting() protocol.DataEntry {
	return g.formatEntry(g.Routing)
}

func loadAccount[T encoding.BinaryValue](accountUrl *url.URL, name string, getState getStateFunc, value T, ptr *T) error {
	var account *protocol.DataAccount
	err := getState(accountUrl, &account)
	if err != nil {
		return errors.UnknownError.WithFormat("load %s: %w", name, err)
	}

	return parseEntryAs(name, account.Entry, value, ptr)
}

func parseEntryAs[T encoding.BinaryValue](name string, entry protocol.DataEntry, value T, ptr *T) error {
	if entry == nil {
		return errors.BadRequest.WithFormat("unmarshal %s: entry is missing", name)
	}

	if len(entry.GetData()) != 1 {
		return errors.BadRequest.WithFormat("unmarshal %s: want 1 record, got %d", name, len(entry.GetData()))
	}

	err := value.UnmarshalBinary(entry.GetData()[0])
	if err != nil {
		return errors.BadRequest.WithFormat("unmarshal %s: %w", name, err)
	}

	*ptr = value
	return nil
}

func (g *GlobalValues) storeAccount(accountUrl *url.URL, name string, getState getStateFunc, putState putStateFunc, value encoding.BinaryValue) error {
	var dataAccount *protocol.DataAccount
	err := getState(accountUrl, &dataAccount)
	if err != nil {
		return errors.BadRequest.WithFormat("load %s: %w", name, err)
	}

	dataAccount.Entry = g.formatEntry(value)

	err = putState(dataAccount)
	if err != nil {
		return errors.BadRequest.WithFormat("store %s: %w", name, err)
	}

	return nil
}

func (g *GlobalValues) formatEntry(value encoding.BinaryValue) protocol.DataEntry {
	data, err := value.MarshalBinary()
	if err != nil {
		panic(err) // Should be impossible
	}

	if g.ExecutorVersion.V2Enabled() {
		return &protocol.DoubleHashDataEntry{Data: [][]byte{data}}
	}
	return &protocol.AccumulateDataEntry{Data: [][]byte{data}}
}
