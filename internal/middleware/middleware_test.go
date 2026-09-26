package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoggingWritesRequest(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		handler   http.HandlerFunc
		wantLevel string
		wantInLog string
	}{
		{
			name:      "статус из WriteHeader",
			path:      "/records",
			handler:   func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) },
			wantLevel: "level=INFO",
			wantInLog: "status=201",
		},
		{
			name:      "без WriteHeader — 200",
			path:      "/records",
			handler:   func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) },
			wantLevel: "level=INFO",
			wantInLog: "status=200",
		},
		{
			name:      "5xx пишется как ошибка",
			path:      "/records",
			handler:   func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantLevel: "level=ERROR",
			wantInLog: "status=500",
		},
		{
			name:      "healthz — только DEBUG",
			path:      "/healthz",
			handler:   func(w http.ResponseWriter, r *http.Request) {},
			wantLevel: "level=DEBUG",
			wantInLog: "path=/healthz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := Logging(newTestLogger(&buf))(tt.handler)
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", tt.path, nil))

			log := buf.String()
			for _, want := range []string{tt.wantLevel, tt.wantInLog, "method=POST", "duration="} {
				if !strings.Contains(log, want) {
					t.Errorf("в логе нет %q:\n%s", want, log)
				}
			}
		})
	}
}

func TestRecoverReturns500(t *testing.T) {
	var buf bytes.Buffer
	h := Recover(newTestLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("что-то сломалось")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/records", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("статус = %d, ожидали 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "что-то сломалось") {
		t.Errorf("текст паники утёк клиенту: %s", rec.Body)
	}
	if log := buf.String(); !strings.Contains(log, "что-то сломалось") || !strings.Contains(log, "stack=") {
		t.Errorf("в логе нет паники со стеком:\n%s", log)
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}), mw("first"), mw("second"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	if got := strings.Join(order, ","); got != "first,second,handler" {
		t.Errorf("порядок = %s", got)
	}
}

// Паника под Logging+Recover должна попасть в лог запроса как 500.
func TestLoggingSeesRecoveredPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("бум")
	}), Logging(logger), Recover(logger))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/records", nil))

	if !strings.Contains(buf.String(), "msg=request") || !strings.Contains(buf.String(), "status=500") {
		t.Errorf("в логе нет запроса со статусом 500:\n%s", buf.String())
	}
}
