package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePinger struct{ err error }

func (p fakePinger) Ping(ctx context.Context) error { return p.err }

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"БД отвечает", nil, http.StatusOK, `{"status":"ok"}`},
		{"БД недоступна", errors.New("connection refused"), http.StatusServiceUnavailable, `{"status":"unavailable"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Handler(fakePinger{tt.pingErr})(rec, httptest.NewRequest("GET", "/healthz", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("статус = %d, ожидали %d", rec.Code, tt.wantStatus)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("тело = %s, ожидали %s", got, tt.wantBody)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	ok := httptest.NewServer(Handler(fakePinger{}))
	defer ok.Close()
	if err := Check(ok.URL); err != nil {
		t.Errorf("здоровый сервер: %v", err)
	}

	bad := httptest.NewServer(Handler(fakePinger{errors.New("down")}))
	defer bad.Close()
	if err := Check(bad.URL); err == nil {
		t.Error("сервер без БД: ожидали ошибку")
	}

	if err := Check("http://127.0.0.1:1/healthz"); err == nil {
		t.Error("сервер не запущен: ожидали ошибку")
	}
}
