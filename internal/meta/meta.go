package meta

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"sort"
	"strconv"

	_ "modernc.org/sqlite"
)

const (
	driverName    = "sqlite"
	maxOpenConns  = 1
	busyTimeoutMs = 5000
	// maxLimit caps the page size returned by ListSnapshots.
	maxLimit = 500
)

//go:embed schema_v1_create_snapshots_table.sql
var schemaV1 string

//go:embed schema_v2_create_extractor_runs_table.sql
var schemaV2 string

var migrations = map[int]string{
	1: schemaV1,
	2: schemaV2,
}

func dsn(path string) string {
	return fmt.Sprintf(
		"%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)",
		path, busyTimeoutMs,
	)
}

type DB struct {
	*sql.DB
}

func Open(ctx context.Context, path string) (*DB, error) {
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return nil, fmt.Errorf("meta.Open: open %q: %w", path, err)
	}
	db.SetMaxOpenConns(maxOpenConns)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("meta.Open: ping %q: %w", path, err)
	}

	d := &DB{db}
	if err := d.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("meta.Open: migrate %q: %w", path, err)
	}
	return d, nil
}

func (d *DB) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}
	return d.DB.Close()
}

func (d *DB) migrate(ctx context.Context) error {
	current, err := d.userVersion(ctx)
	if err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	versions := make([]int, 0, len(migrations))
	for v := range migrations {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	for _, v := range versions {
		if v <= current {
			continue
		}
		script := migrations[v]
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", v, err)
		}
		if _, err := tx.ExecContext(ctx, script); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", v, err)
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(v)); err != nil {
			tx.Rollback()
			return fmt.Errorf("set user_version=%d: %w", v, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", v, err)
		}
	}
	return nil
}

func (d *DB) userVersion(ctx context.Context) (int, error) {
	var v int
	if err := d.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}