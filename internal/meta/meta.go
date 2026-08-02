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

//go:embed schema_v3_snapshots_surrogate_id.sql
var schemaV3 string

//go:embed schema_v4_extractor_runs_per_extractor.sql
var schemaV4 string

//go:embed schema_v5_drop_snapshot_status.sql
var schemaV5 string

//go:embed schema_v6_on_delete_cascade.sql
var schemaV6 string

var migrations = map[int]string{
	1: schemaV1,
	2: schemaV2,
	3: schemaV3,
	4: schemaV4,
	5: schemaV5,
	6: schemaV6,
}

// latestVersion returns the highest migration version number.
func latestVersion() int {
	v := 0
	for version := range migrations {
		if version > v {
			v = version
		}
	}
	return v
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

func (d *DB) migrate(ctx context.Context) (retErr error) {
	current, err := d.userVersion(ctx)
	if err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	versions := make([]int, 0, len(migrations))
	for v := range migrations {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	pending := make([]int, 0, len(versions))
	for _, v := range versions {
		if v > current {
			pending = append(pending, v)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	// Migrations may rebuild a parent table of an existing foreign key (v3
	// rebuilds snapshots, the parent of extractor_runs.timestamp). SQLite blocks
	// dropping a parent table while FK enforcement is on, so disable it for the
	// migration run and re-enable + verify afterwards. Each migration's data
	// copy preserves referential integrity by construction.
	if _, err := d.ExecContext(ctx, "PRAGMA foreign_keys=off"); err != nil {
		return fmt.Errorf("migrate: disable foreign_keys: %w", err)
	}
	defer func() {
		if _, err := d.ExecContext(ctx, "PRAGMA foreign_keys=on"); err != nil && retErr == nil {
			retErr = fmt.Errorf("migrate: re-enable foreign_keys: %w", err)
		}
	}()

	for _, v := range pending {
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

	// PRAGMA foreign_key_check reports orphaned child rows regardless of the
	// foreign_keys enforcement flag, so it works here even though we re-enable
	// enforcement via the deferred call. Fail loudly rather than leave dangling
	// references from a buggy migration.
	rows, err := d.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("migrate: foreign_key_check: %w", err)
	}
	violations := 0
	for rows.Next() {
		violations++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate: foreign_key_check rows: %w", err)
	}
	if violations > 0 {
		return fmt.Errorf("migrate: foreign_key_check reported %d FK violation(s)", violations)
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