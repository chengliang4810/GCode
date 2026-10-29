//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

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
