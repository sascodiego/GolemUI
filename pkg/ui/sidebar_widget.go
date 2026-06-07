package ui

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// BuildNavTree transforms a flat []MenuItem slice into an interactive Fyne
// widget.Tree. Leaf nodes with a non-empty VistaID trigger the package-level
// Navigate function on selection. Branch nodes (structural containers) are
// expanded/collapsed but do not navigate.
func BuildNavTree(items []MenuItem) *widget.Tree {
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

	tree.OnSelected = func(uid widget.TreeNodeID) {
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

	return tree
}
