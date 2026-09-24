package repro4366

import (
	"encoding/json"
	"os"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestRouteStrandedAccount: which partition did the one stranded user
// transaction of run 20260919T191634Z route to, under that run's own
// routing table?
func TestRouteStrandedAccount(t *testing.T) {
	f := os.Getenv("NETDEF")
	if f == "" {
		t.Skip("set NETDEF")
	}
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Routing *protocol.RoutingTable `json:"routing"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	tree, err := routing.NewRouteTree(doc.Routing)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"acc://lg-0a85adbd08bc679d.acme",
		"acc://lg-0a85adbd08bc679d.acme/book/2",
	} {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		p, err := tree.Route(u)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s -> %s", s, p)
	}
}
