package ui

import (
	"sort"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// NavTree wraps a Fyne widget.Tree with navigation metadata for bidirectional
// sync. It maintains vistaID-to-nodeID and parent mappings needed to
// programmatically select and expand tree nodes when navigation occurs
// externally (e.g., via a button click).
type NavTree struct {
	tree        *widget.Tree
	vistaToNode map[string]string // vistaID → menuItem.ID
	parentOf    map[string]string // menuItem.ID → parent menuItem.ID
	navigating  atomic.Bool       // re-entrancy guard: true while programmatic SelectByVistaID is active
}

// Widget returns the underlying *widget.Tree for integration with Fyne layouts.
func (nt *NavTree) Widget() *widget.Tree {
	return nt.tree
}

// SelectByVistaID programmatically selects the tree node associated with the
// given vistaID. It opens all ancestor branches (root→parent order) before
// selecting the target node. If vistaID is empty or not found, it is a no-op.
//
// The navigating re-entrancy guard prevents the tree's OnSelected callback
// from re-triggering Navigate when the selection is programmatic.
func (nt *NavTree) SelectByVistaID(vistaID string) {
	if vistaID == "" {
		return
	}
	nodeID, ok := nt.vistaToNode[vistaID]
	if !ok {
		return
	}

	// Walk ancestor chain from target to root, then open root→parent
	ancestors := []string{}
	for cur := nodeID; cur != ""; {
		pid := nt.parentOf[cur]
		if pid != "" {
			ancestors = append(ancestors, pid)
		}
		cur = pid
	}

	nt.navigating.Store(true)
	defer func() { nt.navigating.Store(false) }()

	// fyne.DoAndWait blocks until the callback completes on the UI thread.
	// This preserves the re-entrancy guard: navigating is true for the
	// entire duration of the tree mutation, so OnSelected cannot re-enter
	// Navigate during programmatic selection.
	fyne.DoAndWait(func() {
		for i := len(ancestors) - 1; i >= 0; i-- {
			nt.tree.OpenBranch(widget.TreeNodeID(ancestors[i]))
		}
		nt.tree.Select(widget.TreeNodeID(nodeID))
	})
}

// BuildNavTree transforms a flat []MenuItem slice into a NavTree containing an
// interactive Fyne widget.Tree. Leaf nodes with a non-empty VistaID trigger
// the package-level Navigate function on selection. Branch nodes (structural
// containers) are expanded/collapsed but do not navigate.
//
// The returned NavTree supports bidirectional sync via SelectByVistaID.
func BuildNavTree(items []MenuItem) *NavTree {
	// Build parent-to-children index, sorted by Orden then ID.
	parentToChildren := make(map[string][]string)
	idToItem := make(map[string]MenuItem)

	for _, item := range items {
		idToItem[item.ID] = item
		parentToChildren[item.PadreID] = append(parentToChildren[item.PadreID], item.ID)
	}

	// Sort each children slice by Orden ascending, then ID as tiebreaker.
	for parentID, childIDs := range parentToChildren {
		sort.Slice(childIDs, func(i, j int) bool {
			a, okA := idToItem[childIDs[i]]
			b, okB := idToItem[childIDs[j]]
			if !okA || !okB {
				return childIDs[i] < childIDs[j]
			}
			if a.Orden != b.Orden {
				return a.Orden < b.Orden
			}
			return a.ID < b.ID
		})
		parentToChildren[parentID] = childIDs
	}

	tree := widget.NewTree(
		func(uid widget.TreeNodeID) []widget.TreeNodeID {
			children := parentToChildren[string(uid)]
			result := make([]widget.TreeNodeID, len(children))
			for i, id := range children {
				result[i] = widget.TreeNodeID(id)
			}
			return result
		},
		func(uid widget.TreeNodeID) bool {
			_, ok := parentToChildren[string(uid)]
			return ok
		},
		func(branch bool) fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(uid widget.TreeNodeID, branch bool, node fyne.CanvasObject) {
			label, ok := node.(*widget.Label)
			if !ok {
				return
			}
			item, exists := idToItem[string(uid)]
			if exists {
				label.SetText(item.Titulo)
			}
		},
	)

	navTree := &NavTree{
		tree:        tree,
		vistaToNode: make(map[string]string),
		parentOf:    make(map[string]string),
	}

	for _, item := range items {
		if item.VistaID != "" {
			navTree.vistaToNode[item.VistaID] = item.ID
		}
		if item.PadreID != "" {
			navTree.parentOf[item.ID] = item.PadreID
		}
	}

	tree.OnSelected = func(uid widget.TreeNodeID) {
		if navTree.navigating.Load() {
			return
		}
		item, exists := idToItem[string(uid)]
		if !exists {
			return
		}
		_, isBranch := parentToChildren[string(uid)]
		if isBranch {
			return
		}
		if item.VistaID == "" {
			return
		}
		if Navigate != nil {
			Navigate(item.VistaID)
		}
	}

	return navTree
}
