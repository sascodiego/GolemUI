package ui

import (
	"context"
	"encoding/json"
	"fmt"

	"GolemUI/pkg/db"
)

func LoadScreen(ctx context.Context, pool db.DatabasePool, vistaID string) (NodeMeta, error) {
	if pool == nil {
		return NodeMeta{}, fmt.Errorf("LoadScreen: pool is nil")
	}

	var jsonBytes []byte
	sql := "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"
	err := pool.QueryRow(ctx, sql, vistaID).Scan(&jsonBytes)
	if err != nil {
		return NodeMeta{}, fmt.Errorf("LoadScreen: vista %q not found", vistaID)
	}

	var nodeMeta NodeMeta
	if err := json.Unmarshal(jsonBytes, &nodeMeta); err != nil {
		return NodeMeta{}, fmt.Errorf("LoadScreen: failed to parse config_columnas for vista %q: %w", vistaID, err)
	}

	return nodeMeta, nil
}
