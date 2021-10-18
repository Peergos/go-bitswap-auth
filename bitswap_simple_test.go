package bitswap_test

import (
	"context"
	"fmt"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	delay "github.com/ipfs/go-ipfs-delay"
	mockrouting "github.com/ipfs/go-ipfs-routing/mock"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/peergos/go-bitswap-auth/auth"
	testinstance "github.com/peergos/go-bitswap-auth/testinstance"
	tn "github.com/peergos/go-bitswap-auth/testnet"
)

func TestSimpleBlockExchangeWithAuth(t *testing.T) {
	my_block := blocks.NewBlock([]byte{'t', 'e', 's', 't'}) //assuming there will be a block creation function that takes raw bytes and adds auth information
	invalid_auth := ""                                      //assuming that the empty string is always invalid; otherwise, do something else
	valid_auth := "someauth"                                //get or calculate an auth string

	allow := func(c cid.Cid, p peer.ID, a string) bool {
		return a == valid_auth
	}
	ig := testinstance.NewTestInstanceGenerator(tn.VirtualNetwork(mockrouting.NewServer(), delay.Fixed(0)), nil, nil, allow)
	my_instances := ig.Instances(2)
	my_instances[0].Blockstore().Put(auth.NewBlock(my_block))

	//test auth.blockstore; expect that the blockstore of instance[0] has my_block
	has_block, err := my_instances[0].Blockstore().Has(my_block.Cid())
	if err != nil {
		t.Fatal(err)
	} else if !has_block {
		t.Fatal("auth.blockstore failed to store block or doesn't report that it has stored block!")
	}

	//test that I only retrieve block from auth.blockstore with the correct auth string
	_, err = my_instances[0].Blockstore().Get(my_block.Cid(), my_instances[0].Peer, invalid_auth)
	if err == nil {
		t.Fatal("Able to retrieve block from block store using invalid auth string!")
	}

	block_from_auth_blockstore, err := my_instances[0].Blockstore().Get(my_block.Cid(), my_instances[0].Peer, valid_auth)
	if err != nil {
		t.Fatal(err)
	} else if my_block.Cid() != block_from_auth_blockstore.Cid() {
		t.Fatal("expected to retrieve the same block I stored")
	}

	fmt.Println("This is hanging...")
	received_block, err := my_instances[1].Exchange.GetBlock(context.Background(), my_block.Cid(), valid_auth)
	if err != nil {
		t.Fatal(err)
	} else if my_block.Cid() != received_block.Cid() {
		t.Fatal("expected to receive a block with the same CID that I requested")
	}
	fmt.Println("received_block=", string(received_block.RawData()[:]))

	fmt.Println("This is also hanging...")
	//test that I only receive a block from a peer when I provide the correct auth string
	_, err = my_instances[1].Exchange.GetBlock(context.Background(), my_block.Cid(), invalid_auth)
	if err == nil {
		t.Fatal("Peer released block upon receiving request containing an invalid auth string!")
	}
	fmt.Println("escaped hang!")
}
