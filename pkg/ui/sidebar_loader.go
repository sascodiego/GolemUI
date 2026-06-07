package ui

import (
	"context"
	"fmt"
	"log"

	"GolemUI/pkg/db"
)

// NavigationMenuQuery retrieves all menu items ordered hierarchically:
// roots (NULL padre_id) first, then children grouped by parent and sorted by orden.
const NavigationMenuQuery = "SELECT id, padre_id, titulo, vista_id, orden FROM golemui.menu_navegacion ORDER BY padre_id NULLS FIRST, orden, id"

// MenuItem represents a single node in the navigation menu hierarchy.
type MenuItem struct {
	ID      string // Stable identifier (menu_navegacion.id)
	PadreID string // Parent node ID; empty string for root nodes (SQL NULL → "")
	Titulo  string // Human-readable display label
	VistaID string // Linked view ID; empty string for structural/folder nodes (SQL NULL → "")
	Orden   int    // Sort order among siblings (ascending)
}

// LoadNavigationMenu reads all menu items from the core database and validates
// that the parent-child hierarchy is acyclic. Returns the items sorted by the
// SQL query ordering (roots first, then children by parent, orden, and id).
func LoadNavigationMenu(ctx context.Context, pool db.DatabasePool) ([]MenuItem, error) {
	if pool == nil {
		return nil, fmt.Errorf("LoadNavigationMenu: pool is nil")
	}

	log.Printf("[UI/NavMenuLoader] Querying navigation menu from DB core")

	rows, err := pool.Query(ctx, NavigationMenuQuery)
	if err != nil {
		return nil, fmt.Errorf("LoadNavigationMenu: query failed: %w", err)
	}
	defer rows.Close()

	var items []MenuItem
	for rows.Next() {
		var item MenuItem
		var padreID, vistaID *string

		if err := rows.Scan(&item.ID, &padreID, &item.Titulo, &vistaID, &item.Orden); err != nil {
			return nil, fmt.Errorf("LoadNavigationMenu: scan failed: %w", err)
		}

		if padreID != nil {
			item.PadreID = *padreID
		}
		if vistaID != nil {
			item.VistaID = *vistaID
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("LoadNavigationMenu: row iteration error: %w", err)
	}

	// Ensure non-nil slice for empty results
	if items == nil {
		items = []MenuItem{}
	}

	log.Printf("[UI/NavMenuLoader] Loaded %d menu items, validating hierarchy", len(items))

	if err := validateNoCycles(items); err != nil {
		log.Printf("[UI/NavMenuLoader] %v", err)
		return nil, err
	}

	log.Printf("[UI/NavMenuLoader] Navigation menu validated successfully (%d items)", len(items))
	return items, nil
}

// validateNoCycles performs a DFS traversal over the menu items to detect
// circular references in the parent-child hierarchy. Returns an error describing
// the cycle path if one is found.
func validateNoCycles(items []MenuItem) error {
	// Build adjacency map: parent ID → children IDs
	children := make(map[string][]string, len(items))
	itemIDs := make(map[string]bool, len(items))

	for _, item := range items {
		itemIDs[item.ID] = true
		if item.PadreID != "" {
			children[item.PadreID] = append(children[item.PadreID], item.ID)
		}
	}

	visited := make(map[string]bool, len(items))
	visiting := make(map[string]bool, len(items))

	var dfs func(nodeID string, path []string) error
	dfs = func(nodeID string, path []string) error {
		if visiting[nodeID] {
			// Build cycle path string
			cycleStart := -1
			for i, id := range path {
				if id == nodeID {
					cycleStart = i
					break
				}
			}
			cyclePath := nodeID
			for i := len(path) - 1; i >= cycleStart; i-- {
				cyclePath = path[i] + " → " + cyclePath
			}
			return fmt.Errorf("cycle detected: %s", cyclePath)
		}
		if visited[nodeID] {
			return nil
		}

		visiting[nodeID] = true
		path = append(path, nodeID)

		for _, childID := range children[nodeID] {
			if err := dfs(childID, path); err != nil {
				return err
			}
		}

		visiting[nodeID] = false
		visited[nodeID] = true
		return nil
	}

	// Traverse every node to ensure pure cycles (where no node is a root) are detected
	for _, item := range items {
		if !visited[item.ID] && !visiting[item.ID] {
			if err := dfs(item.ID, nil); err != nil {
				return err
			}
		}
	}

	return nil
}
