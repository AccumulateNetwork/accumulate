package main

import (
	"context"
	"encoding/json"

	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/client"
)

func getVersion(client *client.Client) *api.VersionResponse {
	resp, err := client.Version(context.Background())
	checkf(err, "failed to get version")

	data, err := json.Marshal(resp.Data)
	checkf(err, "failed to get version")

	version := new(api.VersionResponse)
	err = json.Unmarshal(data, version)
	checkf(err, "failed to get version")

	return version
}
