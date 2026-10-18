package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"everroot/internal/ai"
	"everroot/internal/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

// loadDotEnv reads key=value pairs from a .env file if it exists and sets
// them in the environment only if they are not already set.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

// createPool initializes a connection pool tuned for Neon and standard PostgreSQL.
// It sets MinConns=0 so idle serverless instances can scale-to-zero when unused.
func createPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}

	// Neon serverless compute suspends when idle; MinConns: 0 allows scale-to-zero.
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

	return pgxpool.NewWithConfig(ctx, cfg)
}

// writeTimeout bounds how long the server will spend answering one request. It
// must exceed the AI provider timeout, because /api/v1/explain and
// /api/v1/questions answer inside the request: a shorter write deadline kills
// the connection mid-answer, and the browser receives no response at all
// instead of a slow one.
func writeTimeout() time.Duration { return ai.ProviderTimeout + 20*time.Second }

func main() {
	loadDotEnv(".env")
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dsn := os.Getenv("DATABASE_URL")
	var pool *pgxpool.Pool
	if dsn == "" {
		logger.Warn("DATABASE_URL is not set; starting in AI-only mode (persistence offline)")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var err error
		pool, err = createPool(ctx, dsn)
		cancel()
		if err != nil {
			logger.Warn("Database connection failed; starting in AI-only mode without persistence", "error", err)
			pool = nil
		} else {
			pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := pool.Ping(pingCtx); err != nil {
				logger.Warn("Database ping failed; starting in AI-only mode without persistence", "error", err)
				pool.Close()
				pool = nil
			}
			pingCancel()
		}
	}
	if pool != nil {
		defer pool.Close()
	}

	addr := os.Getenv("API_ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			addr = ":" + port
		} else {
			addr = "127.0.0.1:8080"
		}
	}
	handler, err := httpapi.New(pool, logger)
	if err != nil {
		logger.Error("api initialization failed", "error", err)
		os.Exit(2)
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: writeTimeout(), IdleTimeout: 60 * time.Second}
	logger.Info("Everroot API listening", "addr", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
