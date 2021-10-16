package decision

import (
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/peergos/go-bitswap-auth/auth"
	pb "github.com/peergos/go-bitswap-auth/message/pb"
	wl "github.com/peergos/go-bitswap-auth/wantlist"
)

func newLedger(p peer.ID) *ledger {
	return &ledger{
		wantList: wl.New(),
		Partner:  p,
	}
}

// Keeps the wantlist for the partner. NOT threadsafe!
type ledger struct {
	// Partner is the remote Peer.
	Partner peer.ID

	// wantList is a (bounded, small) set of keys that Partner desires.
	wantList *wl.Wantlist

	lk sync.RWMutex
}

func (l *ledger) Wants(k auth.Want, priority int32, wantType pb.Message_Wantlist_WantType) {
	log.Debugf("peer %s wants %s", l.Partner, k)
	l.wantList.Add(k, priority, wantType)
}

func (l *ledger) CancelWant(k auth.Want) bool {
	return l.wantList.Remove(k)
}

func (l *ledger) WantListContains(k auth.Want) (wl.Entry, bool) {
	return l.wantList.Contains(k)
}

func (l *ledger) Entries() []wl.Entry {
	return l.wantList.Entries()
}
