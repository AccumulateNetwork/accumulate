package network

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestNetwork_GetBlock(t *testing.T) {
	params, err := url.Parse("acc://staking.acme/parameters")
	require.NoError(t, err, "can't parse parameters")
	n, err := New("http://127.0.1.1:26660/v2", params)
	require.NoError(t, err, "can't create a network")

	for i := int64(0);; {
		got, err := n.GetBlock(i)
		var attempt int
		if err != nil {
			attempt++
			fmt.Printf("attempt %d %d\n",attempt,rand.Int63())
			fmt.Printf(" waiting a block at height %d: %v\n",i,err.Error())
			time.Sleep(time.Second)
		} else {
			fmt.Printf("On Block %d\n", got.MajorHeight)
			require.True(t,i==got.MajorHeight,"block with incorrect height detected")
			i++
			attempt = 0
		}
	}
}