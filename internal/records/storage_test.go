package records

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"go-education/migrations"
)

// newTestStorage подключается к TEST_DATABASE_URL, создаёт отдельную
// временную схему и применяет в ней все миграции, чтобы не трогать настоящие данные.
// Без TEST_DATABASE_URL тест пропускается.
func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — пропускаю тесты с БД")
	}
	ctx := context.Background()

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

	// search_path в URL: и миграции, и запросы работают только внутри временной схемы.
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	schemaDSN := u.String()

	if err := migrations.Up(schemaDSN); err != nil {
		t.Fatal(err)
	}

	db, err := pgxpool.New(ctx, schemaDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return NewStorage(db)
}

func TestStorageCRUD(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	list, err := s.GetList(ctx)
	if err != nil || len(list) != 0 || list == nil {
		t.Fatalf("пустой список: %v, %v", list, err)
	}

	created, err := s.Create(ctx, RecordInput{Name: "первая", Description: "описание"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "первая" || created.Description != "описание" || created.CreatedAt.IsZero() {
		t.Errorf("Create вернул %+v", created)
	}

	got, err := s.GetByID(ctx, created.ID)
	if err != nil || got != created {
		t.Errorf("GetByID: %+v, %v", got, err)
	}

	updated, err := s.Update(ctx, created.ID, RecordInput{Name: "изменённая"})
	if err != nil || updated.Name != "изменённая" || updated.Description != "" || updated.ID != created.ID {
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
	if _, err := s.Update(ctx, 999, RecordInput{Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update: %v, ожидали ErrNotFound", err)
	}
	if err := s.Delete(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete: %v, ожидали ErrNotFound", err)
	}
}

func TestStorageRejectsBlankName(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.Create(context.Background(), RecordInput{Name: "   "}); err == nil {
		t.Error("БД приняла пустое имя, ожидали ошибку CHECK")
	}
}
