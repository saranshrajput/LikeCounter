package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saranshrajput/likecounter/internal/api"
	"github.com/saranshrajput/likecounter/internal/likes"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := pgxpool.ParseConfig(env("DATABASE_URL",
		"postgres://like_counter:like_counter@localhost:5438/like_counter"))
	if err != nil {
		log.Fatal(err)
	}
	cfg.MaxConns = 10 // same cap as Hikari / pg.Pool defaults in the other versions

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := likes.InitDatabase(ctx, pool, true); err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              ":" + env("PORT", "3002"),
		Handler:           api.New(likes.NewService(pool)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("like-counter listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
