package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"go-education/internal/records"
	"go-education/migrations"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("не задана переменная окружения DATABASE_URL")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// ctx отменяется по Ctrl+C (SIGINT) или docker stop (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := migrations.Up(dsn); err != nil {
		log.Fatal(err)
	}
	log.Println("Миграции применены")

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("подключение к БД: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		log.Fatalf("БД недоступна: %v", err)
	}

	storage := records.NewStorage(db)
	handler := records.NewHandler(storage)

	mux := http.NewServeMux()
	handler.Register(mux)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Сервер запущен на http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("сервер: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Останавливаю сервер...")

	// Даём текущим запросам до 10 секунд на завершение.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("остановка сервера: %v", err)
	}
	log.Println("Сервер остановлен")
}
