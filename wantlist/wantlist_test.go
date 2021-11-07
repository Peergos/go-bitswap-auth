package wantlist

import (
	"testing"

	cid "github.com/ipfs/go-cid"
	"github.com/peergos/go-bitswap-auth/auth"
	pb "github.com/peergos/go-bitswap-auth/message/pb"
)

var testcids []auth.Want

func init() {
	strs := []string{
		"QmQL8LqkEgYXaDHdNYCG2mmpow7Sp8Z8Kt3QS688vyBeC7",
		"QmcBDsdjgSXU7BP4A4V8LJCXENE5xVwnhrhRGVTJr9YCVj",
		"QmQakgd2wDxc3uUF4orGdEm28zUT9Mmimp5pyPG2SFS9Gj",
	}
	for _, s := range strs {
		c, err := cid.Decode(s)
		if err != nil {
			panic(err)
		}
		testcids = append(testcids, auth.NewWant(c, "auth"))
	}

}

type wli interface {
	Contains(auth.Want) (Entry, bool)
}

func assertHasCid(t *testing.T, w wli, c auth.Want) {
	e, ok := w.Contains(c)
	if !ok {
		t.Fatal("expected to have ", c)
	}
	if !e.Want.Equals(c) {
		t.Fatal("returned entry had wrong cid value")
	}
}

func TestBasicWantlist(t *testing.T) {
	wl := New()

	if !wl.Add(testcids[0], 5, pb.Message_Wantlist_Block) {
		t.Fatal("expected true")
	}
	assertHasCid(t, wl, testcids[0])
	if !wl.Add(testcids[1], 4, pb.Message_Wantlist_Block) {
		t.Fatal("expected true")
	}
	assertHasCid(t, wl, testcids[0])
	assertHasCid(t, wl, testcids[1])

	if wl.Len() != 2 {
		t.Fatal("should have had two items")
	}

	if wl.Add(testcids[1], 4, pb.Message_Wantlist_Block) {
		t.Fatal("add shouldnt report success on second add")
	}
	assertHasCid(t, wl, testcids[0])
	assertHasCid(t, wl, testcids[1])

	if wl.Len() != 2 {
		t.Fatal("should have had two items")
	}

	if !wl.RemoveType(testcids[0], pb.Message_Wantlist_Block) {
		t.Fatal("should have gotten true")
	}

	assertHasCid(t, wl, testcids[1])
	if _, has := wl.Contains(testcids[0]); has {
		t.Fatal("shouldnt have this cid")
	}
}

func TestAddHaveThenBlock(t *testing.T) {
	wl := New()

	wl.Add(testcids[0], 5, pb.Message_Wantlist_Have)
	wl.Add(testcids[0], 5, pb.Message_Wantlist_Block)

	e, ok := wl.Contains(testcids[0])
	if !ok {
		t.Fatal("expected to have ", testcids[0])
	}
	if e.WantType != pb.Message_Wantlist_Block {
		t.Fatal("expected to be ", pb.Message_Wantlist_Block)
	}
}

func TestAddBlockThenHave(t *testing.T) {
	wl := New()

	wl.Add(testcids[0], 5, pb.Message_Wantlist_Block)
	wl.Add(testcids[0], 5, pb.Message_Wantlist_Have)

	e, ok := wl.Contains(testcids[0])
	if !ok {
		t.Fatal("expected to have ", testcids[0])
	}
	if e.WantType != pb.Message_Wantlist_Block {
		t.Fatal("expected to be ", pb.Message_Wantlist_Block)
	}
}

func TestAddHaveThenRemoveBlock(t *testing.T) {
	wl := New()

	wl.Add(testcids[0], 5, pb.Message_Wantlist_Have)
	wl.RemoveType(testcids[0], pb.Message_Wantlist_Block)

	_, ok := wl.Contains(testcids[0])
	if ok {
		t.Fatal("expected not to have ", testcids[0])
	}
}

func TestAddBlockThenRemoveHave(t *testing.T) {
	wl := New()

	wl.Add(testcids[0], 5, pb.Message_Wantlist_Block)
	wl.RemoveType(testcids[0], pb.Message_Wantlist_Have)

	e, ok := wl.Contains(testcids[0])
	if !ok {
		t.Fatal("expected to have ", testcids[0])
	}
	if e.WantType != pb.Message_Wantlist_Block {
		t.Fatal("expected to be ", pb.Message_Wantlist_Block)
	}
}

func TestAddHaveThenRemoveAny(t *testing.T) {
	wl := New()

	wl.Add(testcids[0], 5, pb.Message_Wantlist_Have)
	wl.Remove(testcids[0])

	_, ok := wl.Contains(testcids[0])
	if ok {
		t.Fatal("expected not to have ", testcids[0])
	}
}

func TestAddBlockThenRemoveAny(t *testing.T) {
	wl := New()

	wl.Add(testcids[0], 5, pb.Message_Wantlist_Block)
	wl.Remove(testcids[0])

	_, ok := wl.Contains(testcids[0])
	if ok {
		t.Fatal("expected not to have ", testcids[0])
	}
}

func TestAbsort(t *testing.T) {
	wl := New()
	wl.Add(testcids[0], 5, pb.Message_Wantlist_Block)
	wl.Add(testcids[1], 4, pb.Message_Wantlist_Have)
	wl.Add(testcids[2], 3, pb.Message_Wantlist_Have)

	wl2 := New()
	wl2.Add(testcids[0], 2, pb.Message_Wantlist_Have)
	wl2.Add(testcids[1], 1, pb.Message_Wantlist_Block)

	wl.Absorb(wl2)

	e, ok := wl.Contains(testcids[0])
	if !ok {
		t.Fatal("expected to have ", testcids[0])
	}
	if e.Priority != 5 {
		t.Fatal("expected priority 5")
	}
	if e.WantType != pb.Message_Wantlist_Block {
		t.Fatal("expected type ", pb.Message_Wantlist_Block)
	}

	e, ok = wl.Contains(testcids[1])
	if !ok {
		t.Fatal("expected to have ", testcids[1])
	}
	if e.Priority != 1 {
		t.Fatal("expected priority 1")
	}
	if e.WantType != pb.Message_Wantlist_Block {
		t.Fatal("expected type ", pb.Message_Wantlist_Block)
	}

	e, ok = wl.Contains(testcids[2])
	if !ok {
		t.Fatal("expected to have ", testcids[2])
	}
	if e.Priority != 3 {
		t.Fatal("expected priority 3")
	}
	if e.WantType != pb.Message_Wantlist_Have {
		t.Fatal("expected type ", pb.Message_Wantlist_Have)
	}
}

func TestSortEntries(t *testing.T) {
	wl := New()

	wl.Add(testcids[0], 3, pb.Message_Wantlist_Block)
	wl.Add(testcids[1], 5, pb.Message_Wantlist_Have)
	wl.Add(testcids[2], 4, pb.Message_Wantlist_Have)

	entries := wl.Entries()
	SortEntries(entries)

	if !entries[0].Want.Equals(testcids[1]) ||
		!entries[1].Want.Equals(testcids[2]) ||
		!entries[2].Want.Equals(testcids[0]) {
		t.Fatal("wrong order")
	}
}
