package session

import (
	"context"
	"sync"
	"testing"
	"time"

	peer "github.com/libp2p/go-libp2p-core/peer"
	bsbpm "github.com/peergos/go-bitswap-auth/internal/blockpresencemanager"
	bspm "github.com/peergos/go-bitswap-auth/internal/peermanager"
	bsspm "github.com/peergos/go-bitswap-auth/internal/sessionpeermanager"
	"github.com/peergos/go-bitswap-auth/internal/testutil"
        "github.com/peergos/go-bitswap-auth/auth"
)

type sentWants struct {
	sync.Mutex
	p          peer.ID
	wantHaves  *auth.Set
	wantBlocks *auth.Set
}

func (sw *sentWants) add(wantBlocks []auth.Want, wantHaves []auth.Want) {
	sw.Lock()
	defer sw.Unlock()

	for _, c := range wantBlocks {
		sw.wantBlocks.Add(c)
	}
	for _, c := range wantHaves {
		if !sw.wantBlocks.Has(c) {
			sw.wantHaves.Add(c)
		}
	}

}
func (sw *sentWants) wantHavesKeys() []auth.Want {
	sw.Lock()
	defer sw.Unlock()
	return sw.wantHaves.Keys()
}
func (sw *sentWants) wantBlocksKeys() []auth.Want {
	sw.Lock()
	defer sw.Unlock()
	return sw.wantBlocks.Keys()
}

type mockPeerManager struct {
	lk           sync.Mutex
	peerSessions map[peer.ID]bspm.Session
	peerSends    map[peer.ID]*sentWants
}

func newMockPeerManager() *mockPeerManager {
	return &mockPeerManager{
		peerSessions: make(map[peer.ID]bspm.Session),
		peerSends:    make(map[peer.ID]*sentWants),
	}
}

func (pm *mockPeerManager) RegisterSession(p peer.ID, sess bspm.Session) {
	pm.lk.Lock()
	defer pm.lk.Unlock()

	pm.peerSessions[p] = sess
}

func (pm *mockPeerManager) has(p peer.ID, sid uint64) bool {
	pm.lk.Lock()
	defer pm.lk.Unlock()

	if session, ok := pm.peerSessions[p]; ok {
		return session.ID() == sid
	}
	return false
}

func (*mockPeerManager) UnregisterSession(uint64)                      {}
func (*mockPeerManager) BroadcastWantHaves(context.Context, []auth.Want) {}
func (*mockPeerManager) SendCancels(context.Context, []auth.Want)        {}

func (pm *mockPeerManager) SendWants(ctx context.Context, p peer.ID, wantBlocks []auth.Want, wantHaves []auth.Want) {
	pm.lk.Lock()
	defer pm.lk.Unlock()

	sw, ok := pm.peerSends[p]
	if !ok {
		sw = &sentWants{p: p, wantHaves: auth.NewSet(), wantBlocks: auth.NewSet()}
		pm.peerSends[p] = sw
	}
	sw.add(wantBlocks, wantHaves)
}

func (pm *mockPeerManager) waitNextWants() map[peer.ID]*sentWants {
	time.Sleep(10 * time.Millisecond)

	pm.lk.Lock()
	defer pm.lk.Unlock()
	nw := make(map[peer.ID]*sentWants)
	for p, sentWants := range pm.peerSends {
		nw[p] = sentWants
	}
	return nw
}

func (pm *mockPeerManager) clearWants() {
	pm.lk.Lock()
	defer pm.lk.Unlock()

	for p := range pm.peerSends {
		delete(pm.peerSends, p)
	}
}

type exhaustedPeers struct {
	lk sync.Mutex
	ks []auth.Want
}

func (ep *exhaustedPeers) onPeersExhausted(ks []auth.Want) {
	ep.lk.Lock()
	defer ep.lk.Unlock()

	ep.ks = append(ep.ks, ks...)
}

func (ep *exhaustedPeers) clear() {
	ep.lk.Lock()
	defer ep.lk.Unlock()

	ep.ks = nil
}

