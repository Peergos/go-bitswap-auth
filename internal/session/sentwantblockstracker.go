package session

import (
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/peergos/go-bitswap-auth/auth"
)

// sentWantBlocksTracker keeps track of which peers we've sent a want-block to
type sentWantBlocksTracker struct {
	sentWantBlocks map[peer.ID]map[auth.Want]struct{}
}

func newSentWantBlocksTracker() *sentWantBlocksTracker {
	return &sentWantBlocksTracker{
		sentWantBlocks: make(map[peer.ID]map[auth.Want]struct{}),
	}
}

func (s *sentWantBlocksTracker) addSentWantBlocksTo(p peer.ID, ks []auth.Want) {
	cids, ok := s.sentWantBlocks[p]
	if !ok {
		cids = make(map[auth.Want]struct{}, len(ks))
		s.sentWantBlocks[p] = cids
	}
	for _, c := range ks {
		cids[c] = struct{}{}
	}
}

func (s *sentWantBlocksTracker) haveSentWantBlockTo(p peer.ID, w auth.Want) bool {
	_, ok := s.sentWantBlocks[p][w]
	return ok
}
