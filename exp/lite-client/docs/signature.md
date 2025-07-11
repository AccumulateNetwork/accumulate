# Why I Can't Find Block Signatures in Major Block Queries

Paul,

Here's a summary of what I've found while trying to retrieve block signatures from the Accumulate API, and why I'm currently stuck.

---

## 1. The Response Structure: No Signatures Field

When I query for major blocks using the V2 API, the response is a `MajorQueryResponse` struct. Here is its definition:

```go
// From pkg/client/api/v2/types_gen.go
 type MajorQueryResponse struct {
     MajorBlockIndex uint64      `json:"majorBlockIndex,omitempty"`
     MajorBlockTime  *time.Time  `json:"majorBlockTime,omitempty"`
     MinorBlocks     []*MinorBlock `json:"minorBlocks,omitempty"`
     LastBlockTime   *time.Time  `json:"lastBlockTime,omitempty"`
 }
```

**Notice:** There is no `signatures` field (nor `threshold`, nor anything related to block signatures) in this struct.

---

## 2. The Query Code Path

The function I use to fetch major blocks is as follows:

```go
func QueryMajorBlocksV2(ctx context.Context, cl *client.Client, partitionUrl string, startIndex, count uint64) ([]*client.MajorQueryResponse, error) {
    // ...
    resp, err := cl.QueryMajorBlocks(ctx, query)
    // ...
    // Unmarshal items into []*client.MajorQueryResponse
    for _, item := range resp.Items {
        var block client.MajorQueryResponse
        if err := json.Unmarshal(data, &block); err != nil {
            // ...
        }
        blocks = append(blocks, &block)
    }
    return blocks, nil
}
```

So, every block I get is a `*MajorQueryResponse` -- which, again, has no signature data.

---

## 3. Test Results: No Signatures Present

My tests confirm this. For example:

```go
for i, block := range recordRange {
    t.Logf("[v2] Block %d: majorBlockIndex=%v majorBlockTime=%v", i, block.MajorBlockIndex, block.MajorBlockTime)
}
```

I can see the index and time, but there is no way to access signatures because they simply aren't present in the struct or the response.

---

## 4. The Underlying Client Code

The client code that actually makes the API call is:

```go
func (c *Client) QueryMajorBlocks(ctx context.Context, req *api.MajorBlocksQuery) (*api.MultiResponse, error) {
    return c.RequestAPIv2(ctx, "query-major-blocks", req, &resp)
}
```

And the response is always unmarshaled into the struct shown above.

---

## 5. Conclusion: Why I'm Stuck

- The API response for major block queries (V2) does not contain block signatures.
- The Go struct used for unmarshaling the response has no field for signatures.
- There is no code path (in the lite-client or the underlying client) that exposes block signatures via this query.

**Unless there is a different API or query method I'm missing, there is simply no way to retrieve block signatures from the standard major block query response.**

If you know of another endpoint or method to get this data, please let me know! Otherwise, this appears to be a missing feature or a limitation of the current API.

---

Let me know if you want me to dig deeper or try a different approach.
type MajorQueryResponse struct {

	// MajorBlockIndex is the index of the major block..
	MajorBlockIndex uint64 `json:"majorBlockIndex,omitempty" form:"majorBlockIndex" query:"majorBlockIndex" validate:"required"`
	// MajorBlockTime is the start time of the major block..
	MajorBlockTime *time.Time    `json:"majorBlockTime,omitempty" form:"majorBlockTime" query:"majorBlockTime" validate:"required"`
	MinorBlocks    []*MinorBlock `json:"minorBlocks,omitempty" form:"minorBlocks" query:"minorBlocks" validate:"required"`
	LastBlockTime  *time.Time    `json:"lastBlockTime,omitempty" form:"lastBlockTime" query:"lastBlockTime" validate:"required"`
}

Query Major Blocks Function using V2:

package blocks

