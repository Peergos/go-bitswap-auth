package auth

import (
	"context"
	"errors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

var ErrUnauthorised = errors.New("Unauthorised")

type AuthBlockstore interface {
	DeleteBlock(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Get(cid.Cid, peer.ID, string) (blocks.Block, error)

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

var Undef = Want{}

type Want struct {
	Cid  cid.Cid
	Auth string
}

func (w Want) Defined() bool {
	return w.Cid.Defined()
}

type AuthBlock struct {
	Want  Want
	block blocks.Block
}

func (a AuthBlock) Cid() cid.Cid {
	return a.block.Cid()
}

func (a AuthBlock) Size() int {
	return len(a.block.RawData())
}

func (a AuthBlock) Loggable() map[string]interface{} {
	return map[string]interface{}{}
}

func NewBlock(block blocks.Block) AuthBlock {
	return AuthBlock{block: block}
}

type AuthedBlockstore struct {
	source blockstore.Blockstore
	allow  func(cid.Cid, peer.ID, string) bool
	AuthBlockstore
}

func NewAuthBlockstore(bstore blockstore.Blockstore, allow func(cid.Cid, peer.ID, string) bool) AuthBlockstore {
	return &AuthedBlockstore{source: bstore, allow: allow}
}

func (bs *AuthedBlockstore) Get(c cid.Cid, remote peer.ID, auth string) (blocks.Block, error) {
	if bs.allow(c, remote, auth) {
		return bs.source.Get(c)
	}
	return nil, ErrUnauthorised
}

func (bs *AuthedBlockstore) DeleteBlock(c cid.Cid) error {
	return bs.source.DeleteBlock(c)
}

func (bs *AuthedBlockstore) Has(c cid.Cid) (bool, error) {
	return bs.source.Has(c)
}

func (bs *AuthedBlockstore) GetSize(c cid.Cid) (int, error) {
	return bs.source.GetSize(c)
}

func (bs *AuthedBlockstore) Put(b AuthBlock) error {
	return bs.source.Put(b.block)
}

func (bs *AuthedBlockstore) PutMany(authblocks []AuthBlock) error {
	unwrapped := make([]blocks.Block, len(authblocks))
	for i, b := range authblocks {
		unwrapped[i] = b.block
	}
	return bs.source.PutMany(unwrapped)
}

func (bs *AuthedBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return bs.source.AllKeysChan(ctx)
}

func (bs *AuthedBlockstore) HashOnRead(enabled bool) {
	bs.source.HashOnRead(enabled)
}
