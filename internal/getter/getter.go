package getter

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/peergos/go-bitswap-auth/auth"
	notifications "github.com/peergos/go-bitswap-auth/internal/notifications"
)

var log = logging.Logger("bitswap")

// GetBlocksFunc is any function that can take an array of CIDs and return a
// channel of incoming blocks.
type GetBlocksFunc func(context.Context, []auth.Want) (<-chan blocks.Block, error)

// SyncGetBlock takes a block cid and an async function for getting several
// blocks that returns a channel, and uses that function to return the
// block syncronously.
func SyncGetBlock(p context.Context, w auth.Want, gb GetBlocksFunc) (blocks.Block, error) {
	if !w.Cid.Defined() {
		log.Error("undefined cid in GetBlock")
		return nil, blockstore.ErrNotFound
	}

	// Any async work initiated by this function must end when this function
	// returns. To ensure this, derive a new context. Note that it is okay to
	// listen on parent in this scope, but NOT okay to pass |parent| to
	// functions called by this one. Otherwise those functions won't return
	// when this context's cancel func is executed. This is difficult to
	// enforce. May this comment keep you safe.
	ctx, cancel := context.WithCancel(p)
	defer cancel()

	promise, err := gb(ctx, []auth.Want{w})
	if err != nil {
		return nil, err
	}

	select {
	case block, ok := <-promise:
		if !ok {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return nil, errors.New("promise channel was closed")
			}
		}
		return block, nil
	case <-p.Done():
		return nil, p.Err()
	}
}

// WantFunc is any function that can express a want for set of blocks.
type WantFunc func(context.Context, []auth.Want)

// AsyncGetBlocks take a set of block cids, a pubsub channel for incoming
// blocks, a want function, and a close function, and returns a channel of
// incoming blocks.
func AsyncGetBlocks(ctx context.Context, sessctx context.Context, wants []auth.Want, notif notifications.PubSub,
	want WantFunc, cwants func([]cid.Cid)) (<-chan blocks.Block, error) {

	// If there are no keys supplied, just return a closed channel
	if len(wants) == 0 {
		out := make(chan blocks.Block)
		close(out)
		return out, nil
	}

	// Use a PubSub notifier to listen for incoming blocks for each key
	remaining := cid.NewSet()
	cids := make([]cid.Cid, len(wants))
	for i, w := range wants {
		cids[i] = w.Cid
	}
	promise := notif.Subscribe(ctx, cids...)
	for _, w := range wants {
		log.Debugw("Bitswap.GetBlockRequest.Start", "cid", w.Cid)
		remaining.Add(w.Cid)
	}

	// Send the want request for the keys to the network
	want(ctx, wants)

	out := make(chan blocks.Block)
	go handleIncoming(ctx, sessctx, remaining, promise, out, cwants)
	return out, nil
}

// Listens for incoming blocks, passing them to the out channel.
// If the context is cancelled or the incoming channel closes, calls cfun with
// any keys corresponding to blocks that were never received.
func handleIncoming(ctx context.Context, sessctx context.Context, remaining *cid.Set,
	in <-chan blocks.Block, out chan blocks.Block, cfun func([]cid.Cid)) {

	ctx, cancel := context.WithCancel(ctx)

	// Clean up before exiting this function, and call the cancel function on
	// any remaining keys
	defer func() {
		cancel()
		close(out)
		// can't just defer this call on its own, arguments are resolved *when* the defer is created
		cfun(remaining.Keys())
	}()

	for {
		select {
		case blk, ok := <-in:
			// If the channel is closed, we're done (note that PubSub closes
			// the channel once all the keys have been received)
			if !ok {
				return
			}

			remaining.Remove(blk.Cid())
			select {
			case out <- blk:
			case <-ctx.Done():
				return
			case <-sessctx.Done():
				return
			}
		case <-ctx.Done():
			return
		case <-sessctx.Done():
			return
		}
	}
}