import (
	"context"
	"encoding/json"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryMajorBlocksV2 retrieves a paginated slice of major blocks using the v2 API.
// Each block is returned as a typed *client.MajorQueryResponse for structured access.
func QueryMajorBlocksV2(ctx context.Context, cl *client.Client, partitionUrl string, startIndex, count uint64) ([]*client.MajorQueryResponse, error) {
	parsedUrl, err := accurl.Parse(partitionUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse partition URL: %v", err)
	}

	query := &client.MajorBlocksQuery{
		QueryPagination: client.QueryPagination{
			Start: startIndex,
			Count: count,
		},
		UrlQuery: client.UrlQuery{
			Url: parsedUrl,
		},
	}

	resp, err := cl.QueryMajorBlocks(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major blocks (v2): %v", err)
	}

	if resp == nil || len(resp.Items) == 0 {
		return nil, fmt.Errorf("no major block records returned (v2)")
	}

	// Unmarshal items into []*api.MajorQueryResponse
	var blocks []*client.MajorQueryResponse
	for _, item := range resp.Items {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal major block item: %w", err)
		}
		var block client.MajorQueryResponse
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, fmt.Errorf("failed to unmarshal major block item: %w", err)
		}
		blocks = append(blocks, &block)
	}
	return blocks, nil
}

Test File:

package blocks

