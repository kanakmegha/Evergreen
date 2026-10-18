package handler

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"everroot/internal/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	handler http.Handler
	once    sync.Once
	initErr error
)

func initHandler() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dsn := os.Getenv("DATABASE_URL")
	var pool *pgxpool.Pool
	if dsn != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg, err := pgxpool.ParseConfig(dsn)
		if err == nil {
			cfg.MinConns = 0
			if maxConns := os.Getenv("DATABASE_MAX_CONNS"); maxConns != "" {
				if n, err := strconv.Atoi(maxConns); err == nil && n > 0 {
					cfg.MaxConns = int32(n)
				}
			} else {
				cfg.MaxConns = 10
			}
			cfg.MaxConnIdleTime = 5 * time.Minute
			cfg.MaxConnLifetime = 30 * time.Minute
			cfg.HealthCheckPeriod = 1 * time.Minute

			p, err := pgxpool.NewWithConfig(ctx, cfg)
			if err == nil {
				pool = p
			} else {
				logger.Warn("Database connection failed; running in AI-only mode without persistence", "error", err)
			}
		} else {
			logger.Warn("Failed to parse database configuration; running in AI-only mode", "error", err)
		}
	} else {
		logger.Info("DATABASE_URL is not set; running in AI-only mode without persistence")
	}

	h, err := httpapi.New(pool, logger)
	if err != nil {
		initErr = err
		logger.Error("API initialization failed", "error", err)
		return
	}
	handler = h
}

// Handler is the Vercel Go Serverless Function entry point.
// If deployed as a Vercel Function, Vercel routes incoming requests here.
// The connection pool is safely shared across warm invocations.
func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(initHandler)
	if initErr != nil {
		http.Error(w, "Service Unavailable: database or API initialization error", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}
