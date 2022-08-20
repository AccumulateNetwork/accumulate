package api

import (
	"context"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func getLatestRootChainAnchor(apiclient connections.APIClient, ledgerurl *url.URL, c context.Context) (roothash *[32]byte, err error) {
	req := new(GeneralQuery)
	apiinfo := new(ChainQueryResponse)
	req.Url = ledgerurl
	req.Prove = true
	req.Expand = true
	err = apiclient.RequestAPIv2(c, "query", req, apiinfo)
	if err != nil {
		return nil, err
	}
	ms := new(managed.MerkleState)
	for _, chain := range apiinfo.Chains {
		if chain.Name == "root" {
			for _, h := range chain.Roots {
				ms.Pending = append(ms.Pending, h)
			}
		}
		ms.Count = int64(chain.Height)
	}

	anchor := ms.GetMDRoot()
	return (*[32]byte)(anchor), nil
}

func getLatestDirectoryAnchor(ctx connections.ConnectionContext, anchorurl *url.URL) (lastAnchor uint64, err error) {
	apiclient := ctx.GetAPIClient()
	anchorinfo := new(MultiResponse)
	req := new(TxHistoryQuery)
	req.Url = anchorurl
	req.Count = 1
	err = apiclient.RequestAPIv2(context.Background(), "query-tx-history", req, anchorinfo)
	if err != nil {
		return uint64(0), err

	}

	total := anchorinfo.Total
	req.Start = total - 1
	req.Count = 1
	anchorinfo = new(MultiResponse)
	err = apiclient.RequestAPIv2(context.Background(), "query-tx-history", req, anchorinfo)
	if err != nil {
		return uint64(0), err
	}
	b, err := json.Marshal(anchorinfo.Items[0])
	if err != nil {
		return 0, err
	}
	txnresp := new(TransactionQueryResponse)
	err = json.Unmarshal(b, txnresp)
	if err != nil {
		return 0, err
	}
	anchor, ok := txnresp.Transaction.Body.(*protocol.DirectoryAnchor)
	if !ok {
		// It's not an anchor so it's probably Genesis
		return 0, nil
	}
	lastAnchor = anchor.MinorBlockIndex
	return lastAnchor, nil
}
