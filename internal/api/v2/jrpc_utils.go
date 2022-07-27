package api

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func GetLatestRootChainAnchor(tmclient connections.ABCIClient, apiclient connections.APIClient, ledgerurl *url.URL, c context.Context) (bptHash *[32]byte, rootHash *[32]byte, height int64, err error) {
	req := new(GeneralQuery)
	apiinfo := new(ChainQueryResponse)
	req.Url = ledgerurl
	req.Prove = true
	req.Expand = true
	err = apiclient.RequestAPIv2(c, "query", req, apiinfo)
	if err != nil {
		return nil, nil, int64(0), err
	}
	tminfo, err := tmclient.ABCIInfo(c)
	if err != nil {
		return nil, nil, int64(0), err
	}
	height = tminfo.Response.LastBlockHeight
	hash := (*[32]byte)(tminfo.Response.LastBlockAppHash)

	ms := new(managed.MerkleState)
	for _, chain := range apiinfo.Chains {
		if chain.Name == "root" {
			fmt.Println("found root")
			for _, h := range chain.Roots {
				ms.Pending = append(ms.Pending, h)
			}
		}
		ms.Count = int64(chain.Height)
	}

	anchor := ms.GetMDRoot()
	return hash, (*[32]byte)(anchor), height, nil
}

func getLatestDirectoryAnchor(ctx connections.ConnectionContext, anchorurl *url.URL) (lastAnchor uint64, err error) {
	apiclient := ctx.GetAPIClient()
	anchorinfo := new(MultiResponse)
	req := new(TxHistoryQuery)
	req.Url = anchorurl
	req.Count = 0
	err = apiclient.RequestAPIv2(context.Background(), "query-tx-history", req, anchorinfo)
	if err != nil {
		return uint64(0), err

	}
	total := anchorinfo.Total
	req.Start = total
	req.Count = 1
	anchorinfo = new(MultiResponse)
	err = apiclient.RequestAPIv2(context.Background(), "query-tx-history", req, anchorinfo)
	if err != nil {
		return uint64(0), err
	}
	anchor := anchorinfo.Items[0].(protocol.DirectoryAnchor)
	lastAnchor = anchor.MinorBlockIndex
	return lastAnchor, nil
}
