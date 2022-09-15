package walletd

import (
	"context"
	"encoding/json"
	"fmt"
	"gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate/usbwallet"
)

type LedgerHub struct {
	*usbwallet.Hub
}

func NewLedgerHub() (*LedgerHub, error) {
	hub, err := usbwallet.NewLedgerHub()
	if err != nil {
		return nil, err
	}

	return &LedgerHub{
		Hub: hub,
	}, nil
}

func (m *JrpcMethods) GetLedgerInfo(_ context.Context, params json.RawMessage) interface{} {
	resp := &api.LedgerInfoResponse{}
	hub, err := NewLedgerHub()
	if err != nil {
		return validatorError(err)
	}
	resp.LedgerInfos, err = hub.GetLedgerInfos()
	if err != nil {
		return validatorError(err)
	}
	return resp
}

func (h *LedgerHub) GetLedgerInfos() ([]api.LedgerInfo, error) {
	wallets := h.Wallets()
	ledgerInfos := make([]api.LedgerInfo, len(wallets))
	for _, wallet := range wallets {
		info, err := h.getLedgerInfo(wallet)
		if err != nil {
			return nil, err
		}

		ledgerInfos = append(ledgerInfos, *info)
		fmt.Println(wallet.Status())
	}
	return ledgerInfos, nil
}

func (h *LedgerHub) getLedgerInfo(wallet accounts.Wallet) (*api.LedgerInfo, error) {
	err := wallet.Open("")
	if err != nil {
		return nil, err
	}
	defer func() {
		wallet.Close()
	}()

	fmt.Println(wallet.Status())
	fmt.Println(wallet.URL())

	return &api.LedgerInfo{
		Version: "",
	}, nil
}
