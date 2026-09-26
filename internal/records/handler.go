package records

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type RecordStore interface {
	List(ctx context.Context, f ListFilter) ([]Record, int, error)
	GetByID(ctx context.Context, id int64) (Record, error)
	Create(ctx context.Context, in RecordInput) (Record, error)
	Update(ctx context.Context, id int64, in RecordInput) (Record, error)
	Delete(ctx context.Context, id int64) error
}

var _ RecordStore = (*Storage)(nil)

type Handler struct {
	storage RecordStore
	loc     *time.Location // пояс для дат без пояса в фильтрах
}

func NewHandler(storage RecordStore, loc *time.Location) *Handler {
	return &Handler{storage: storage, loc: loc}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, err := ParseListFilter(r.URL.Query(), h.loc)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	records, total, err := h.storage.List(r.Context(), f)
	if err != nil {
		internalError(w, "list records", err)
		return
	}
	writeJSON(w, http.StatusOK, newListResponse(f, records, total, h.loc))
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	rec, err := h.storage.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "запись не найдена")
		return
	}
	if err != nil {
		internalError(w, "get record", err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeInput(w, r)
	if !ok {
		return
	}
	rec, err := h.storage.Create(r.Context(), in)
	if err != nil {
		internalError(w, "create record", err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	in, ok := decodeInput(w, r)
	if !ok {
		return
	}
	rec, err := h.storage.Update(r.Context(), id, in)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "запись не найдена")
		return
	}
	if err != nil {
		internalError(w, "update record", err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := h.storage.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "запись не найдена")
		return
	}
	if err != nil {
		internalError(w, "delete record", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /records", h.List)
	mux.HandleFunc("POST /records", h.Create)
	mux.HandleFunc("GET /records/{id}", h.GetByID)
	mux.HandleFunc("PUT /records/{id}", h.Update)
	mux.HandleFunc("DELETE /records/{id}", h.Delete)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return 0, false
	}
	return id, true
}

func decodeInput(w http.ResponseWriter, r *http.Request) (RecordInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var in RecordInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return RecordInput{}, false
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "name обязателен")
		return RecordInput{}, false
	}
	return in, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func internalError(w http.ResponseWriter, op string, err error) {
	slog.Error(op, "error", err)
	writeError(w, http.StatusInternalServerError, "внутренняя ошибка")
}

// writeError отвечает в едином формате ошибок: {"success": false, "error": "..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"success": false, "error": msg})
}
