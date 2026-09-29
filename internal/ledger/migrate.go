package ledger

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrate applies every migration file not yet recorded, in filename order,
// each in its own transaction. Several processes can open a new ledger at
// once (issue #153), so whether a migration is applied is decided inside the
// transaction that applies it: the ledger's transactions BEGIN IMMEDIATE
// (_txlock=immediate), which serializes them on SQLite's write lock, and a
// migration another process recorded first is skipped, not applied twice.
func (l *Ledger) migrate(ctx context.Context) error {
	err := l.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
  name       TEXT PRIMARY KEY,
  applied_at TEXT NOT NULL
)`)
		return err
	})
	if err != nil {
		return fmt.Errorf("ledger: creating schema_migrations: %w", err)
	}

	applied, err := l.appliedMigrations(ctx)
	if err != nil {
		return err
	}
	names, err := migrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		if applied[name] {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("ledger: reading migration %s: %w", name, err)
		}
		err = l.tx(ctx, func(tx *sql.Tx) error {
			var done int
			if err := tx.QueryRowContext(ctx,
				`SELECT count(*) FROM schema_migrations WHERE name = ?`, name).Scan(&done); err != nil {
				return fmt.Errorf("ledger: reading schema_migrations: %w", err)
			}
			if done > 0 {
				return nil // another process applied it while this one waited
			}
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("ledger: applying migration %s: %w", name, err)
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
				name, l.stamp(l.now()))
			if err != nil {
				return fmt.Errorf("ledger: recording migration %s: %w", name, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// appliedMigrations reads the names already recorded. It is a fast path
// only: migrate re-checks each name under the write lock before applying it.
func (l *Ledger) appliedMigrations(ctx context.Context) (map[string]bool, error) {
	applied := map[string]bool{}
	rows, err := l.db.QueryContext(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("ledger: reading schema_migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("ledger: reading schema_migrations: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: reading schema_migrations: %w", err)
	}
	return applied, nil
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("ledger: listing migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
