package sessionmanager

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	delay "github.com/ipfs/go-ipfs-delay"

	bsbpm "github.com/peergos/go-bitswap-auth/internal/blockpresencemanager"
	notifications "github.com/peergos/go-bitswap-auth/internal/notifications"
	bspm "github.com/peergos/go-bitswap-auth/internal/peermanager"
	bssession "github.com/peergos/go-bitswap-auth/internal/session"
	bssim "github.com/peergos/go-bitswap-auth/internal/sessioninterestmanager"
	"github.com/peergos/go-bitswap-auth/internal/testutil"
        "github.com/peergos/go-bitswap-auth/auth"

	blocks "github.com/ipfs/go-block-format"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

type fakeSession struct {
	ks         []auth.Want
	wantBlocks []auth.Want
	wantHaves  []auth.Want
	id         uint64
	pm         *fakeSesPeerManager
	sm         bssession.SessionManager
	notif      notifications.PubSub
}

func (*fakeSession) GetBlock(context.Context, auth.Want) (auth.AuthBlock, error) {
	return nil, nil
}
func (*fakeSession) GetBlocks(context.Context, []auth.Want) (<-chan auth.AuthBlock, error) {
	return nil, nil
}
func (fs *fakeSession) ID() uint64 {
	return fs.id
}
func (fs *fakeSession) ReceiveFrom(p peer.ID, ks []auth.Want, wantBlocks []auth.Want, wantHaves []auth.Want) {
	fs.ks = append(fs.ks, ks...)
	fs.wantBlocks = append(fs.wantBlocks, wantBlocks...)
	fs.wantHaves = append(fs.wantHaves, wantHaves...)
}
func (fs *fakeSession) Shutdown() {
	fs.sm.RemoveSession(fs.id)
}

type fakeSesPeerManager struct {
}

func (*fakeSesPeerManager) Peers() []peer.ID          { return nil }
func (*fakeSesPeerManager) PeersDiscovered() bool     { return false }
func (*fakeSesPeerManager) Shutdown()                 {}
func (*fakeSesPeerManager) AddPeer(peer.ID) bool      { return false }
func (*fakeSesPeerManager) RemovePeer(peer.ID) bool   { return false }
func (*fakeSesPeerManager) HasPeers() bool            { return false }
func (*fakeSesPeerManager) ProtectConnection(peer.ID) {}

type fakePeerManager struct {
	lk      sync.Mutex
	cancels []auth.Want
}

func (*fakePeerManager) RegisterSession(peer.ID, bspm.Session)                    {}
func (*fakePeerManager) UnregisterSession(uint64)                                 {}
func (*fakePeerManager) SendWants(context.Context, peer.ID, []auth.Want, []auth.Want) {}
func (*fakePeerManager) BroadcastWantHaves(context.Context, []auth.Want)            {}
func (fpm *fakePeerManager) SendCancels(ctx context.Context, cancels []auth.Want) {
	fpm.lk.Lock()
	defer fpm.lk.Unlock()
	fpm.cancels = append(fpm.cancels, cancels...)
}
func (fpm *fakePeerManager) cancelled() []auth.Want {
	fpm.lk.Lock()
	defer fpm.lk.Unlock()
	return fpm.cancels
}

func sessionFactory(ctx context.Context,
	sm bssession.SessionManager,
	id uint64,
	sprm bssession.SessionPeerManager,
	sim *bssim.SessionInterestManager,
	pm bssession.PeerManager,
	bpm *bsbpm.BlockPresenceManager,
	notif notifications.PubSub,
	provSearchDelay time.Duration,
	rebroadcastDelay delay.D,
	self peer.ID) Session {
	fs := &fakeSession{
		id:    id,
		pm:    sprm.(*fakeSesPeerManager),
		sm:    sm,
		notif: notif,
	}
	go func() {
		<-ctx.Done()
		sm.RemoveSession(fs.id)
	}()
	return fs
}

func peerManagerFactory(ctx context.Context, id uint64) bssession.SessionPeerManager {
	return &fakeSesPeerManager{}
}