func (ep *exhaustedPeers) exhausted() []auth.Want {
	ep.lk.Lock()
	defer ep.lk.Unlock()

	return append([]auth.Want{}, ep.ks...)
}

func TestSendWants(t *testing.T) {
	cids := testutil.GenerateWants(4)
	peers := testutil.GeneratePeers(1)
	peerA := peers[0]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// add cid0, cid1
	blkCids0 := cids[0:2]
	spm.Add(blkCids0)
	// peerA: HAVE cid0
	spm.Update(peerA, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	// Wait for processing to complete
	peerSends := pm.waitNextWants()

	// Should have sent
	// peerA: want-block cid0, cid1
	sw, ok := peerSends[peerA]
	if !ok {
		t.Fatal("Nothing sent to peer")
	}
	if !testutil.MatchWantsIgnoreOrder(sw.wantBlocksKeys(), blkCids0) {
		t.Fatal("Wrong keys")
	}
	if len(sw.wantHavesKeys()) > 0 {
		t.Fatal("Expecting no want-haves")
	}
}

func TestSendsWantBlockToOnePeerOnly(t *testing.T) {
	cids := testutil.GenerateWants(4)
	peers := testutil.GeneratePeers(2)
	peerA := peers[0]
	peerB := peers[1]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// add cid0, cid1
	blkCids0 := cids[0:2]
	spm.Add(blkCids0)
	// peerA: HAVE cid0
	spm.Update(peerA, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	// Wait for processing to complete
	peerSends := pm.waitNextWants()

	// Should have sent
	// peerA: want-block cid0, cid1
	sw, ok := peerSends[peerA]
	if !ok {
		t.Fatal("Nothing sent to peer")
	}
	if !testutil.MatchWantsIgnoreOrder(sw.wantBlocksKeys(), blkCids0) {
		t.Fatal("Wrong keys")
	}

	// Clear wants (makes keeping track of what's been sent easier)
	pm.clearWants()

	// peerB: HAVE cid0
	spm.Update(peerB, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	// Wait for processing to complete
	peerSends = pm.waitNextWants()

	// Have not received response from peerA, so should not send want-block to
	// peerB. Should have sent
	// peerB: want-have cid0, cid1
	sw, ok = peerSends[peerB]
	if !ok {
		t.Fatal("Nothing sent to peer")
	}
	if sw.wantBlocks.Len() > 0 {
		t.Fatal("Expecting no want-blocks")
	}
	if !testutil.MatchWantsIgnoreOrder(sw.wantHavesKeys(), blkCids0) {
		t.Fatal("Wrong keys")
	}
}

func TestReceiveBlock(t *testing.T) {
	cids := testutil.GenerateWants(2)
	peers := testutil.GeneratePeers(2)
	peerA := peers[0]
	peerB := peers[1]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// add cid0, cid1
	spm.Add(cids)
	// peerA: HAVE cid0
	spm.Update(peerA, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	// Wait for processing to complete
	peerSends := pm.waitNextWants()

	// Should have sent
	// peerA: want-block cid0, cid1
	sw, ok := peerSends[peerA]
	if !ok {
		t.Fatal("Nothing sent to peer")
	}
	if !testutil.MatchWantsIgnoreOrder(sw.wantBlocksKeys(), cids) {
		t.Fatal("Wrong keys")
	}

	// Clear wants (makes keeping track of what's been sent easier)
	pm.clearWants()

	// peerA: block cid0, DONT_HAVE cid1
	bpm.ReceiveFrom(peerA, []auth.Want{}, []auth.Want{cids[1]})
	spm.Update(peerA, []auth.Want{cids[0]}, []auth.Want{}, []auth.Want{cids[1]})
	// peerB: HAVE cid0, cid1
	bpm.ReceiveFrom(peerB, cids, []auth.Want{})
	spm.Update(peerB, []auth.Want{}, cids, []auth.Want{})

	// Wait for processing to complete
	peerSends = pm.waitNextWants()

	// Should have sent
	// peerB: want-block cid1
	// (should not have sent want-block for cid0 because block0 has already
	// been received)
	sw, ok = peerSends[peerB]
	if !ok {
		t.Fatal("Nothing sent to peer")
	}
	wb := sw.wantBlocksKeys()
	if len(wb) != 1 || !wb[0].Equals(cids[1]) {
		t.Fatal("Wrong keys", wb)
	}
}

func TestCancelWants(t *testing.T) {
	cids := testutil.GenerateWants(4)
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// add cid0, cid1, cid2
	blkCids := cids[0:3]
	spm.Add(blkCids)

	time.Sleep(5 * time.Millisecond)

	// cancel cid0, cid2
	cancelCids := []auth.Want{cids[0], cids[2]}
	spm.Cancel(cancelCids)

	// Wait for processing to complete
	time.Sleep(5 * time.Millisecond)

	// Should have sent cancels for cid0, cid2
	sent := swc.cancelled()
	if !testutil.MatchWantsIgnoreOrder(sent, cancelCids) {
		t.Fatal("Wrong keys")
	}
}

func TestRegisterSessionWithPeerManager(t *testing.T) {
	cids := testutil.GenerateWants(2)
	peers := testutil.GeneratePeers(2)
	peerA := peers[0]
	peerB := peers[1]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// peerA: HAVE cid0
	spm.Update(peerA, nil, cids[:1], nil)

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Expect session to have been registered with PeerManager
	if !pm.has(peerA, sid) {
		t.Fatal("Expected HAVE to register session with PeerManager")
	}

	// peerB: block cid1
	spm.Update(peerB, cids[1:], nil, nil)

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Expect session to have been registered with PeerManager
	if !pm.has(peerB, sid) {
		t.Fatal("Expected HAVE to register session with PeerManager")
	}
}

func TestProtectConnFirstPeerToSendWantedBlock(t *testing.T) {
	cids := testutil.GenerateWants(2)
	peers := testutil.GeneratePeers(3)
	peerA := peers[0]
	peerB := peers[1]
	peerC := peers[2]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpt := newFakePeerTagger()
	fpm := bsspm.New(1, fpt)
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// add cid0
	spm.Add(cids[:1])

	// peerA: block cid0
	spm.Update(peerA, cids[:1], nil, nil)

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Expect peer A to be protected as it was first to send the block
	if !fpt.isProtected(peerA) {
		t.Fatal("Expected first peer to send block to have protected connection")
	}

	// peerB: block cid0
	spm.Update(peerB, cids[:1], nil, nil)

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Expect peer B not to be protected as it was not first to send the block
	if fpt.isProtected(peerB) {
		t.Fatal("Expected peer not to be protected")
	}

	// peerC: block cid1
	spm.Update(peerC, cids[1:], nil, nil)

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Expect peer C not to be protected as we didn't want the block it sent
	if fpt.isProtected(peerC) {
		t.Fatal("Expected peer not to be protected")
	}
}

func TestPeerUnavailable(t *testing.T) {
	cids := testutil.GenerateWants(2)
	peers := testutil.GeneratePeers(2)
	peerA := peers[0]
	peerB := peers[1]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// add cid0, cid1
	spm.Add(cids)
	// peerA: HAVE cid0
	spm.Update(peerA, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	// Wait for processing to complete
	peerSends := pm.waitNextWants()

	// Should have sent
	// peerA: want-block cid0, cid1
	sw, ok := peerSends[peerA]
	if !ok {
		t.Fatal("Nothing sent to peer")
	}
	if !testutil.MatchWantsIgnoreOrder(sw.wantBlocksKeys(), cids) {
		t.Fatal("Wrong keys")
	}

	// Clear wants (makes keeping track of what's been sent easier)
	pm.clearWants()

	// peerB: HAVE cid0
	spm.Update(peerB, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	// Wait for processing to complete
	peerSends = pm.waitNextWants()

	// Should not have sent anything because want-blocks were already sent to
	// peer A
	sw, ok = peerSends[peerB]
	if ok && sw.wantBlocks.Len() > 0 {
		t.Fatal("Expected no wants sent to peer")
	}

	// peerA becomes unavailable
	spm.SignalAvailability(peerA, false)

	// Wait for processing to complete
	peerSends = pm.waitNextWants()

	// Should now have sent want-block cid0, cid1 to peerB
	sw, ok = peerSends[peerB]
	if !ok {
		t.Fatal("Nothing sent to peer")
	}
	if !testutil.MatchWantsIgnoreOrder(sw.wantBlocksKeys(), cids) {
		t.Fatal("Wrong keys")
	}
}

func TestPeersExhausted(t *testing.T) {
	cids := testutil.GenerateWants(3)
	peers := testutil.GeneratePeers(2)
	peerA := peers[0]
	peerB := peers[1]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}

	ep := exhaustedPeers{}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, ep.onPeersExhausted)

	go spm.Run()

	// add cid0, cid1
	spm.Add(cids)

	// peerA: HAVE cid0
	bpm.ReceiveFrom(peerA, []auth.Want{cids[0]}, []auth.Want{})
	// Note: this also registers peer A as being available
	spm.Update(peerA, []auth.Want{cids[0]}, []auth.Want{}, []auth.Want{})

	// peerA: DONT_HAVE cid1
	bpm.ReceiveFrom(peerA, []auth.Want{}, []auth.Want{cids[1]})
	spm.Update(peerA, []auth.Want{}, []auth.Want{}, []auth.Want{cids[1]})

	time.Sleep(5 * time.Millisecond)

	// All available peers (peer A) have sent us a DONT_HAVE for cid1,
	// so expect that onPeersExhausted() will be called with cid1
	if !testutil.MatchWantsIgnoreOrder(ep.exhausted(), []auth.Want{cids[1]}) {
		t.Fatal("Wrong keys")
	}

	// Clear exhausted cids
	ep.clear()

	// peerB: HAVE cid0
	bpm.ReceiveFrom(peerB, []auth.Want{cids[0]}, []auth.Want{})
	// Note: this also registers peer B as being available
	spm.Update(peerB, []auth.Want{cids[0]}, []auth.Want{}, []auth.Want{})

	// peerB: DONT_HAVE cid1, cid2
	bpm.ReceiveFrom(peerB, []auth.Want{}, []auth.Want{cids[1], cids[2]})
	spm.Update(peerB, []auth.Want{}, []auth.Want{}, []auth.Want{cids[1], cids[2]})

	// Wait for processing to complete
	pm.waitNextWants()

	// All available peers (peer A and peer B) have sent us a DONT_HAVE
	// for cid1, but we already called onPeersExhausted with cid1, so it
	// should not be called again
	if len(ep.exhausted()) > 0 {
		t.Fatal("Wrong keys")
	}

	// peerA: DONT_HAVE cid2
	bpm.ReceiveFrom(peerA, []auth.Want{}, []auth.Want{cids[2]})
	spm.Update(peerA, []auth.Want{}, []auth.Want{}, []auth.Want{cids[2]})

	// Wait for processing to complete
	pm.waitNextWants()

	// All available peers (peer A and peer B) have sent us a DONT_HAVE for
	// cid2, so expect that onPeersExhausted() will be called with cid2
	if !testutil.MatchWantsIgnoreOrder(ep.exhausted(), []auth.Want{cids[2]}) {
		t.Fatal("Wrong keys")
	}
}

// Tests that when
// - all the peers except one have sent a DONT_HAVE for a CID
// - the remaining peer becomes unavailable
// onPeersExhausted should be sent for that CID
func TestPeersExhaustedLastWaitingPeerUnavailable(t *testing.T) {
	cids := testutil.GenerateWants(2)
	peers := testutil.GeneratePeers(2)
	peerA := peers[0]
	peerB := peers[1]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}

	ep := exhaustedPeers{}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, ep.onPeersExhausted)

	go spm.Run()

	// add cid0, cid1
	spm.Add(cids)

	// peerA: HAVE cid0
	bpm.ReceiveFrom(peerA, []auth.Want{cids[0]}, []auth.Want{})
	// Note: this also registers peer A as being available
	spm.Update(peerA, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})
	// peerB: HAVE cid0
	bpm.ReceiveFrom(peerB, []auth.Want{cids[0]}, []auth.Want{})
	// Note: this also registers peer B as being available
	spm.Update(peerB, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	// peerA: DONT_HAVE cid1
	bpm.ReceiveFrom(peerA, []auth.Want{}, []auth.Want{cids[1]})
	spm.Update(peerA, []auth.Want{}, []auth.Want{}, []auth.Want{cids[0]})

	time.Sleep(5 * time.Millisecond)

	// peerB: becomes unavailable
	spm.SignalAvailability(peerB, false)

	time.Sleep(5 * time.Millisecond)

	// All remaining peers (peer A) have sent us a DONT_HAVE for cid1,
	// so expect that onPeersExhausted() will be called with cid1
	if !testutil.MatchWantsIgnoreOrder(ep.exhausted(), []auth.Want{cids[1]}) {
		t.Fatal("Wrong keys")
	}
}

// Tests that when all the peers are removed from the session
// onPeersExhausted should be called with all outstanding CIDs
func TestPeersExhaustedAllPeersUnavailable(t *testing.T) {
	cids := testutil.GenerateWants(3)
	peers := testutil.GeneratePeers(2)
	peerA := peers[0]
	peerB := peers[1]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}

	ep := exhaustedPeers{}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, ep.onPeersExhausted)

	go spm.Run()

	// add cid0, cid1, cid2
	spm.Add(cids)

	// peerA: receive block for cid0 (and register peer A with sessionWantSender)
	spm.Update(peerA, []auth.Want{cids[0]}, []auth.Want{}, []auth.Want{})
	// peerB: HAVE cid1
	bpm.ReceiveFrom(peerB, []auth.Want{cids[0]}, []auth.Want{})
	// Note: this also registers peer B as being available
	spm.Update(peerB, []auth.Want{}, []auth.Want{cids[0]}, []auth.Want{})

	time.Sleep(5 * time.Millisecond)

	// peerA and peerB: become unavailable
	spm.SignalAvailability(peerA, false)
	spm.SignalAvailability(peerB, false)

	time.Sleep(5 * time.Millisecond)

	// Expect that onPeersExhausted() will be called with all cids for blocks
	// that have not been received
	if !testutil.MatchWantsIgnoreOrder(ep.exhausted(), []auth.Want{cids[1], cids[2]}) {
		t.Fatal("Wrong keys")
	}
}

