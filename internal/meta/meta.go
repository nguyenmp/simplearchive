package meta

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const (
	driverName      = "sqlite"
	maxOpenConns    = 1
	busyTimeoutMs   = 5000
)

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
	return &DB{db}, nil
}

func (d *DB) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}
	return d.DB.Close()
}