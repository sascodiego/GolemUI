package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"GolemUI/pkg/db"
)

func LoadScreen(ctx context.Context, pool db.DatabasePool, vistaID string) (NodeMeta, error) {
	if pool == nil {
		return NodeMeta{}, fmt.Errorf("LoadScreen: pool is nil")
	}

	log.Printf("[UI/ScreenLoader] Querying layout definition from DB core for vistaID: %q", vistaID)

	var jsonBytes []byte
	sql := "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
	err := pool.QueryRow(ctx, sql, vistaID).Scan(&jsonBytes)
	if err != nil {
		log.Printf("[UI/ScreenLoader] Error: vista %q not found in DB core: %v", vistaID, err)
		return NodeMeta{}, fmt.Errorf("LoadScreen: vista %q not found", vistaID)
	}

	log.Printf("[UI/ScreenLoader] Raw JSON layout retrieved from DB (len: %d bytes)", len(jsonBytes))

	var nodeMeta NodeMeta
	if err := json.Unmarshal(jsonBytes, &nodeMeta); err != nil {
		log.Printf("[UI/ScreenLoader] Error: failed to unmarshal layout JSON for vista %q: %v", vistaID, err)
		return NodeMeta{}, fmt.Errorf("LoadScreen: failed to parse config_columnas for vista %q: %w", vistaID, err)
	}

	log.Printf("[UI/ScreenLoader] Successfully unmarshaled layout for vista %q (area: %q, root: %q)", vistaID, nodeMeta.Area, nodeMeta.ComponentRef)
	return nodeMeta, nil
}
