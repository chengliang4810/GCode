//go:build windows && (amd64 || arm64)

package windows

import (
	"slices"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// accessTree returns a tree of a list of rows, each with a text, as the
// content describes a frame: rows are the IDs of the rows in order.
func accessTree(rows ...uint64) *platform.AccessTree {
	t := &platform.AccessTree{Nodes: []platform.AccessNode{{ID: 1, Parent: -1, Role: platform.RoleList}}}
	for _, id := range rows {
		t.Nodes = append(t.Nodes, platform.AccessNode{ID: id, Parent: 0, Role: platform.RoleListItem, Label: "row"})
		t.Nodes = append(t.Nodes, platform.AccessNode{ID: id << 8, Parent: len(t.Nodes) - 1, Role: platform.RoleText, Label: "text"})
	}
	return t
}

// newTestTree returns a tree as accessRoot makes it, without a window.
func newTestTree(t testing.TB, clients bool) *uiaTree {
	uiaOnce.Do(initUIA)
	saved := listening
	listening = func() bool { return clients }
	t.Cleanup(func() { listening = saved })
	tree := &uiaTree{nodes: map[uint64]*uiaElement{}}
	tree.root = newUIAElement(tree)
	tree.root.root = true
	return tree
}

func ids(es []*uiaElement) []uint64 {
	var out []uint64
	for _, e := range es {
		out = append(out, e.n.ID)
	}
	return out
}

// The elements of the tree follow the content's trees as it reuses the
// buffers of the last ones: rows coming, going and moving.
func TestUIATreeUpdate(t *testing.T) {
	tree := newTestTree(t, false)
	frames := [][]uint64{{2, 3, 4}, {2, 3, 4}, {3, 4, 5, 6}, {6, 3}, {6, 3}, {}, {7, 2}}
	var list *uiaElement
	for i, rows := range frames {
		tree.update(accessTree(rows...))
		if got := ids(tree.root.children); !slices.Equal(got, []uint64{1}) {
			t.Fatalf("frame %d: the root's children %v", i, got)
		}
		l := tree.nodes[1]
		if i > 0 && l != list {
			t.Fatalf("frame %d: the list is a new element", i)
		}
		list = l
		if got := ids(l.children); !slices.Equal(got, rows) {
			t.Fatalf("frame %d: the list's children %v, want %v", i, got, rows)
		}
		for _, row := range l.children {
			if row.parent != l || len(row.children) != 1 || row.children[0].n.ID != row.n.ID<<8 || row.children[0].parent != row {
				t.Fatalf("frame %d: row %d's links", i, row.n.ID)
			}
		}
		if len(tree.nodes) != 1+2*len(rows) || len(tree.order) != len(tree.nodes) {
			t.Fatalf("frame %d: %d nodes, %d in order", i, len(tree.nodes), len(tree.order))
		}
	}
	// Rows that left were let go of: none of the spare buffers holds them.
	for _, e := range tree.spareOrder[:cap(tree.spareOrder)] {
		if e != nil {
			t.Fatal("the spare order keeps an element")
		}
	}
	if len(tree.spareNodes) != 0 {
		t.Fatal("the spare map keeps elements")
	}
}

// The nodes with the announcer are let go of once the elements have them,
// as the strings of a tree that shrank would be kept past its length.
func TestUIATreeAnnouncer(t *testing.T) {
	tree := newTestTree(t, false)
	frame := accessTree(2, 3, 4)
	frame.Announcements = []string{"saved"}
	tree.update(frame)
	tree.update(accessTree(2))
	if e := tree.nodes[announcerID]; e == nil || e.n.Label != "saved" {
		t.Fatal("the announcer left the tree")
	}
	if got := ids(tree.root.children); !slices.Equal(got, []uint64{1, announcerID}) {
		t.Fatalf("the root's children %v", got)
	}
	for _, n := range tree.withAnnouncer[:cap(tree.withAnnouncer)] {
		if n.Label != "" {
			t.Fatalf("the nodes with the announcer keep %q", n.Label)
		}
	}
	if tree.announcerTree.Nodes != nil || tree.announcerTree.Announcements != nil {
		t.Fatal("the tree with the announcer is kept")
	}
}

func TestChildChanges(t *testing.T) {
	e := func(id uint64) *uiaElement { return &uiaElement{n: platform.AccessNode{ID: id}} }
	a, b, c, d := e(1), e(2), e(3), e(4)
	added, removed := childChanges([]*uiaElement{a, b, c}, []*uiaElement{b, d, a}, nil, nil)
	if !slices.Equal(ids(added), []uint64{4}) || !slices.Equal(ids(removed), []uint64{3}) {
		t.Errorf("added %v, removed %v", ids(added), ids(removed))
	}
	added, removed = childChanges([]*uiaElement{a}, []*uiaElement{a}, added, removed)
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("unchanged children: added %v, removed %v", ids(added), ids(removed))
	}
}

