package decision

import (
	"context"
	"fmt"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-metrics-interface"
	process "github.com/jbenet/goprocess"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/peergos/go-bitswap-auth/auth"
)

// blockstoreManager maintains a pool of workers that make requests to the blockstore.
type blockstoreManager struct {
	bs           auth.AuthBlockstore
	workerCount  int
	jobs         chan func()
	px           process.Process
	pendingGauge metrics.Gauge
	activeGauge  metrics.Gauge
}

// newBlockstoreManager creates a new blockstoreManager with the given context
// and number of workers
func newBlockstoreManager(
	ctx context.Context,
	bs auth.AuthBlockstore,
	workerCount int,
	pendingGauge metrics.Gauge,
	activeGauge metrics.Gauge,
) *blockstoreManager {
	return &blockstoreManager{
		bs:           bs,
		workerCount:  workerCount,
		jobs:         make(chan func()),
		px:           process.WithTeardown(func() error { return nil }),
		pendingGauge: pendingGauge,
		activeGauge:  activeGauge,
	}
}

func (bsm *blockstoreManager) start(px process.Process) {
	px.AddChild(bsm.px)
	// Start up workers
	for i := 0; i < bsm.workerCount; i++ {
		bsm.px.Go(func(px process.Process) {
			bsm.worker(px)
		})
	}
}

func (bsm *blockstoreManager) worker(px process.Process) {
	for {
		select {
		case <-px.Closing():
			return
		case job := <-bsm.jobs:
			bsm.pendingGauge.Dec()
			bsm.activeGauge.Inc()
			job()
			bsm.activeGauge.Dec()
		}
	}
}

func (bsm *blockstoreManager) addJob(ctx context.Context, job func()) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-bsm.px.Closing():
		return fmt.Errorf("shutting down")
	case bsm.jobs <- job:
		bsm.pendingGauge.Inc()
		return nil
	}
}

func (bsm *blockstoreManager) getBlockSizes(ctx context.Context, ks []cid.Cid, auths []string, remote peer.ID) (map[cid.Cid]int, error) {
	res := make(map[cid.Cid]int)
	if len(ks) == 0 {
		return res, nil
	}

	var lk sync.Mutex
	return res, bsm.jobPerKey(ctx, ks, auths, remote, func(c cid.Cid, remote peer.ID, a string) {
		size, err := bsm.bs.GetSize(c)
		if err != nil {
			if err != bstore.ErrNotFound && err != auth.ErrUnauthorised {
				// Note: this isn't a fatal error. We shouldn't abort the request
				log.Errorf("blockstore.GetSize(%s) error: %s", c, err)
			}
		} else {
			lk.Lock()
			res[c] = size
			lk.Unlock()
		}
	})
}

func (bsm *blockstoreManager) getBlocks(ctx context.Context, ks []cid.Cid, auths []string, remote peer.ID) (map[cid.Cid]blocks.Block, error) {
	res := make(map[cid.Cid]blocks.Block)
	if len(ks) == 0 {
		return res, nil
	}

	var lk sync.Mutex
	return res, bsm.jobPerKey(ctx, ks, auths, remote, func(c cid.Cid, remote peer.ID, a string) {
		blk, err := bsm.bs.Get(c, remote, a)
		if err != nil {
			if err != bstore.ErrNotFound && err != auth.ErrUnauthorised {
				// Note: this isn't a fatal error. We shouldn't abort the request
				log.Errorf("blockstore.Get(%s) error: %s", c, err)
			}
		} else {
			lk.Lock()
			res[c] = blk
			lk.Unlock()
		}
	})
}

func (bsm *blockstoreManager) jobPerKey(ctx context.Context, ks []cid.Cid, auth []string, remote peer.ID, jobFn func(c cid.Cid, remote peer.ID, auth string)) error {
	var err error
	wg := sync.WaitGroup{}
	for i, k := range ks {
		c := k
		a := auth[i]
		wg.Add(1)
		err = bsm.addJob(ctx, func() {
			jobFn(c, remote, a)
			wg.Done()
		})
		if err != nil {
			wg.Done()
			break
		}
	}
	wg.Wait()
	return err
}
