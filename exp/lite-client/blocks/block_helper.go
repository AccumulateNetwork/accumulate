package blocks

import (
	"context"
	"fmt"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Use canonical types from pkg/api/v3
type MajorBlockRecord = api.MajorBlockRecord
type MinorBlockRecord = api.MinorBlockRecord

func parseUrl(urlStr string) (*accurl.URL, error) {
	return accurl.Parse(urlStr)
}

// Get a KeyBook given its URL.
func GetKeyBook(ctx context.Context, cl *client.Client, urlStr string) (*protocol.KeyBook, error) {
	var book protocol.KeyBook
	if err := queryAccountAs(ctx, cl, urlStr, &book); err != nil {
		return nil, fmt.Errorf("failed to query key book: %v", err)
	}
	return &book, nil
}

// Get a KeyPage from a KeyBook and page index.
func GetKeyPage(ctx context.Context, cl *client.Client, bookUrl string, pageIndex int) (*protocol.KeyPage, error) {
	pageUrl := fmt.Sprintf("%s/page/%d", bookUrl, pageIndex)
	var page protocol.KeyPage
	if err := queryAccountAs(ctx, cl, pageUrl, &page); err != nil {
		return nil, fmt.Errorf("failed to query key page: %v", err)
	}
	return &page, nil
}

func queryAccountAs(ctx context.Context, cl *client.Client, urlStr string, out interface{}) error {
	parsedUrl, err := parseUrl(urlStr)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}
	query := &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: parsedUrl}}
	_, err = cl.QueryAccountAs(ctx, query, out)
	if err != nil {
		return fmt.Errorf("failed to query account %s: %v", urlStr, err)
	}
	return nil
}

func getFirstAuthorityUrl(auth *protocol.AccountAuth) (string, error) {
	if len(auth.Authorities) == 0 {
		return "", fmt.Errorf("no authorities found")
	}
	return auth.Authorities[0].Url.String(), nil
}
