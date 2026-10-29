//go:build !uifixture

package workbench

import "github.com/egoist/mygo/ui"

// Production builds have no observation state. Fixture builds bind additional
// persistent handles to the same controls without changing their identity.
type tabStripObserver struct{}

func (tabStripObserver) bind(string, string, ui.Element) {}
