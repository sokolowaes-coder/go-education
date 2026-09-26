package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"go-education/internal/health"
	"go-education/internal/middleware"
	"go-education/internal/records"
	"go-education/migrations"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// `server healthcheck` — проверка для Docker: в образе нет curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := health.Check("http://localhost:" + port + "/healthz"); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		return
	}

	logger := newLogger()
	slog.SetDefault(logger)

	if err := run(port, logger); err != nil {
		logger.Error("сервер завершился с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run(port string, logger *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("не задана переменная окружения DATABASE_URL")
	}

	// ctx отменяется по Ctrl+C (SIGINT) или docker stop (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := migrations.Up(dsn); err != nil {
		return err
	}
	logger.Info("миграции применены")

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return err
	}

	mux := http.NewServeMux()
	records.NewHandler(records.NewStorage(db)).Register(mux)
	mux.HandleFunc("GET /healthz", health.Handler(db))

	srv := &http.Server{
		Addr: ":" + port,
		Handler: middleware.Chain(mux,
			middleware.Logging(logger),
			middleware.Recover(logger),
		),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("сервер запущен", "addr", "http://localhost:"+port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}
	logger.Info("останавливаю сервер")

	// Даём текущим запросам до 10 секунд на завершение.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("сервер остановлен")
	return nil
}

// newLogger: LOG_LEVEL=debug|info|warn|error (по умолчанию info),
// LOG_FORMAT=json для машинного формата (по умолчанию текст).
func newLogger() *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	if os.Getenv("LOG_FORMAT") == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
