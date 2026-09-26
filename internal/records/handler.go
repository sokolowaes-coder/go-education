package records

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type RecordStore interface {
	GetList(ctx context.Context) ([]Record, error)
	GetByID(ctx context.Context, id int64) (Record, error)
	Create(ctx context.Context, in RecordInput) (Record, error)
	Update(ctx context.Context, id int64, in RecordInput) (Record, error)
	Delete(ctx context.Context, id int64) error
}

var _ RecordStore = (*Storage)(nil)

type Handler struct {
	storage RecordStore
}

func NewHandler(storage RecordStore) *Handler {
	return &Handler{storage: storage}
}

func (h *Handler) GetList(w http.ResponseWriter, r *http.Request) {
	records, err := h.storage.GetList(r.Context())
	if err != nil {
		internalError(w, "get records", err)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	rec, err := h.storage.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "запись не найдена", http.StatusNotFound)
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
		http.Error(w, "запись не найдена", http.StatusNotFound)
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
		http.Error(w, "запись не найдена", http.StatusNotFound)
		return
	}
	if err != nil {
		internalError(w, "delete record", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /records", h.GetList)
	mux.HandleFunc("POST /records", h.Create)
	mux.HandleFunc("GET /records/{id}", h.GetByID)
	mux.HandleFunc("PUT /records/{id}", h.Update)
	mux.HandleFunc("DELETE /records/{id}", h.Delete)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "неверный id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func decodeInput(w http.ResponseWriter, r *http.Request) (RecordInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var in RecordInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "неверный JSON", http.StatusBadRequest)
		return RecordInput{}, false
	}
	if strings.TrimSpace(in.Name) == "" {
		http.Error(w, "name обязателен", http.StatusBadRequest)
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
	log.Printf("%s: %v", op, err)
	http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
}
