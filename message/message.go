package message

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	pb "github.com/peergos/go-bitswap-auth/message/pb"
	"github.com/peergos/go-bitswap-auth/wantlist"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	u "github.com/ipfs/go-ipfs-util"
	pool "github.com/libp2p/go-buffer-pool"
	"github.com/libp2p/go-libp2p-core/network"
	msgio "github.com/libp2p/go-msgio"
	"github.com/peergos/go-bitswap-auth/auth"
)

// BitSwapMessage is the basic interface for interacting building, encoding,
// and decoding messages sent on the BitSwap protocol.
type BitSwapMessage interface {
	// Wantlist returns a slice of unique keys that represent data wanted by
	// the sender.
	Wantlist() []Entry

	// Blocks returns a slice of unique blocks.
	Blocks() []auth.AuthBlock
	// BlockPresences returns the list of HAVE / DONT_HAVE in the message
	BlockPresences() []BlockPresence
	// Haves returns the Cids for each HAVE
	Haves() []auth.Want
	// DontHaves returns the Cids for each DONT_HAVE
	DontHaves() []auth.Want
	// PendingBytes returns the number of outstanding bytes of data that the
	// engine has yet to send to the client (because they didn't fit in this
	// message)
	PendingBytes() int32

	// AddEntry adds an entry to the Wantlist.
	AddEntry(key auth.Want, priority int32, wantType pb.Message_Wantlist_WantType, sendDontHave bool) int

	// Cancel adds a CANCEL for the given CID to the message
	// Returns the size of the CANCEL entry in the protobuf
	Cancel(w auth.Want) int

	// Remove removes any entries for the given CID. Useful when the want
	// status for the CID changes when preparing a message.
	Remove(key auth.Want)

	// Empty indicates whether the message has any information
	Empty() bool
	// Size returns the size of the message in bytes
	Size() int

	// A full wantlist is an authoritative copy, a 'non-full' wantlist is a patch-set
	Full() bool

	// AddBlock adds a block to the message, with the auth that released it
	AddBlock(blocks.Block, string)
	// AddBlockPresence adds a HAVE / DONT_HAVE for the given Cid to the message
	AddBlockPresence(auth.Want, pb.Message_BlockPresenceType)
	// AddHave adds a HAVE for the given Cid to the message
	AddHave(auth.Want)
	// AddDontHave adds a DONT_HAVE for the given Cid to the message
	AddDontHave(auth.Want)
	// SetPendingBytes sets the number of bytes of data that are yet to be sent
	// to the client (because they didn't fit in this message)
	SetPendingBytes(int32)
	Exportable

	Loggable() map[string]interface{}

	// Reset the values in the message back to defaults, so it can be reused
	Reset(bool)

	// Clone the message fields
	Clone() BitSwapMessage
}

// Exportable is an interface for structures than can be
// encoded in a bitswap protobuf.
type Exportable interface {
	// Note that older Bitswap versions use a different wire format, so we need
	// to convert the message to the appropriate format depending on which
	// version of the protocol the remote peer supports.
	ToProtoV0() *pb.Message
	ToProtoV1() *pb.Message
	ToNetV0(w io.Writer) error
	ToNetV1(w io.Writer) error
}

// BlockPresence represents a HAVE / DONT_HAVE for a given Cid
type BlockPresence struct {
	Want auth.Want
	Type pb.Message_BlockPresenceType
}

// Entry is a wantlist entry in a Bitswap message, with flags indicating
// - whether message is a cancel
// - whether requester wants a DONT_HAVE message
// - whether requester wants a HAVE message (instead of the block)
type Entry struct {
	wantlist.Entry
	Cancel       bool
	SendDontHave bool
}

// Get the size of the entry on the wire
func (e *Entry) Size() int {
	epb := e.ToPB()
	return epb.Size()
}

