package db_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"GolemUI/pkg/db"

	"github.com/jackc/pgx/v5/pgconn"
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

func TestDBInterfaces(t *testing.T) {
	// This will fail compilation if DatabasePool or DBQuerier is not defined, or if DB.CorePool/BusinessPool do not implement it.
	var _ db.DatabasePool = nil
	var _ db.DBQuerier = nil

	var d db.DB
	var _ db.DatabasePool = d.CorePool
	var _ db.DatabasePool = d.BusinessPool
}

func TestMockDBPool_Scenario1(t *testing.T) {
	ctx := context.Background()
	pool := db.NewMockDBPool()

	columns := []string{"id", "name"}
	values := [][]any{
		{101, "core_widget"},
		{102, "layout_grid"},
	}
	pool.RegisterQuery("SELECT id, name FROM components", columns, values, nil)

	rows, err := pool.Query(ctx, "SELECT id, name FROM components")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if id != values[count][0].(int) || name != values[count][1].(string) {
			t.Errorf("Unexpected values at row %d: got (%d, %q), want (%d, %q)",
				count, id, name, values[count][0].(int), values[count][1].(string))
		}
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 rows, got %d", count)
	}
	if err := rows.Err(); err != nil {
		t.Errorf("rows.Err() returned error: %v", err)
	}
}

func TestMockDBPool_Scenario2(t *testing.T) {
	ctx := context.Background()
	pool := db.NewMockDBPool()

	expectedTag := pgconn.NewCommandTag("UPDATE 1")
	pool.RegisterExec("UPDATE components SET name = $1 WHERE id = $2", expectedTag, nil)

	tag, err := pool.Exec(ctx, "UPDATE components SET name = $1 WHERE id = $2", "new_name", 101)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if tag.String() != expectedTag.String() {
		t.Errorf("Unexpected command tag: got %q, want %q", tag.String(), expectedTag.String())
	}
	if tag.RowsAffected() != 1 {
		t.Errorf("Expected 1 row affected, got %d", tag.RowsAffected())
	}
}

func TestMockDBPool_Scenario3(t *testing.T) {
	ctx := context.Background()
	pool := db.NewMockDBPool()

	expectedErr := fmt.Errorf("database connection timeout")
	pool.RegisterQuery("SELECT * FROM logs", nil, nil, expectedErr)

	rows, err := pool.Query(ctx, "SELECT * FROM logs")
	if err != expectedErr {
		t.Fatalf("Expected query failure with %v, got %v (rows: %v)", expectedErr, err, rows)
	}
	if rows != nil {
		t.Errorf("Expected nil rows, got %v", rows)
	}
}

func TestMockDBPool_Scenario4(t *testing.T) {
	ctx := context.Background()
	pool := db.NewMockDBPool()

	pool.RegisterQuery("SELECT status FROM systems WHERE id = 1", []string{"status"}, [][]any{{"active"}}, nil)

	var status string
	err := pool.QueryRow(ctx, "SELECT status FROM systems WHERE id = 1").Scan(&status)
	if err != nil {
		t.Fatalf("QueryRow Scan failed: %v", err)
	}
	if status != "active" {
		t.Errorf("Expected status %q, got %q", "active", status)
	}
}

func TestMockDBPool_Concurrency(t *testing.T) {
	pool := db.NewMockDBPool()
	ctx := context.Background()

	var wg sync.WaitGroup
	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			queryStr := fmt.Sprintf("SELECT val FROM data WHERE id = %d", id)
			pool.RegisterQuery(queryStr, []string{"val"}, [][]any{{id}}, nil)
		}(i)
	}
	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			queryStr := fmt.Sprintf("SELECT val FROM data WHERE id = %d", id)
			_, _ = pool.Query(ctx, queryStr)
		}(i)
	}
	wg.Wait()
}

func TestMockRowsFieldDescriptions(t *testing.T) {
	ctx := context.Background()

	// Case 1: normal columns
	t.Run("NormalColumns", func(t *testing.T) {
		pool := db.NewMockDBPool()
		cols := []string{"id", "title", "amount"}
		pool.RegisterQuery("SELECT id, title, amount FROM items", cols, [][]any{{1, "Book", 12.5}}, nil)

		rows, err := pool.Query(ctx, "SELECT id, title, amount FROM items")
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		defer rows.Close()

		fds := rows.FieldDescriptions()
		if len(fds) != len(cols) {
			t.Fatalf("expected %d field descriptions, got %d", len(cols), len(fds))
		}

		for i, fd := range fds {
			if fd.Name != cols[i] {
				t.Errorf("expected field %d to have name %q, got %q", i, cols[i], fd.Name)
			}
		}
	})

	// Case 2: empty columns (edge case)
	t.Run("EmptyColumns", func(t *testing.T) {
		pool := db.NewMockDBPool()
		pool.RegisterQuery("SELECT 1", nil, nil, nil)

		rows, err := pool.Query(ctx, "SELECT 1")
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		defer rows.Close()

		fds := rows.FieldDescriptions()
		if len(fds) != 0 {
			t.Errorf("expected 0 field descriptions, got %d", len(fds))
		}
	})
}



