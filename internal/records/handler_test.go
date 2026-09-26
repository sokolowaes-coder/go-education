package records

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	records []Record
	nextID  int64
	err     error
}

func newFakeStore(names ...string) *fakeStore {
	s := &fakeStore{nextID: 1}
	for _, n := range names {
		s.Create(context.Background(), n)
	}
	return s
}

func (s *fakeStore) GetList(ctx context.Context) ([]Record, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out, nil
}

func (s *fakeStore) GetByID(ctx context.Context, id int64) (Record, error) {
	if s.err != nil {
		return Record{}, s.err
	}
	for _, r := range s.records {
		if r.ID == id {
			return r, nil
		}
	}
	return Record{}, ErrNotFound
}

func (s *fakeStore) Create(ctx context.Context, name string) (Record, error) {
	if s.err != nil {
		return Record{}, s.err
	}
	rec := Record{ID: s.nextID, Name: name, CreatedAt: time.Now()}
	s.nextID++
	s.records = append(s.records, rec)
	return rec, nil
}

func (s *fakeStore) Update(ctx context.Context, id int64, name string) (Record, error) {
	if s.err != nil {
		return Record{}, s.err
	}
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].Name = name
			return s.records[i], nil
		}
	}
	return Record{}, ErrNotFound
}

func (s *fakeStore) Delete(ctx context.Context, id int64) error {
	if s.err != nil {
		return s.err
	}
	for i, r := range s.records {
		if r.ID == id {
			s.records = append(s.records[:i], s.records[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// do отправляет запрос в хендлер через настоящий роутер,
// чтобы заодно проверить пути и {id}.
func do(t *testing.T, store RecordStore, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(store).Register(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHandlerStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"список", "GET", "/records", "", http.StatusOK},
		{"одна запись", "GET", "/records/1", "", http.StatusOK},
		{"нет записи", "GET", "/records/999", "", http.StatusNotFound},
		{"id не число", "GET", "/records/abc", "", http.StatusBadRequest},
		{"id отрицательный", "GET", "/records/-1", "", http.StatusBadRequest},

		{"создание", "POST", "/records", `{"name":"новая"}`, http.StatusCreated},
		{"создание: пустое имя", "POST", "/records", `{"name":""}`, http.StatusBadRequest},
		{"создание: имя из пробелов", "POST", "/records", `{"name":"   "}`, http.StatusBadRequest},
		{"создание: без name", "POST", "/records", `{}`, http.StatusBadRequest},
		{"создание: сломанный JSON", "POST", "/records", `{`, http.StatusBadRequest},

		{"изменение", "PUT", "/records/1", `{"name":"другая"}`, http.StatusOK},
		{"изменение: нет записи", "PUT", "/records/999", `{"name":"x"}`, http.StatusNotFound},
		{"изменение: пустое имя", "PUT", "/records/1", `{"name":""}`, http.StatusBadRequest},
		{"изменение: id не число", "PUT", "/records/abc", `{"name":"x"}`, http.StatusBadRequest},

		{"удаление", "DELETE", "/records/1", "", http.StatusNoContent},
		{"удаление: нет записи", "DELETE", "/records/999", "", http.StatusNotFound},
		{"удаление: id не число", "DELETE", "/records/abc", "", http.StatusBadRequest},

		{"неподдерживаемый метод", "PATCH", "/records/1", "", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore("первая")
			rec := do(t, store, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Errorf("статус = %d, ожидали %d; тело: %s", rec.Code, tt.wantStatus, rec.Body)
			}
		})
	}
}

func TestHandlerStoreError(t *testing.T) {
	requests := []struct {
		method, path, body string
	}{
		{"GET", "/records", ""},
		{"GET", "/records/1", ""},
		{"POST", "/records", `{"name":"x"}`},
		{"PUT", "/records/1", `{"name":"x"}`},
		{"DELETE", "/records/1", ""},
	}

	for _, r := range requests {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			store := &fakeStore{err: errors.New("БД упала")}
			rec := do(t, store, r.method, r.path, r.body)
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("статус = %d, ожидали 500", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "БД упала") {
				t.Errorf("текст внутренней ошибки утёк клиенту: %s", rec.Body)
			}
		})
	}
}

func TestGetListEmptyIsArray(t *testing.T) {
	rec := do(t, newFakeStore(), "GET", "/records", "")
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("тело = %q, ожидали []", got)
	}
}

func TestCreateReturnsRecord(t *testing.T) {
	store := newFakeStore()
	rec := do(t, store, "POST", "/records", `{"name":"новая"}`)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got Record
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if got.ID != 1 || got.Name != "новая" {
		t.Errorf("получили %+v", got)
	}
	if len(store.records) != 1 {
		t.Errorf("в хранилище %d записей, ожидали 1", len(store.records))
	}
}

func TestUpdateChangesName(t *testing.T) {
	store := newFakeStore("старая")
	do(t, store, "PUT", "/records/1", `{"name":"новая"}`)

	if store.records[0].Name != "новая" {
		t.Errorf("имя = %q, ожидали \"новая\"", store.records[0].Name)
	}
}

func TestDeleteRemovesRecord(t *testing.T) {
	store := newFakeStore("первая", "вторая")
	do(t, store, "DELETE", "/records/1", "")

	if len(store.records) != 1 || store.records[0].ID != 2 {
		t.Errorf("после удаления осталось %+v", store.records)
	}
}
