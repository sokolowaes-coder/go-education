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
	records    []Record
	nextID     int64
	err        error
	lastFilter ListFilter // с какими фильтрами вызвали List
}

func newFakeStore(names ...string) *fakeStore {
	s := &fakeStore{nextID: 1}
	for _, n := range names {
		s.Create(context.Background(), RecordInput{Name: n})
	}
	return s
}

// List в фейке только применяет пагинацию: сами фильтры проверяются
// в тестах хранилища на настоящей БД.
func (s *fakeStore) List(ctx context.Context, f ListFilter) ([]Record, int, error) {
	s.lastFilter = f
	if s.err != nil {
		return nil, 0, s.err
	}
	start := min(f.Offset, len(s.records))
	end := min(start+f.Limit, len(s.records))
	out := make([]Record, end-start)
	copy(out, s.records[start:end])
	return out, len(s.records), nil
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

func (s *fakeStore) Create(ctx context.Context, in RecordInput) (Record, error) {
	if s.err != nil {
		return Record{}, s.err
	}
	rec := Record{ID: s.nextID, Name: in.Name, Description: in.Description, CreatedAt: time.Now()}
	s.nextID++
	s.records = append(s.records, rec)
	return rec, nil
}

func (s *fakeStore) Update(ctx context.Context, id int64, in RecordInput) (Record, error) {
	if s.err != nil {
		return Record{}, s.err
	}
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].Name = in.Name
			s.records[i].Description = in.Description
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
	NewHandler(store, time.UTC).Register(mux)
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
		{"создание с описанием", "POST", "/records", `{"name":"новая","description":"текст"}`, http.StatusCreated},
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

func TestListEmptyDataIsArray(t *testing.T) {
	rec := do(t, newFakeStore(), "GET", "/records", "")
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("пустой список должен быть \"data\":[], тело: %s", rec.Body)
	}
}

func TestListResponseFormat(t *testing.T) {
	store := newFakeStore("первая", "вторая", "третья")
	rec := do(t, store, "GET", "/records?fullText=вто&id=2,3&dateStart=2026-09-27&dateEnd=2026-09-27&limit=2&offset=1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d; тело: %s", rec.Code, rec.Body)
	}
	var got struct {
		Success bool `json:"success"`
		Filters struct {
			FullText  struct{ Value *string } `json:"fullText"`
			ID        struct{ Value []int64 } `json:"id"`
			DateStart struct{ Value *string } `json:"dateStart"`
			DateEnd   struct{ Value *string } `json:"dateEnd"`
		} `json:"filters"`
		Count      int        `json:"count"`
		Pagination Pagination `json:"pagination"`
		Data       []Record   `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if !got.Success || got.Count != 3 || len(got.Data) != 2 || got.Data[0].Name != "вторая" {
		t.Errorf("success=%v count=%d data=%+v", got.Success, got.Count, got.Data)
	}
	if got.Pagination != (Pagination{Limit: 2, Offset: 1}) {
		t.Errorf("pagination = %+v", got.Pagination)
	}
	f := got.Filters
	if f.FullText.Value == nil || *f.FullText.Value != "вто" {
		t.Errorf("fullText = %v", f.FullText.Value)
	}
	if len(f.ID.Value) != 2 || f.ID.Value[0] != 2 || f.ID.Value[1] != 3 {
		t.Errorf("id = %v", f.ID.Value)
	}
	if f.DateStart.Value == nil || *f.DateStart.Value != "2026-09-27T00:00:00" {
		t.Errorf("dateStart = %v", f.DateStart.Value)
	}
	if f.DateEnd.Value == nil || *f.DateEnd.Value != "2026-09-27T23:59:59" {
		t.Errorf("dateEnd = %v", f.DateEnd.Value)
	}
}

func TestListUnsetFiltersAreNull(t *testing.T) {
	rec := do(t, newFakeStore(), "GET", "/records", "")
	body := rec.Body.String()
	for _, want := range []string{
		`"fullText":{"value":null,"fields":null}`,
		`"id":{"value":null}`,
		`"dateStart":{"value":null}`,
		`"dateEnd":{"value":null}`,
		`"pagination":{"limit":20,"offset":0}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("в ответе нет %s\nтело: %s", want, body)
		}
	}
}

func TestListBadFilter(t *testing.T) {
	rec := do(t, newFakeStore(), "GET", "/records?limit=1000", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("статус = %d, ожидали 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"success":false`) || !strings.Contains(rec.Body.String(), "limit") {
		t.Errorf("тело = %s", rec.Body)
	}
}

func TestErrorsAreJSON(t *testing.T) {
	rec := do(t, newFakeStore(), "GET", "/records/999", "")
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"запись не найдена","success":false}` {
		t.Errorf("тело = %s", got)
	}
}

func TestCreateReturnsRecord(t *testing.T) {
	store := newFakeStore()
	rec := do(t, store, "POST", "/records", `{"name":"новая","description":"описание"}`)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got Record
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if got.ID != 1 || got.Name != "новая" || got.Description != "описание" {
		t.Errorf("получили %+v", got)
	}
	if len(store.records) != 1 {
		t.Errorf("в хранилище %d записей, ожидали 1", len(store.records))
	}
}

func TestUpdateChangesName(t *testing.T) {
	store := newFakeStore("старая")
	do(t, store, "PUT", "/records/1", `{"name":"новая","description":"описание"}`)

	if got := store.records[0]; got.Name != "новая" || got.Description != "описание" {
		t.Errorf("после изменения %+v", got)
	}
}

func TestDeleteRemovesRecord(t *testing.T) {
	store := newFakeStore("первая", "вторая")
	do(t, store, "DELETE", "/records/1", "")

	if len(store.records) != 1 || store.records[0].ID != 2 {
		t.Errorf("после удаления осталось %+v", store.records)
	}
}
