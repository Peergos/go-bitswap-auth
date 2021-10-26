package bitswap_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	delay "github.com/ipfs/go-ipfs-delay"
	mockrouting "github.com/ipfs/go-ipfs-routing/mock"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/peergos/go-bitswap-auth/auth"
	testinstance "github.com/peergos/go-bitswap-auth/testinstance"
	tn "github.com/peergos/go-bitswap-auth/testnet"
)

// Node 0 has a block and is not connected to anyone
// Node 1 and 2 ask node 3 for the block with valid and invalid auth.
// Node 3 is then connected to 0
func TestConcurrentWantsWithAuth(t *testing.T) {
	my_block := blocks.NewBlock([]byte{'t', 'e', 's', 't'})
	invalid_auth := "bad-auth"
	valid_auth := "good-auth"

	allowGen := func(i int) func(cid.Cid, peer.ID, string) bool {
		return func(c cid.Cid, p peer.ID, a string) bool {
			fmt.Println("Allow-", i, c, a)
			return a == valid_auth
		}
	}
	ig := testinstance.NewTestInstanceGenerator(tn.VirtualNetwork(mockrouting.NewServer(), delay.Fixed(0)), nil, nil, allowGen)
	nodes := ig.UnconnectedInstances(4)
	testinstance.ConnectInstances(nodes[1:])
	nodes[0].Blockstore().Put(auth.NewBlock(my_block, auth.NewWant(my_block.Cid(), valid_auth)))

	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		defer wg.Done()
		received_block1, err := nodes[1].Exchange.GetBlock(context.Background(), auth.NewWant(my_block.Cid(), valid_auth))
		if err != nil {
			t.Fatal(err)
		} else if my_block.Cid() != received_block1.Cid() {
			t.Fatal("expected to receive a block with the same CID that I requested")
		}
	}()

	go func() {
		wg.Add(1)
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err2 := nodes[2].Exchange.GetBlock(ctx, auth.NewWant(my_block.Cid(), invalid_auth))
		if err2 == nil {
			t.Fatal("Peer released block upon receiving request containing an invalid auth string!")
		}
	}()

	testinstance.ConnectInstances([]testinstance.Instance{nodes[0], nodes[3]})
	received_block3, err := nodes[3].Exchange.GetBlock(context.Background(), auth.NewWant(my_block.Cid(), valid_auth))
	if err != nil {
		t.Fatal(err)
	} else if my_block.Cid() != received_block3.Cid() {
		t.Fatal("expected to receive a block with the same CID that I requested")
	}
	wg.Wait()
}
