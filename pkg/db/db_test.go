package db_test

import (
	"context"
	"testing"
	"GolemUI/pkg/db"
)

func TestInitDB_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	// Provide invalid host/port to force a connection/dial failure
	cfgCore := db.Config{
		Host:     "invalid_host_golemui_core",
		Port:     5432,
		Database: "golemui_core",
		User:     "postgres",
		Password: "password",
	}
	cfgBiz := db.Config{
		Host:     "invalid_host_negocio",
		Port:     5432,
		Database: "negocio_production",
		User:     "postgres",
		Password: "password",
	}

	_, err := db.InitDB(ctx, cfgCore, cfgBiz)
	if err == nil {
		t.Error("Expected InitDB to fail with invalid hosts, but got no error")
	}
}

func TestInitDB_ParseConfigError(t *testing.T) {
	ctx := context.Background()
	// An out of range port should cause a parse error or connection pool error
	cfgCore := db.Config{
		Host:     "localhost",
		Port:     -1, // Invalid port
		Database: "golemui_core",
		User:     "postgres",
		Password: "password",
	}
	cfgBiz := db.Config{
		Host:     "localhost",
		Port:     5432,
		Database: "negocio_production",
		User:     "postgres",
		Password: "password",
	}

	_, err := db.InitDB(ctx, cfgCore, cfgBiz)
	if err == nil {
		t.Error("Expected InitDB to fail with invalid port, but got no error")
	}
}