// Get the entry in protobuf form
func (e *Entry) ToPB() pb.Message_Wantlist_Entry {
	return pb.Message_Wantlist_Entry{
		Block:        pb.Cid{Cid: e.Want.Cid},
		Auth:         e.Want.Auth,
		Priority:     int32(e.Priority),
		Cancel:       e.Cancel,
		WantType:     e.WantType,
		SendDontHave: e.SendDontHave,
	}
}

var MaxEntrySize = maxEntrySize()

func maxEntrySize() int {
	var maxInt32 int32 = (1 << 31) - 1

	c := cid.NewCidV0(u.Hash([]byte("cid")))
	a := strings.Repeat("A", 120)
	e := Entry{
		Entry: wantlist.Entry{
			Want:     auth.Want{Cid: c, Auth: a},
			Priority: maxInt32,
			WantType: pb.Message_Wantlist_Have,
		},
		SendDontHave: true, // true takes up more space than false
		Cancel:       true,
	}
	return e.Size()
}

type impl struct {
	full           bool
	wantlist       map[auth.Want]*Entry
	blocks         map[auth.Want]auth.AuthBlock
	rawData        map[auth.Want][]byte
	blockPresences map[auth.Want]pb.Message_BlockPresenceType
	pendingBytes   int32
}

// New returns a new, empty bitswap message
func New(full bool) BitSwapMessage {
	return newMsg(full)
}

func newMsg(full bool) *impl {
	return &impl{
		full:           full,
		wantlist:       make(map[auth.Want]*Entry),
		blocks:         make(map[auth.Want]auth.AuthBlock),
		rawData:        make(map[auth.Want][]byte),
		blockPresences: make(map[auth.Want]pb.Message_BlockPresenceType),
	}
}

// Clone the message fields
func (m *impl) Clone() BitSwapMessage {
	msg := newMsg(m.full)
	for k := range m.wantlist {
		msg.wantlist[k] = m.wantlist[k]
	}
	for k := range m.blocks {
		msg.blocks[k] = m.blocks[k]
	}
	for k := range m.blockPresences {
		msg.blockPresences[k] = m.blockPresences[k]
	}
	msg.pendingBytes = m.pendingBytes
	return msg
}

// Reset the values in the message back to defaults, so it can be reused
func (m *impl) Reset(full bool) {
	m.full = full
	for k := range m.wantlist {
		delete(m.wantlist, k)
	}
	for k := range m.blocks {
		delete(m.blocks, k)
	}
	for k := range m.blockPresences {
		delete(m.blockPresences, k)
	}
	m.pendingBytes = 0
}

var errCidMissing = errors.New("missing cid")

func newMessageFromProto(pbm pb.Message) (BitSwapMessage, error) {
	fmt.Println("messageFromProto")
	m := newMsg(pbm.Wantlist.Full)
	for _, e := range pbm.Wantlist.Entries {
		if !e.Block.Cid.Defined() {
			return nil, errCidMissing
		}
		m.addEntry(auth.Want{Cid: e.Block.Cid, Auth: e.Auth}, e.Priority, e.Cancel, e.WantType, e.SendDontHave)
	}

	// deprecated
	for _, d := range pbm.Blocks {
		// CIDv0, sha256, protobuf only
		b := blocks.NewBlock(d)
		m.AddBlock(b, "")
	}
	//

	for _, b := range pbm.GetPayload() {
		pref, err := cid.PrefixFromBytes(b.GetPrefix())
		if err != nil {
			return nil, err
		}

		c, err := pref.Sum(b.GetData())
		if err != nil {
			return nil, err
		}

		blk, err := blocks.NewBlockWithCid(b.GetData(), c)
		if err != nil {
			return nil, err
		}

		m.AddBlock(blk, b.Auth)
	}

	for _, bi := range pbm.GetBlockPresences() {
		if !bi.Cid.Cid.Defined() {
			return nil, errCidMissing
		}
		w := auth.Want{Cid: bi.Cid.Cid, Auth: bi.Auth}
		m.AddBlockPresence(w, bi.Type)
	}

	m.pendingBytes = pbm.PendingBytes

	return m, nil
}

func (m *impl) Full() bool {
	return m.full
}

