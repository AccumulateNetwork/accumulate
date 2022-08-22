package core

import (
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type getStateFunc func(accountUrl *url.URL, target interface{}) error
type putStateFunc func(account protocol.Account) error

const labelOracle = "oracle"
const labelGlobals = "network globals"
const labelNetwork = "network definition"
const labelRouting = "routing table"

func (g *GlobalValues) Load(net config.NetworkUrl, getState getStateFunc) error {
	if err := loadAccount(net.JoinPath(protocol.Oracle), labelOracle, getState, new(protocol.AcmeOracle), &g.Oracle); err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	if err := loadAccount(net.JoinPath(protocol.Globals), labelGlobals, getState, new(protocol.NetworkGlobals), &g.Globals); err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	if err := loadAccount(net.JoinPath(protocol.Network), labelNetwork, getState, new(protocol.NetworkDefinition), &g.Network); err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	if err := loadAccount(net.JoinPath(protocol.Routing), labelRouting, getState, new(protocol.RoutingTable), &g.Routing); err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return nil
}

func (g *GlobalValues) Store(net config.NetworkUrl, getState getStateFunc, putState putStateFunc) error {
	if err := storeAccount(net.JoinPath(protocol.Oracle), labelOracle, getState, putState, g.Oracle); err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	if err := storeAccount(net.JoinPath(protocol.Globals), labelGlobals, getState, putState, g.Globals); err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	if g.Network != nil {
		// TODO Make this unconditional once the corresponding part of genesis
		// is unconditional
		if err := storeAccount(net.JoinPath(protocol.Network), labelNetwork, getState, putState, g.Network); err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	if err := storeAccount(net.JoinPath(protocol.Routing), labelRouting, getState, putState, g.Routing); err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return nil
}

func (g *GlobalValues) ParseOracle(entry protocol.DataEntry) error {
	return parseEntryAs(labelOracle, entry, new(protocol.AcmeOracle), &g.Oracle)
}

func (g *GlobalValues) FormatOracle() protocol.DataEntry {
	return formatEntry(g.Oracle)
}

func (g *GlobalValues) ParseGlobals(entry protocol.DataEntry) error {
	return parseEntryAs(labelGlobals, entry, new(protocol.NetworkGlobals), &g.Globals)
}

func (g *GlobalValues) FormatGlobals() protocol.DataEntry {
	return formatEntry(g.Globals)
}

func (g *GlobalValues) ParseNetwork(entry protocol.DataEntry) error {
	version := g.Network.Version
	err := parseEntryAs(labelNetwork, entry, new(protocol.NetworkDefinition), &g.Network)
	if err != nil {
		return err
	}

	if g.Network.Version <= version {
		return errors.Format(errors.StatusBadRequest, "version must increase: %d <= %d", g.Network.Version, version)
	}
	return nil
}

func (g *GlobalValues) FormatNetwork() protocol.DataEntry {
	return formatEntry(g.Network)
}

func (g *GlobalValues) ParseRouting(entry protocol.DataEntry) error {
	return parseEntryAs(labelRouting, entry, new(protocol.RoutingTable), &g.Routing)
}

func (g *GlobalValues) FormatRouting() protocol.DataEntry {
	return formatEntry(g.Routing)
}

func loadAccount[T encoding.BinaryValue](accountUrl *url.URL, name string, getState getStateFunc, value T, ptr *T) error {
	var account *protocol.DataAccount
	err := getState(accountUrl, &account)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load %s: %w", name, err)
	}

	return parseEntryAs(name, account.Entry, value, ptr)
}

func parseEntryAs[T encoding.BinaryValue](name string, entry protocol.DataEntry, value T, ptr *T) error {
	if entry == nil {
		return errors.Format(errors.StatusBadRequest, "unmarshal %s: entry is missing", name)
	}

	if len(entry.GetData()) != 1 {
		return errors.Format(errors.StatusBadRequest, "unmarshal %s: want 1 record, got %d", name, len(entry.GetData()))
	}

	err := value.UnmarshalBinary(entry.GetData()[0])
	if err != nil {
		return errors.Format(errors.StatusBadRequest, "unmarshal %s: %w", name, err)
	}

	*ptr = value
	return nil
}

func storeAccount(accountUrl *url.URL, name string, getState getStateFunc, putState putStateFunc, value encoding.BinaryValue) error {
	var dataAccount *protocol.DataAccount
	err := getState(accountUrl, &dataAccount)
	if err != nil {
		return errors.Format(errors.StatusBadRequest, "load %s: %w", name, err)
	}

	dataAccount.Entry = formatEntry(value)

	err = putState(dataAccount)
	if err != nil {
		return errors.Format(errors.StatusBadRequest, "store %s: %w", name, err)
	}

	return nil
}

func formatEntry(value encoding.BinaryValue) protocol.DataEntry {
	data, err := value.MarshalBinary()
	if err != nil {
		panic(err) // Should be impossible
	}

	return &protocol.AccumulateDataEntry{Data: [][]byte{data}}
}
