package auth

import (
	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

var Undef = Want{}

func NewWant(c cid.Cid, a string) Want {
	return Want{Cid: c, Auth: a}
}

type Want struct {
	Cid  cid.Cid
	Auth string
}

func (w Want) Defined() bool {
	return w.Cid.Defined()
}

func (w Want) Equals(b Want) bool {
	return w.Cid.Equals(b.Cid) && w.Auth == b.Auth
}

func NewBlock(block blocks.Block, a string) AuthBlock {
	return AuthBlockImpl{block: block, want: NewWant(block.Cid(), a)}
}

type AuthBlock interface {
	Want() Want
	Cid() cid.Cid
	Size() int
	GetAuthedData() []byte
	Loggable() map[string]interface{}
}

type AuthBlockImpl struct {
	want  Want
	block blocks.Block
}

func (a AuthBlockImpl) Cid() cid.Cid {
	return a.block.Cid()
}

func (a AuthBlockImpl) Want() Want {
	return a.want
}

func (a AuthBlockImpl) Size() int {
	return len(a.block.RawData())
}

func (a AuthBlockImpl) GetAuthedData() []byte {
	return a.block.RawData()
}

func (a AuthBlockImpl) Loggable() map[string]interface{} {
	return map[string]interface{}{}
}