func TestConsecutiveDontHaveLimit(t *testing.T) {
	cids := testutil.GenerateWants(peerDontHaveLimit + 10)
	p := testutil.GeneratePeers(1)[0]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// Add all cids as wants
	spm.Add(cids)

	// Receive a block from peer (adds it to the session)
	spm.Update(p, cids[:1], []auth.Want{}, []auth.Want{})

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}

	// Receive DONT_HAVEs from peer that do not exceed limit
	for _, c := range cids[1:peerDontHaveLimit] {
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}

	// Wait for processing to complete
	time.Sleep(20 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}

	// Receive DONT_HAVEs from peer that exceed limit
	for _, c := range cids[peerDontHaveLimit:] {
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}

	// Wait for processing to complete
	time.Sleep(20 * time.Millisecond)

	// Session should remove peer
	if has := fpm.HasPeer(p); has {
		t.Fatal("Expected peer not to be available")
	}
}

func TestConsecutiveDontHaveLimitInterrupted(t *testing.T) {
	cids := testutil.GenerateWants(peerDontHaveLimit + 10)
	p := testutil.GeneratePeers(1)[0]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// Add all cids as wants
	spm.Add(cids)

	// Receive a block from peer (adds it to the session)
	spm.Update(p, cids[:1], []auth.Want{}, []auth.Want{})

	// Wait for processing to complete
	time.Sleep(5 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}

	// Receive DONT_HAVE then HAVE then DONT_HAVE from peer,
	// where consecutive DONT_HAVEs would have exceeded limit
	// (but they are not consecutive)
	for _, c := range cids[1:peerDontHaveLimit] {
		// DONT_HAVEs
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}
	for _, c := range cids[peerDontHaveLimit : peerDontHaveLimit+1] {
		// HAVEs
		bpm.ReceiveFrom(p, []auth.Want{c}, []auth.Want{})
		spm.Update(p, []auth.Want{}, []auth.Want{c}, []auth.Want{})
	}
	for _, c := range cids[peerDontHaveLimit+1:] {
		// DONT_HAVEs
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}

	// Wait for processing to complete
	time.Sleep(5 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}
}