import (
	"context"
	"os"
	"testing"
	"time"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// TestQueryMajorBlocks_Kermit connects to the Kermit testnet and retrieves major blocks using v3 (default).
func TestQueryMajorBlocks_Kermit(t *testing.T) {
	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	cl, err := client.New(kermitUrl)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	startIndex := uint64(0)
	count := uint64(2)
	partition := "acc://dn"

	recordRange, err := QueryMajorBlocks(ctx, cl, partition, startIndex, count, "v3")
	if err != nil {
		t.Fatalf("QueryMajorBlocks (v3) failed: %v", err)
	}
	if len(recordRange) == 0 {
		t.Fatalf("No major blocks returned from Kermit testnet (v3)")
	}

	t.Logf("[v3] Retrieved %d major blocks from Kermit", len(recordRange))
	for i, block := range recordRange {
		if block == nil {
			t.Errorf("[v3] Block %d is nil", i)
			continue
		}
		if block.MajorBlockIndex == 0 {
			t.Errorf("[v3] Block %d missing or zero 'MajorBlockIndex' field", i)
		}
		if block.MajorBlockTime == nil {
			t.Errorf("[v3] Block %d missing 'MajorBlockTime' field", i)
		}
		t.Logf("[v3] Block %d: majorBlockIndex=%v majorBlockTime=%v", i, block.MajorBlockIndex, block.MajorBlockTime)
	}
}

// TestQueryMajorBlocksV2_Kermit connects to the Kermit testnet and retrieves major blocks using v2 API.
func TestQueryMajorBlocksV2_Kermit(t *testing.T) {
	kermitUrl := os.Getenv("KERMIT_API")
	if kermitUrl == "" {
		kermitUrl = "https://kermit.accumulatenetwork.io"
	}

	cl, err := client.New(kermitUrl)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	startIndex := uint64(0)
	count := uint64(2)
	partition := "acc://dn"

	recordRange, err := QueryMajorBlocks(ctx, cl, partition, startIndex, count, "v2")
	if err != nil {
		t.Fatalf("QueryMajorBlocks (v2) failed: %v", err)
	}
	if len(recordRange) == 0 {
		t.Fatalf("No major blocks returned from Kermit testnet (v2)")
	}

	t.Logf("[v2] Retrieved %d major blocks from Kermit", len(recordRange))
	for i, block := range recordRange {
		if block == nil {
			t.Errorf("[v2] Block %d is nil", i)
			continue
		}
		if block.MajorBlockIndex == 0 {
			t.Errorf("[v2] Block %d missing or zero 'MajorBlockIndex' field", i)
		}
		if block.MajorBlockTime == nil {
			t.Errorf("[v2] Block %d missing 'MajorBlockTime' field", i)
		}
		t.Logf("[v2] Block %d: majorBlockIndex=%v majorBlockTime=%v", i, block.MajorBlockIndex, block.MajorBlockTime)
	}
}

Result:

Running tool: C:\Program Files\Go\bin\go.exe test -timeout 30s -run ^TestQueryMajorBlocksV2_Kermit$ gitlab.com/accumulatenetwork/accumulate/exp/lite-client/blocks

=== RUN   TestQueryMajorBlocksV2_Kermit
    c:\Users\pradord\Documents\_Accumulate\GitLabRepo\accumulate\exp\lite-client\blocks\block_major_test.go:82: [v2] Retrieved 2 major blocks from Kermit
    c:\Users\pradord\Documents\_Accumulate\GitLabRepo\accumulate\exp\lite-client\blocks\block_major_test.go:94: [v2] Block 0: majorBlockIndex=1 majorBlockTime=2025-03-09 19:06:31 +0000 UTC
    c:\Users\pradord\Documents\_Accumulate\GitLabRepo\accumulate\exp\lite-client\blocks\block_major_test.go:94: [v2] Block 1: majorBlockIndex=2 majorBlockTime=2025-03-10 00:00:01 +0000 UTC
--- PASS: TestQueryMajorBlocksV2_Kermit (4.94s)
PASS
ok      gitlab.com/accumulatenetwork/accumulate/exp/lite-client/blocks  (cached)


PATH used to Query:

pkg/client/api/v2/api_v2_sdk_gen.go

func (c *Client) QueryMajorBlocks(ctx context.Context, req *api.MajorBlocksQuery) (*api.MultiResponse, error) {
	var resp api.MultiResponse

	err := c.RequestAPIv2(ctx, "query-major-blocks", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

pkg/client/api/v2/client.go

func (c *Client) RequestAPIv2(ctx context.Context, method string, params, result interface{}) error {
	if c.DebugRequest {
		fmt.Println("accumulated:", c.serverV2) //nolint:noprint
	}

	return c.Client.Request(ctx, c.serverV2, method, params, result)
}

func (c *Client) Request(ctx context.Context, url, method string,
	params, result interface{}) error {

	// Generate a psuedo random ID for this request.
	reqID := rand.Int()%5000 + 1

	// Marshal the JSON RPC Request.
	var req interface{}
	var batch bool
	switch v := params.(type) {
	case Request:
		req = v
	case BatchRequest:
		req, batch = v, true
	default:
		req = Request{ID: reqID, Method: method, Params: params}
	}
	if c.DebugRequest {
		if c.Log == nil {
			c.Log = log.New(os.Stderr, "", 0)
		}
		c.Log.Println(req)
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// Compose the HTTP request.
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqData))
	if err != nil {
		return err
	}
	if ctx != nil {
		httpReq = httpReq.WithContext(ctx)
	}
	httpReq.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
	for k, v := range c.Header {
		httpReq.Header[http.CanonicalHeaderKey(k)] = v
	}
	if c.BasicAuth {
		httpReq.SetBasicAuth(c.User, c.Password)
	}

	// Make the request.
	httpRes, err := c.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpRes.Body.Close()

	// Read the HTTP response.
	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return err
	}
	if c.DebugRequest {
		fmt.Println("<--", string(body))
		fmt.Println()
	}

	if !batch {
		// Unmarshal the HTTP response into a JSON RPC response.
		var resID int
		res := Response{Result: result, ID: &resID}
		if err := json.Unmarshal(body, &res); err != nil {
			return newErrorUnexpectedHTTPResponse(err, body, httpRes)
		}

		if res.HasError() {
			return res.Error
		}

		return nil
	}

	var rawResp []json.RawMessage
	err = json.Unmarshal(body, &rawResp)
	if err != nil {
		return newErrorUnexpectedHTTPResponse(err, body, httpRes)
	}

	rv := reflect.ValueOf(result)
	if rv.Kind() != reflect.Ptr || rv.Type().Elem().Kind() != reflect.Slice {
		panic("when sending a batch the result must be a pointer to a slice")
	}
	responses := reflect.MakeSlice(rv.Type().Elem(), len(rawResp), len(rawResp))
	rv.Elem().Set(responses)

	var errs BatchError
	for i, raw := range rawResp {
		var resID int
		result := responses.Index(i).Addr().Interface()
		res := Response{Result: result, ID: &resID}
		if err := json.Unmarshal(raw, &res); err != nil {
			return newErrorUnexpectedHTTPResponse(err, body, httpRes)
		}

		if res.HasError() {
			errs = append(errs, res.Error)
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

