package blockpresencemanager

import (
	"fmt"
	"testing"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/peergos/go-bitswap-auth/internal/testutil"
        "github.com/peergos/go-bitswap-auth/auth"
)

const (
	expHasFalseMsg         = "Expected PeerHasBlock to return false"
	expHasTrueMsg          = "Expected PeerHasBlock to return true"
	expDoesNotHaveFalseMsg = "Expected PeerDoesNotHaveBlock to return false"
	expDoesNotHaveTrueMsg  = "Expected PeerDoesNotHaveBlock to return true"
)

func TestBlockPresenceManager(t *testing.T) {
	bpm := New()

	p := testutil.GeneratePeers(1)[0]
	cids := testutil.GenerateWants(2)
	c0 := cids[0]
	c1 := cids[1]

	// Nothing stored yet, both PeerHasBlock and PeerDoesNotHaveBlock should
	// return false
	if bpm.PeerHasBlock(p, c0.Cid) {
		t.Fatal(expHasFalseMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p, c0.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}

	// HAVE cid0 / DONT_HAVE cid1
	bpm.ReceiveFrom(p, []auth.Want{c0}, []auth.Want{c1})

	// Peer has received HAVE for cid0
	if !bpm.PeerHasBlock(p, c0.Cid) {
		t.Fatal(expHasTrueMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p, c0.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}

	// Peer has received DONT_HAVE for cid1
	if !bpm.PeerDoesNotHaveBlock(p, c1.Cid) {
		t.Fatal(expDoesNotHaveTrueMsg)
	}
	if bpm.PeerHasBlock(p, c1.Cid) {
		t.Fatal(expHasFalseMsg)
	}

	// HAVE cid1 / DONT_HAVE cid0
	bpm.ReceiveFrom(p, []auth.Want{c1}, []auth.Want{c0})

	// DONT_HAVE cid0 should NOT over-write earlier HAVE cid0
	if bpm.PeerDoesNotHaveBlock(p, c0.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}
	if !bpm.PeerHasBlock(p, c0.Cid) {
		t.Fatal(expHasTrueMsg)
	}

	// HAVE cid1 should over-write earlier DONT_HAVE cid1
	if !bpm.PeerHasBlock(p, c1.Cid) {
		t.Fatal(expHasTrueMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p, c1.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}

	// Remove cid0
	bpm.RemoveKeys([]auth.Want{c0})

	// Nothing stored, both PeerHasBlock and PeerDoesNotHaveBlock should
	// return false
	if bpm.PeerHasBlock(p, c0.Cid) {
		t.Fatal(expHasFalseMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p, c0.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}

	// Remove cid1
	bpm.RemoveKeys([]auth.Want{c1})

	// Nothing stored, both PeerHasBlock and PeerDoesNotHaveBlock should
	// return false
	if bpm.PeerHasBlock(p, c1.Cid) {
		t.Fatal(expHasFalseMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p, c1.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}
}

func TestAddRemoveMulti(t *testing.T) {
	bpm := New()

	peers := testutil.GeneratePeers(2)
	p0 := peers[0]
	p1 := peers[1]
	cids := testutil.GenerateWants(3)
	c0 := cids[0]
	c1 := cids[1]
	c2 := cids[2]

	// p0: HAVE cid0, cid1 / DONT_HAVE cid1, cid2
	// p1: HAVE cid1, cid2 / DONT_HAVE cid0
	bpm.ReceiveFrom(p0, []auth.Want{c0, c1}, []auth.Want{c1, c2})
	bpm.ReceiveFrom(p1, []auth.Want{c1, c2}, []auth.Want{c0})

	// Peer 0 should end up with
	// - HAVE cid0
	// - HAVE cid1
	// - DONT_HAVE cid2
	if !bpm.PeerHasBlock(p0, c0.Cid) {
		t.Fatal(expHasTrueMsg)
	}
	if !bpm.PeerHasBlock(p0, c1.Cid) {
		t.Fatal(expHasTrueMsg)
	}
	if !bpm.PeerDoesNotHaveBlock(p0, c2.Cid) {
		t.Fatal(expDoesNotHaveTrueMsg)
	}

	// Peer 1 should end up with
	// - HAVE cid1
	// - HAVE cid2
	// - DONT_HAVE cid0
	if !bpm.PeerHasBlock(p1, c1.Cid) {
		t.Fatal(expHasTrueMsg)
	}
	if !bpm.PeerHasBlock(p1, c2.Cid) {
		t.Fatal(expHasTrueMsg)
	}
	if !bpm.PeerDoesNotHaveBlock(p1, c0.Cid) {
		t.Fatal(expDoesNotHaveTrueMsg)
	}

	// Remove cid1 and cid2. Should end up with
	// Peer 0: HAVE cid0
	// Peer 1: DONT_HAVE cid0
	bpm.RemoveKeys([]auth.Want{c1, c2})
	if !bpm.PeerHasBlock(p0, c0.Cid) {
		t.Fatal(expHasTrueMsg)
	}
	if !bpm.PeerDoesNotHaveBlock(p1, c0.Cid) {
		t.Fatal(expDoesNotHaveTrueMsg)
	}

	// The other keys should have been cleared, so both HasBlock() and
	// DoesNotHaveBlock() should return false
	if bpm.PeerHasBlock(p0, c1.Cid) {
		t.Fatal(expHasFalseMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p0, c1.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}
	if bpm.PeerHasBlock(p0, c2.Cid) {
		t.Fatal(expHasFalseMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p0, c2.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}
	if bpm.PeerHasBlock(p1, c1.Cid) {
		t.Fatal(expHasFalseMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p1, c1.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}
	if bpm.PeerHasBlock(p1, c2.Cid) {
		t.Fatal(expHasFalseMsg)
	}
	if bpm.PeerDoesNotHaveBlock(p1, c2.Cid) {
		t.Fatal(expDoesNotHaveFalseMsg)
	}
}

func TestAllPeersDoNotHaveBlock(t *testing.T) {
	bpm := New()

	peers := testutil.GeneratePeers(3)
	p0 := peers[0]
	p1 := peers[1]
	p2 := peers[2]

	cids := testutil.GenerateWants(3)
	c0 := cids[0]
	c1 := cids[1]
	c2 := cids[2]

	//      c0  c1  c2
	//  p0   ?  N   N
	//  p1   N  Y   ?
	//  p2   Y  Y   N
	bpm.ReceiveFrom(p0, []auth.Want{}, []auth.Want{c1, c2})
	bpm.ReceiveFrom(p1, []auth.Want{c1}, []auth.Want{c0})
	bpm.ReceiveFrom(p2, []auth.Want{c0, c1}, []auth.Want{c2})

	type testcase struct {
		peers []peer.ID
		ks    []auth.Want
		exp   []auth.Want
	}

	testcases := []testcase{
		{[]peer.ID{p0}, []auth.Want{c0}, []auth.Want{}},
		{[]peer.ID{p1}, []auth.Want{c0}, []auth.Want{c0}},
		{[]peer.ID{p2}, []auth.Want{c0}, []auth.Want{}},

		{[]peer.ID{p0}, []auth.Want{c1}, []auth.Want{c1}},
		{[]peer.ID{p1}, []auth.Want{c1}, []auth.Want{}},
		{[]peer.ID{p2}, []auth.Want{c1}, []auth.Want{}},

		{[]peer.ID{p0}, []auth.Want{c2}, []auth.Want{c2}},
		{[]peer.ID{p1}, []auth.Want{c2}, []auth.Want{}},
		{[]peer.ID{p2}, []auth.Want{c2}, []auth.Want{c2}},

		// p0 recieved DONT_HAVE for c1 & c2 (but not for c0)
		{[]peer.ID{p0}, []auth.Want{c0, c1, c2}, []auth.Want{c1, c2}},
		{[]peer.ID{p0, p1}, []auth.Want{c0, c1, c2}, []auth.Want{}},
		// Both p0 and p2 received DONT_HAVE for c2
		{[]peer.ID{p0, p2}, []auth.Want{c0, c1, c2}, []auth.Want{c2}},
		{[]peer.ID{p0, p1, p2}, []auth.Want{c0, c1, c2}, []auth.Want{}},
	}

	for i, tc := range testcases {
		if !testutil.MatchWantsIgnoreOrder(
			bpm.AllPeersDoNotHaveBlock(tc.peers, tc.ks),
			tc.exp,
		) {
			t.Fatal(fmt.Sprintf("test case %d failed: expected matching keys", i))
		}
	}
}
