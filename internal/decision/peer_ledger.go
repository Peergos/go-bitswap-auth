package decision

import (
"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/peergos/go-bitswap-auth/auth"
)

type peerLedger struct {
	cids map[auth.Want]map[peer.ID]struct{}
}

func newPeerLedger() *peerLedger {
fmt.Println("peer_ledger.newPeerLedger")
	return &peerLedger{cids: make(map[auth.Want]map[peer.ID]struct{})}
}

func (l *peerLedger) Wants(p peer.ID, k auth.Want) {
fmt.Println("peer_ledger.Wants", p, k)
	m, ok := l.cids[k]
	if !ok {
		m = make(map[peer.ID]struct{})
		l.cids[k] = m
	}
	m[p] = struct{}{}
        fmt.Println("**** peer_ledger.Wants-end", l.cids)
}

func (l *peerLedger) CancelWant(p peer.ID, k auth.Want) {
fmt.Println("**** peer_ledger.CancelWant", k)
	m, ok := l.cids[k]
	if !ok {
		return
	}
	delete(m, p)
	if len(m) == 0 {
		delete(l.cids, k)
	}
}

func (l *peerLedger) Peers(k auth.Want) []peer.ID {
fmt.Println("peer_ledger.Peers", k)
fmt.Println("**** peer_ledger.cids", l.cids)
	m, ok := l.cids[k]
	if !ok {
		return nil
	}
	peers := make([]peer.ID, 0, len(m))
	for p := range m {
		peers = append(peers, p)
	}
        fmt.Println("peer_ledger.Peers =>", peers)
	return peers
}
