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

	list, total, err := s.List(ctx, ListFilter{Limit: 20})
	if err != nil || len(list) != 0 || list == nil || total != 0 {
		t.Fatalf("пустой список: %v, %d, %v", list, total, err)
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
	list, _, _ = s.List(ctx, ListFilter{Limit: 20})
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

func TestStorageListFilters(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	for _, in := range []RecordInput{
		{Name: "Лазерная депиляция", Description: "кабинет 1"},
		{Name: "Чистка лица", Description: "после лазерной процедуры"},
		{Name: "Массаж", Description: "скидка 50%"},
		{Name: "Массаж_спины"},
		{Name: "Пилинг"},
	} {
		if _, err := s.Create(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	// created_at у всех «сейчас»; одну запись сдвигаем в прошлое для фильтра по дате.
	if _, err := s.db.Exec(ctx, `UPDATE records SET created_at = '2020-01-15T12:00:00Z' WHERE name = 'Пилинг'`); err != nil {
		t.Fatal(err)
	}

	str := func(v string) *string { return &v }
	at := func(v string) *time.Time {
		tm, _ := time.Parse(time.RFC3339, v)
		return &tm
	}

	tests := []struct {
		name      string
		filter    ListFilter
		wantNames []string
		wantTotal int
	}{
		{"без фильтров", ListFilter{}, []string{"Лазерная депиляция", "Чистка лица", "Массаж", "Массаж_спины", "Пилинг"}, 5},
		{"поиск без учёта регистра по name и description", ListFilter{FullText: str("ЛАЗЕР")}, []string{"Лазерная депиляция", "Чистка лица"}, 2},
		{"поиск только по name", ListFilter{FullText: str("лазер"), FullTextFields: []string{"name"}}, []string{"Лазерная депиляция"}, 1},
		{"% ищется как символ", ListFilter{FullText: str("50%")}, []string{"Массаж"}, 1},
		{"_ ищется как символ", ListFilter{FullText: str("_")}, []string{"Массаж_спины"}, 1},
		{"по списку id", ListFilter{IDs: []int64{2, 4, 999}}, []string{"Чистка лица", "Массаж_спины"}, 2},
		{"dateEnd в прошлом", ListFilter{DateEnd: at("2020-12-31T00:00:00Z")}, []string{"Пилинг"}, 1},
		{"dateStart отсекает старую", ListFilter{DateStart: at("2021-01-01T00:00:00Z")}, []string{"Лазерная депиляция", "Чистка лица", "Массаж", "Массаж_спины"}, 4},
		{"фильтры вместе (AND)", ListFilter{FullText: str("массаж"), IDs: []int64{3, 5}}, []string{"Массаж"}, 1},
		{"пагинация: count — всего, data — страница", ListFilter{Limit: 2, Offset: 1}, []string{"Чистка лица", "Массаж"}, 5},
		{"offset за концом", ListFilter{Limit: 2, Offset: 10}, []string{}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.filter.Limit == 0 {
				tt.filter.Limit = 20
			}
			list, total, err := s.List(ctx, tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			names := []string{}
			for _, r := range list {
				names = append(names, r.Name)
			}
			if fmt.Sprint(names) != fmt.Sprint(tt.wantNames) || total != tt.wantTotal {
				t.Errorf("получили %v (всего %d), ожидали %v (всего %d)", names, total, tt.wantNames, tt.wantTotal)
			}
		})
	}
}