// A frame like the last allocates nothing, whether clients listen or not:
// the content describes every frame once one asked for the window.
func TestUIATreeUpdateAllocations(t *testing.T) {
	for _, clients := range []bool{false, true} {
		tree := newTestTree(t, clients)
		frame := accessTree(2, 3, 4, 5, 6, 7, 8, 9)
		// The first tree, as a client asks for it, raises no events: they
		// would reach UI Automation, which has no window here.
		tree.quiet = true
		tree.update(frame)
		tree.quiet = false
		tree.update(frame)
		if n := testing.AllocsPerRun(20, func() { tree.update(frame) }); n != 0 {
			t.Errorf("clients listening %v: %v allocations a frame", clients, n)
		}
	}
}

func BenchmarkUIATreeUpdate(b *testing.B) {
	rows := make([]uint64, 400)
	for i := range rows {
		rows[i] = uint64(i + 2)
	}
	frame := accessTree(rows...)
	for _, clients := range []bool{false, true} {
		name := "quiet"
		if clients {
			name = "listening"
		}
		b.Run(name, func(b *testing.B) {
			tree := newTestTree(b, clients)
			tree.quiet = true
			tree.update(frame)
			tree.quiet = false
			b.ReportAllocs()
			for b.Loop() {
				tree.update(frame)
			}
		})
	}
}

func TestUIAProviderDeclaresServerSideCOMContract(t *testing.T) {
	tree := newUIAContractTree(t)
	tree.update(uiaContractSnapshot(1<<32 | 7))
	for _, e := range append([]*uiaElement{tree.root}, tree.order...) {
		var options int32
		if hr := comCall(e.ptr(ifaceSimple), 3, uintptr(unsafe.Pointer(&options))); hr != sOK {
			t.Fatalf("get_ProviderOptions(%q): HRESULT %#x", e.n.Label, hr)
		}
		// ProviderOptions values are the public COM contract, independently
		// of the constants the implementation uses: ServerSideProvider=2,
		// UseComThreading=32. ClientSideProvider=1 must not be advertised.
		// https://learn.microsoft.com/windows/win32/api/uiautomationcore/ne-uiautomationcore-provideroptions
		if options != 0x22 {
			t.Fatalf("get_ProviderOptions(%q) = %#x, want server-side COM provider %#x", e.n.Label, options, 0x22)
		}
	}
}

func TestUIAProviderFocusTracksPublishedTreeThroughCOM(t *testing.T) {
	tree := newUIAContractTree(t)
	// Distinct 64-bit identities deliberately share their low 32 bits.
	// Fresh COM queries must follow the published focus, not tree order,
	// an earlier fragment reference, or a truncated identifier.
	for _, test := range []struct {
		name  string
		focus uint64
	}{
		{"main editor", 1<<32 | 7},
		{"side editor", 2<<32 | 7},
		{"no focused editor", 0},
		{"main editor again", 1<<32 | 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree.update(uiaContractSnapshot(test.focus))
			var fragment uintptr
			if hr := comCall(tree.root.ptr(ifaceRoot), 4, uintptr(unsafe.Pointer(&fragment))); hr != sOK {
				t.Fatalf("GetFocus: HRESULT %#x", hr)
			}
			if fragment != 0 {
				defer release(fragment)
			}
			if test.focus == 0 {
				if fragment != 0 {
					t.Fatal("GetFocus retained a fragment after focus cleared")
				}
			} else if fragment == 0 || uiaOf(fragment).n.ID != test.focus {
				t.Fatalf("GetFocus did not return published ID %#x", test.focus)
			}
			for _, e := range tree.order {
				var value variant
				if hr := comCall(e.ptr(ifaceSimple), 5, uiaHasKeyboardFocusProperty, uintptr(unsafe.Pointer(&value))); hr != sOK {
					t.Fatalf("HasKeyboardFocus(%q): HRESULT %#x", e.n.Label, hr)
				}
				want := uint64(0)
				if e.n.ID == test.focus {
					want = 0xffff // VARIANT_TRUE
				}
				if value.VT != vtBool || value.Val != want {
					t.Fatalf("HasKeyboardFocus(%q) = %+v, want VT_BOOL %#x", e.n.Label, value, want)
				}
			}
		})
	}
}

func newUIAContractTree(t *testing.T) *uiaTree {
	t.Helper()
	uiaOnce.Do(initUIA)
	// Quiet avoids events or UIA client discovery. There is no HWND,
	// window, renderer, input injection, or external accessibility client.
	tree := &uiaTree{nodes: make(map[uint64]*uiaElement), quiet: true}
	tree.root = newUIAElement(tree)
	tree.root.root = true
	t.Cleanup(func() {
		for _, e := range tree.order {
			e.dead = true
			e.release()
		}
		tree.root.dead = true
		tree.root.release()
	})
	return tree
}

func uiaContractSnapshot(focus uint64) *platform.AccessTree {
	return &platform.AccessTree{
		Focus: focus,
		Nodes: []platform.AccessNode{
			{ID: 1<<32 | 7, Parent: -1, Role: platform.RoleTextField, Label: "Main Message", States: platform.AccessFocusable, Actions: platform.ActionFocus},
			{ID: 2<<32 | 7, Parent: -1, Role: platform.RoleTextField, Label: "Side Message", States: platform.AccessFocusable, Actions: platform.ActionFocus},
		},
	}
}
