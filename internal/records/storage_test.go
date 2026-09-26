package records

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestStorage подключается к TEST_DATABASE_URL и создаёт отдельную
// временную схему с таблицей из миграции, чтобы не трогать настоящие данные.
// Без TEST_DATABASE_URL тест пропускается.
func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — пропускаю тесты с БД")
	}
	ctx := context.Background()

	migration, err := os.ReadFile("../../migrations/000001_create_records.up.sql")
	if err != nil {
		t.Fatal(err)
	}

	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	if _, err := db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	return NewStorage(db)
}

func TestStorageCRUD(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	list, err := s.GetList(ctx)
	if err != nil || len(list) != 0 || list == nil {
		t.Fatalf("пустой список: %v, %v", list, err)
	}

	created, err := s.Create(ctx, "первая")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "первая" || created.CreatedAt.IsZero() {
		t.Errorf("Create вернул %+v", created)
	}

	got, err := s.GetByID(ctx, created.ID)
	if err != nil || got.Name != "первая" {
		t.Errorf("GetByID: %+v, %v", got, err)
	}

	updated, err := s.Update(ctx, created.ID, "изменённая")
	if err != nil || updated.Name != "изменённая" || updated.ID != created.ID {
		t.Errorf("Update: %+v, %v", updated, err)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Errorf("Delete: %v", err)
	}
	list, _ = s.GetList(ctx)
	if len(list) != 0 {
		t.Errorf("после удаления осталось %v", list)
	}
}

func TestStorageNotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	if _, err := s.GetByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID: %v, ожидали ErrNotFound", err)
	}
	if _, err := s.Update(ctx, 999, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update: %v, ожидали ErrNotFound", err)
	}
	if err := s.Delete(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete: %v, ожидали ErrNotFound", err)
	}
}

func TestStorageRejectsBlankName(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.Create(context.Background(), "   "); err == nil {
		t.Error("БД приняла пустое имя, ожидали ошибку CHECK")
	}
}
