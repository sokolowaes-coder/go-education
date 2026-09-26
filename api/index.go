// Package handler — точка входа для Vercel. Vercel не держит сервер запущенным,
// а на каждый запрос вызывает функцию Handler. Все пути направляются сюда
// правилом rewrites в vercel.json, дальше их разбирает обычный роутер приложения.
package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	_ "time/tzdata"

	"go-education/internal/app"
)

var (
	mu      sync.Mutex
	handler http.Handler
)

// getHandler собирает приложение при первом запросе («холодный старт»)
// и переиспользует его, пока Vercel держит экземпляр функции живым.
// Если БД была недоступна, следующий запрос попробует снова.
func getHandler(ctx context.Context) (http.Handler, error) {
	mu.Lock()
	defer mu.Unlock()
	if handler != nil {
		return handler, nil
	}

	logger := app.NewLogger()
	slog.SetDefault(logger)
	cfg, err := app.ConfigFromEnv(logger)
	if err != nil {
		return nil, err
	}
	h, _, err := app.New(ctx, cfg) // пул живёт вместе с экземпляром функции
	if err != nil {
		return nil, err
	}
	handler = h
	return handler, nil
}

func Handler(w http.ResponseWriter, r *http.Request) {
	h, err := getHandler(r.Context())
	if err != nil {
		slog.Error("запуск приложения", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"success":false,"error":"сервис временно недоступен"}` + "\n"))
		return
	}
	h.ServeHTTP(w, r)
}
