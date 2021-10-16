package session

import cid "github.com/ipfs/go-cid"
import "github.com/peergos/go-bitswap-auth/auth"

type cidQueue struct {
	elems []auth.Want
	eset  *cid.Set
}

func newCidQueue() *cidQueue {
	return &cidQueue{eset: cid.NewSet()}
}

func (cq *cidQueue) Pop() auth.Want {
	for {
		if len(cq.elems) == 0 {
			return auth.Want{}
		}

		out := cq.elems[0]
		cq.elems = cq.elems[1:]

		if cq.eset.Has(out.Cid) {
			cq.eset.Remove(out.Cid)
			return out
		}
	}
}

func (cq *cidQueue) Cids() []auth.Want {
	// Lazily delete from the list any cids that were removed from the set
	if len(cq.elems) > cq.eset.Len() {
		i := 0
		for _, c := range cq.elems {
			if cq.eset.Has(c.Cid) {
				cq.elems[i] = c
				i++
			}
		}
		cq.elems = cq.elems[:i]
	}

	// Make a copy of the cids
	return append([]auth.Want{}, cq.elems...)
}

func (cq *cidQueue) Push(c auth.Want) {
	if cq.eset.Visit(c.Cid) {
		cq.elems = append(cq.elems, c)
	}
}

func (cq *cidQueue) Remove(c cid.Cid) {
	cq.eset.Remove(c)
}

func (cq *cidQueue) Has(c cid.Cid) bool {
	return cq.eset.Has(c)
}

func (cq *cidQueue) Len() int {
	return cq.eset.Len()
}