func TestReceiveFrom(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	notif := notifications.New()
	defer notif.Shutdown()
	sim := bssim.New()
	bpm := bsbpm.New()
	pm := &fakePeerManager{}
	sm := New(ctx, sessionFactory, sim, peerManagerFactory, bpm, pm, notif, "")

	p := peer.ID(fmt.Sprint(123))
	block := blocks.NewBlock([]byte("block"))

	firstSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)
	secondSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)
	thirdSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)

        w := auth.NewWant(block.Cid(), "auth")
	sim.RecordSessionInterest(firstSession.ID(), []auth.Want{w})
	sim.RecordSessionInterest(thirdSession.ID(), []auth.Want{w})

	sm.ReceiveFrom(ctx, p, []auth.Want{w}, []auth.Want{}, []auth.Want{})
	if len(firstSession.ks) == 0 ||
		len(secondSession.ks) > 0 ||
		len(thirdSession.ks) == 0 {
		t.Fatal("should have received blocks but didn't")
	}

	sm.ReceiveFrom(ctx, p, []auth.Want{}, []auth.Want{w}, []auth.Want{})
	if len(firstSession.wantBlocks) == 0 ||
		len(secondSession.wantBlocks) > 0 ||
		len(thirdSession.wantBlocks) == 0 {
		t.Fatal("should have received want-blocks but didn't")
	}

	sm.ReceiveFrom(ctx, p, []auth.Want{}, []auth.Want{}, []auth.Want{w})
	if len(firstSession.wantHaves) == 0 ||
		len(secondSession.wantHaves) > 0 ||
		len(thirdSession.wantHaves) == 0 {
		t.Fatal("should have received want-haves but didn't")
	}

	if len(pm.cancelled()) != 1 {
		t.Fatal("should have sent cancel for received blocks")
	}
}

func TestReceiveBlocksWhenManagerShutdown(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	notif := notifications.New()
	defer notif.Shutdown()
	sim := bssim.New()
	bpm := bsbpm.New()
	pm := &fakePeerManager{}
	sm := New(ctx, sessionFactory, sim, peerManagerFactory, bpm, pm, notif, "")

	p := peer.ID(fmt.Sprint(123))
	block := blocks.NewBlock([]byte("block"))

	firstSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)
	secondSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)
	thirdSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)

	w := auth.NewWant(block.Cid(), "auth")
	sim.RecordSessionInterest(firstSession.ID(), []auth.Want{w})
	sim.RecordSessionInterest(secondSession.ID(), []auth.Want{w})
	sim.RecordSessionInterest(thirdSession.ID(), []auth.Want{w})

	sm.Shutdown()

	// wait for sessions to get removed
	time.Sleep(10 * time.Millisecond)

	sm.ReceiveFrom(ctx, p, []auth.Want{w}, []auth.Want{}, []auth.Want{})
	if len(firstSession.ks) > 0 ||
		len(secondSession.ks) > 0 ||
		len(thirdSession.ks) > 0 {
		t.Fatal("received blocks for sessions after manager is shutdown")
	}
}

func TestReceiveBlocksWhenSessionContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notif := notifications.New()
	defer notif.Shutdown()
	sim := bssim.New()
	bpm := bsbpm.New()
	pm := &fakePeerManager{}
	sm := New(ctx, sessionFactory, sim, peerManagerFactory, bpm, pm, notif, "")

	p := peer.ID(fmt.Sprint(123))
	block := blocks.NewBlock([]byte("block"))

	firstSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	secondSession := sm.NewSession(sessionCtx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)
	thirdSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)

	w := auth.NewWant(block.Cid(), "auth")
	sim.RecordSessionInterest(firstSession.ID(), []auth.Want{w})
	sim.RecordSessionInterest(secondSession.ID(), []auth.Want{w})
	sim.RecordSessionInterest(thirdSession.ID(), []auth.Want{w})

	sessionCancel()

	// wait for sessions to get removed
	time.Sleep(10 * time.Millisecond)

	sm.ReceiveFrom(ctx, p, []auth.Want{w}, []auth.Want{}, []auth.Want{})
	if len(firstSession.ks) == 0 ||
		len(secondSession.ks) > 0 ||
		len(thirdSession.ks) == 0 {
		t.Fatal("received blocks for sessions that are canceled")
	}
}

func TestShutdown(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	notif := notifications.New()
	defer notif.Shutdown()
	sim := bssim.New()
	bpm := bsbpm.New()
	pm := &fakePeerManager{}
	sm := New(ctx, sessionFactory, sim, peerManagerFactory, bpm, pm, notif, "")

	p := peer.ID(fmt.Sprint(123))
	block := blocks.NewBlock([]byte("block"))
	w := auth.NewWant(block.Cid(), "auth")
	cids := []auth.Want{w}
	firstSession := sm.NewSession(ctx, time.Second, delay.Fixed(time.Minute)).(*fakeSession)
	sim.RecordSessionInterest(firstSession.ID(), cids)
	sm.ReceiveFrom(ctx, p, []auth.Want{}, []auth.Want{}, cids)

	if !bpm.HasKey(block.Cid()) {
		t.Fatal("expected cid to be added to block presence manager")
	}

	sm.Shutdown()

	// wait for cleanup
	time.Sleep(10 * time.Millisecond)

	if bpm.HasKey(block.Cid()) {
		t.Fatal("expected cid to be removed from block presence manager")
	}
	if !testutil.MatchWantsIgnoreOrder(pm.cancelled(), cids) {
		t.Fatal("expected cancels to be sent")
	}
}
