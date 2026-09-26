// Package migrations хранит SQL-миграции внутри бинарника и применяет их к БД.
package migrations

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var files embed.FS

// Up применяет все ещё не применённые миграции.
// Какие уже применены, migrate хранит в таблице schema_migrations.
func Up(databaseURL string) error {
	src, err := iofs.New(files, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toMigrateURL(databaseURL))
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("применение миграций: %w", err)
	}
	return nil
}

// toMigrateURL меняет схему postgres:// на pgx5:// — так migrate выбирает драйвер.
func toMigrateURL(databaseURL string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(databaseURL, prefix); ok {
			return "pgx5://" + rest
		}
	}
	return databaseURL
}
