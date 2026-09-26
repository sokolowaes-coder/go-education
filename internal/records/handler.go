package records

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	storage *Storage
}

func NewHandler(storage *Storage) *Handler {
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
	name, ok := decodeName(w, r)
	if !ok {
		return
	}
	rec, err := h.storage.Create(r.Context(), name)
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
	name, ok := decodeName(w, r)
	if !ok {
		return
	}
	rec, err := h.storage.Update(r.Context(), id, name)
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

func decodeName(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "неверный JSON", http.StatusBadRequest)
		return "", false
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name обязателен", http.StatusBadRequest)
		return "", false
	}
	return req.Name, true
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
