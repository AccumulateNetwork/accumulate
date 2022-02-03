package main

import (
	"context"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
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