func (m *impl) Empty() bool {
	return len(m.blocks) == 0 && len(m.wantlist) == 0 && len(m.blockPresences) == 0
}

func (m *impl) Wantlist() []Entry {
	out := make([]Entry, 0, len(m.wantlist))
	for _, e := range m.wantlist {
		out = append(out, *e)
	}
	return out
}

func (m *impl) Blocks() []auth.AuthBlock {
	bs := make([]auth.AuthBlock, 0, len(m.blocks))
	for _, block := range m.blocks {
		bs = append(bs, block)
	}
	return bs
}

func (m *impl) BlockPresences() []BlockPresence {
	bps := make([]BlockPresence, 0, len(m.blockPresences))
	for c, t := range m.blockPresences {
		bps = append(bps, BlockPresence{c, t})
	}
	return bps
}

func (m *impl) Haves() []auth.Want {
	return m.getBlockPresenceByType(pb.Message_Have)
}

func (m *impl) DontHaves() []auth.Want {
	return m.getBlockPresenceByType(pb.Message_DontHave)
}

func (m *impl) getBlockPresenceByType(t pb.Message_BlockPresenceType) []auth.Want {
	cids := make([]auth.Want, 0, len(m.blockPresences))
	for c, bpt := range m.blockPresences {
		if bpt == t {
			cids = append(cids, c)
		}
	}
	return cids
}

func (m *impl) PendingBytes() int32 {
	return m.pendingBytes
}

func (m *impl) SetPendingBytes(pendingBytes int32) {
	m.pendingBytes = pendingBytes
}

func (m *impl) Remove(w auth.Want) {
	delete(m.wantlist, w)
}

func (m *impl) Cancel(w auth.Want) int {
	return m.addEntry(w, 0, true, pb.Message_Wantlist_Block, false)
}

func (m *impl) AddEntry(w auth.Want, priority int32, wantType pb.Message_Wantlist_WantType, sendDontHave bool) int {
	return m.addEntry(w, priority, false, wantType, sendDontHave)
}

func (m *impl) addEntry(w auth.Want, priority int32, cancel bool, wantType pb.Message_Wantlist_WantType, sendDontHave bool) int {
	e, exists := m.wantlist[w]
	if exists {
		// Only change priority if want is of the same type
		if e.WantType == wantType {
			e.Priority = priority
		}
		// Only change from "dont cancel" to "do cancel"
		if cancel {
			e.Cancel = cancel
		}
		// Only change from "dont send" to "do send" DONT_HAVE
		if sendDontHave {
			e.SendDontHave = sendDontHave
		}
		// want-block overrides existing want-have
		if wantType == pb.Message_Wantlist_Block && e.WantType == pb.Message_Wantlist_Have {
			e.WantType = wantType
		}
		m.wantlist[w] = e
		return 0
	}

	e = &Entry{
		Entry: wantlist.Entry{
			Want:     w,
			Priority: priority,
			WantType: wantType,
		},
		SendDontHave: sendDontHave,
		Cancel:       cancel,
	}
	m.wantlist[w] = e

	return e.Size()
}

func (m *impl) AddBlock(b blocks.Block, a string) {
	fmt.Println("AddBlock ", b.Cid(), a)
	w := auth.Want{Cid: b.Cid(), Auth: a}
	delete(m.blockPresences, w)
	m.blocks[w] = auth.NewBlock(b)
	m.rawData[w] = b.RawData()
}

func (m *impl) AddBlockPresence(c auth.Want, t pb.Message_BlockPresenceType) {
	if _, ok := m.blocks[c]; ok {
		return
	}
	m.blockPresences[c] = t
}

func (m *impl) AddHave(c auth.Want) {
	m.AddBlockPresence(c, pb.Message_Have)
}

func (m *impl) AddDontHave(c auth.Want) {
	m.AddBlockPresence(c, pb.Message_DontHave)
}