func TestConsecutiveDontHaveReinstateAfterRemoval(t *testing.T) {
	cids := testutil.GenerateWants(peerDontHaveLimit + 10)
	p := testutil.GeneratePeers(1)[0]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// Add all cids as wants
	spm.Add(cids)

	// Receive a block from peer (adds it to the session)
	spm.Update(p, cids[:1], []auth.Want{}, []auth.Want{})

	// Wait for processing to complete
	time.Sleep(5 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}

	// Receive DONT_HAVEs from peer that exceed limit
	for _, c := range cids[1 : peerDontHaveLimit+2] {
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Session should remove peer
	if has := fpm.HasPeer(p); has {
		t.Fatal("Expected peer not to be available")
	}

	// Receive a HAVE from peer (adds it back into the session)
	bpm.ReceiveFrom(p, cids[:1], []auth.Want{})
	spm.Update(p, []auth.Want{}, cids[:1], []auth.Want{})

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}

	cids2 := testutil.GenerateWants(peerDontHaveLimit + 10)

	// Receive DONT_HAVEs from peer that don't exceed limit
	for _, c := range cids2[1:peerDontHaveLimit] {
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}

	// Receive DONT_HAVEs from peer that exceed limit
	for _, c := range cids2[peerDontHaveLimit:] {
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Session should remove peer
	if has := fpm.HasPeer(p); has {
		t.Fatal("Expected peer not to be available")
	}
}

func TestConsecutiveDontHaveDontRemoveIfHasWantedBlock(t *testing.T) {
	cids := testutil.GenerateWants(peerDontHaveLimit + 10)
	p := testutil.GeneratePeers(1)[0]
	sid := uint64(1)
	pm := newMockPeerManager()
	fpm := newFakeSessionPeerManager()
	swc := newMockSessionMgr()
	bpm := bsbpm.New()
	onSend := func(peer.ID, []auth.Want, []auth.Want) {}
	onPeersExhausted := func([]auth.Want) {}
	spm := newSessionWantSender(sid, pm, fpm, swc, bpm, onSend, onPeersExhausted)
	defer spm.Shutdown()

	go spm.Run()

	// Add all cids as wants
	spm.Add(cids)

	// Receive a HAVE from peer (adds it to the session)
	bpm.ReceiveFrom(p, cids[:1], []auth.Want{})
	spm.Update(p, []auth.Want{}, cids[:1], []auth.Want{})

	// Wait for processing to complete
	time.Sleep(10 * time.Millisecond)

	// Peer should be available
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}

	// Receive DONT_HAVEs from peer that exceed limit
	for _, c := range cids[1 : peerDontHaveLimit+5] {
		bpm.ReceiveFrom(p, []auth.Want{}, []auth.Want{c})
		spm.Update(p, []auth.Want{}, []auth.Want{}, []auth.Want{c})
	}

	// Wait for processing to complete
	time.Sleep(20 * time.Millisecond)

	// Peer should still be available because it has a block that we want.
	// (We received a HAVE for cid 0 but didn't yet receive the block)
	if has := fpm.HasPeer(p); !has {
		t.Fatal("Expected peer to be available")
	}
}
