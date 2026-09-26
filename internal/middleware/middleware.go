// Package middleware — обёртки над http.Handler, общие для всех роутов.
package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Chain оборачивает h в middlewares; первый в списке выполняется первым.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// Logging пишет в лог каждый запрос: метод, путь, статус и время ответа.
// Запросы к /healthz пишутся на уровне DEBUG, чтобы healthcheck не засорял лог.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			level := slog.LevelInfo
			switch {
			case r.URL.Path == "/healthz" && rec.status < 500:
				level = slog.LevelDebug
			case rec.status >= 500:
				level = slog.LevelError
			}
			logger.Log(r.Context(), level, "request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration", time.Since(start).Round(time.Microsecond),
			)
		})
	}
}

// Recover перехватывает панику в хендлере: клиент получает 500,
// а в лог пишется ошибка со стеком. Без него паника обрывает соединение.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					if err == http.ErrAbortHandler {
						panic(err)
					}
					logger.Error("panic",
						"method", r.Method,
						"path", r.URL.Path,
						"error", err,
						"stack", string(debug.Stack()),
					)
					http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder запоминает статус ответа, чтобы Logging мог его записать.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

// Unwrap нужен http.ResponseController, чтобы добраться до исходного writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
