package auth

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/peergos/go-ipfs-blockstore"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

var ErrUnauthorised = errors.New("Unauthorised")

type AuthBlockstore interface {
	DeleteBlock(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Get(cid.Cid, peer.ID, string) (blocks.Block, error)
	BloomAdd(cid.Cid) error

	// GetSize returns the CIDs mapped BlockSize
	GetSize(cid.Cid) (int, error)

	// Put puts a given block to the underlying datastore
	Put(AuthBlock) error

	// PutMany puts a slice of blocks at the same time using batching
	// capabilities of the underlying datastore whenever possible.
	PutMany([]AuthBlock) error

	// AllKeysChan returns a channel from which
	// the CIDs in the Blockstore can be read. It should respect
	// the given context, closing the channel if it becomes Done.
	AllKeysChan(ctx context.Context) (<-chan cid.Cid, error)

	// HashOnRead specifies if every read block should be
	// rehashed to make sure it matches its CID.
	HashOnRead(enabled bool)
}

type AuthedBlockstore struct {
	source blockstore.Blockstore
	allow  func(cid.Cid, []byte, peer.ID, string) bool
	AuthBlockstore
}

func NewAuthBlockstore(bstore blockstore.Blockstore, allow func(cid.Cid, []byte, peer.ID, string) bool) AuthBlockstore {
	return &AuthedBlockstore{source: bstore, allow: allow}
}

func (bs *AuthedBlockstore) Get(c cid.Cid, remote peer.ID, auth string) (blocks.Block, error) {
        local, err := bs.source.Has(c)
        if err != nil {
           return nil, err
        }
        if !local {
           return nil, blockstore.ErrNotFound
        }
        block, err := bs.source.Get(c)
        if err != nil {
           return nil, ErrUnauthorised
        }
	if bs.allow(c, block.RawData(), remote, auth) {
		return block, nil
	}
	return nil, ErrUnauthorised
}

func (bs *AuthedBlockstore) DeleteBlock(c cid.Cid) error {
	return bs.source.DeleteBlock(c)
}

func (bs *AuthedBlockstore) BloomAdd(c cid.Cid) error {
	return bs.source.BloomAdd(c)
}

func (bs *AuthedBlockstore) Has(c cid.Cid) (bool, error) {
	codec := c.Type()
	if codec != cid.Raw && codec != cid.DagCBOR {
		return false, nil
	}
	return bs.source.Has(c)
}

func (bs *AuthedBlockstore) GetSize(c cid.Cid) (int, error) {
	return bs.source.GetSize(c)
}

func (bs *AuthedBlockstore) Put(b AuthBlock) error {
	return bs.source.Put(b.(AuthBlockImpl).block)
}

func (bs *AuthedBlockstore) PutMany(authblocks []AuthBlock) error {
	unwrapped := make([]blocks.Block, len(authblocks))
	for i, b := range authblocks {
		unwrapped[i] = b.(AuthBlockImpl).block
	}
	return bs.source.PutMany(unwrapped)
}

func (bs *AuthedBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return bs.source.AllKeysChan(ctx)
}

func (bs *AuthedBlockstore) HashOnRead(enabled bool) {
	bs.source.HashOnRead(enabled)
}