func (m *impl) Size() int {
	size := 0
	for _, block := range m.blocks {
		size += block.Size()
	}
	for c := range m.blockPresences {
		size += BlockPresenceSize(c)
	}
	for _, e := range m.wantlist {
		size += e.Size()
	}

	return size
}

func BlockPresenceSize(w auth.Want) int {
	return (&pb.Message_BlockPresence{
		Cid:  pb.Cid{Cid: w.Cid},
		Auth: w.Auth,
		Type: pb.Message_Have,
	}).Size()
}

// FromNet generates a new BitswapMessage from incoming data on an io.Reader.
func FromNet(r io.Reader) (BitSwapMessage, error) {
	reader := msgio.NewVarintReaderSize(r, network.MessageSizeMax)
	return FromMsgReader(reader)
}

// FromPBReader generates a new Bitswap message from a gogo-protobuf reader
func FromMsgReader(r msgio.Reader) (BitSwapMessage, error) {
	msg, err := r.ReadMsg()
	if err != nil {
		return nil, err
	}

	var pb pb.Message
	err = pb.Unmarshal(msg)
	r.ReleaseMsg(msg)
	if err != nil {
		return nil, err
	}

	return newMessageFromProto(pb)
}

func (m *impl) ToProtoV0() *pb.Message {
	fmt.Println("ToProtoV1")
	pbm := new(pb.Message)
	pbm.Wantlist.Entries = make([]pb.Message_Wantlist_Entry, 0, len(m.wantlist))
	for _, e := range m.wantlist {
		pbm.Wantlist.Entries = append(pbm.Wantlist.Entries, e.ToPB())
	}
	pbm.Wantlist.Full = m.full

	blocks := m.Blocks()
	pbm.Blocks = make([][]byte, 0, len(blocks))
	for c, _ := range m.blocks {
		pbm.Blocks = append(pbm.Blocks, m.rawData[c])
	}
	return pbm
}

func (m *impl) ToProtoV1() *pb.Message {
	fmt.Println("ToProtoV1")
	pbm := new(pb.Message)
	pbm.Wantlist.Entries = make([]pb.Message_Wantlist_Entry, 0, len(m.wantlist))
	for _, e := range m.wantlist {
		pbm.Wantlist.Entries = append(pbm.Wantlist.Entries, e.ToPB())
	}
	pbm.Wantlist.Full = m.full

	blocks := m.Blocks()
	pbm.Payload = make([]pb.Message_Block, 0, len(blocks))
	for c, b := range m.blocks {
		pbm.Payload = append(pbm.Payload, pb.Message_Block{
			Data:   m.rawData[c],
			Prefix: b.Cid().Prefix().Bytes(),
			Auth:   b.Want.Auth,
		})
	}

	pbm.BlockPresences = make([]pb.Message_BlockPresence, 0, len(m.blockPresences))
	for c, t := range m.blockPresences {
		pbm.BlockPresences = append(pbm.BlockPresences, pb.Message_BlockPresence{
			Cid:  pb.Cid{Cid: c.Cid},
			Auth: c.Auth,
			Type: t,
		})
	}

	pbm.PendingBytes = m.PendingBytes()

	return pbm
}

func (m *impl) ToNetV0(w io.Writer) error {
	return write(w, m.ToProtoV0())
}

func (m *impl) ToNetV1(w io.Writer) error {
	return write(w, m.ToProtoV1())
}

func write(w io.Writer, m *pb.Message) error {
	fmt.Println("message.write()")
	size := m.Size()

	buf := pool.Get(size + binary.MaxVarintLen64)
	defer pool.Put(buf)

	n := binary.PutUvarint(buf, uint64(size))

	written, err := m.MarshalTo(buf[n:])
	if err != nil {
		return err
	}
	n += written

	_, err = w.Write(buf[:n])
	return err
}

func (m *impl) Loggable() map[string]interface{} {
	blocks := make([]string, 0, len(m.blocks))
	for _, v := range m.blocks {
		blocks = append(blocks, v.Cid().String())
	}
	return map[string]interface{}{
		"blocks": blocks,
		"wants":  m.Wantlist(),
	}
}
