// Command api runs the Everroot HTTP API.
//
// This is a long-running HTTP server, which is what the deployment platform
// runs and probes. It is not a serverless function: see README "Deploying" for
// why the repository does not also carry a function-style entry point.
package main

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"everroot/internal/ai"
	"everroot/internal/dbhealth"
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

// listenAddr decides where the HTTP server binds.
//
// PORT wins because that is the contract on every platform that runs this
// server: the platform assigns a port, probes it, and declares the deployment
// failed if nothing listens there in time. API_ADDR is a local-development
// convenience only. Letting API_ADDR take precedence, as it used to, meant a
// deployment that inherited API_ADDR=127.0.0.1:8080 from a local .env would
// bind the wrong port and never be detected as started, which is exactly the
// "server did not start listening" failure this fixes.
//
// When PORT is set the server binds all interfaces rather than loopback, so the
// platform's probe can reach it from outside the process.
func listenAddr() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return "0.0.0.0:" + port
	}
	if addr := strings.TrimSpace(os.Getenv("API_ADDR")); addr != "" {
		return addr
	}
	return "127.0.0.1:8080"
}

// connectTimeout bounds a single connection attempt.
//
// pgxpool's default is two minutes, which is longer than any request this API
// serves. DATABASE_CONNECT_TIMEOUT can raise or lower it; the same value is
// used by cmd/dbcheck so a local diagnosis describes the deployed behaviour.
func connectTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("DATABASE_CONNECT_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return dbhealth.DefaultConnectTimeout
}

// connectPool builds the database pool without connecting to the database.
//
// pgxpool is lazy, so this performs no network I/O and cannot delay startup. A
// missing, malformed or unreachable DATABASE_URL therefore surfaces later as a
// query error, which the reviewed-catalogue endpoints report honestly as
// unavailable instead of answering with built-in data.
func connectPool(logger *slog.Logger) *pgxpool.Pool {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		logger.Warn("DATABASE_URL is not set; reviewed catalogue endpoints will report themselves unavailable",
			"category", string(dbhealth.CategoryNotConfigured))
		return nil
	}

	// Describe the target once, credential-free, so an operator reading the log
	// can see which database was configured without the connection string.
	if target, err := dbhealth.Describe(dsn); err == nil {
		logger.Info("database target configured", target.LogArgs()...)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// The parse error can embed the connection string, so only the category
		// is logged.
		logger.Error("DATABASE_URL could not be parsed; reviewed catalogue endpoints will report themselves unavailable",
			"category", string(dbhealth.CategoryMalformedDSN))
		return nil
	}

	// Neon serverless compute suspends when idle; MinConns: 0 allows scale-to-zero.
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = connectTimeout()
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

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		logger.Error("database pool could not be created; reviewed catalogue endpoints will report themselves unavailable",
			"category", string(dbhealth.CategoryUnknown))
		return nil
	}
	return pool
}

// warmDatabase reports the database state once, in the background.
//
// It never blocks startup, and it never discards the pool. A database that is
// asleep (Neon scales to zero) or briefly unreachable can serve the next
// request normally, so throwing the pool away on the first failed ping would
// turn a temporary condition into a permanent one.
func warmDatabase(pool *pgxpool.Pool, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout())
	defer cancel()
	started := time.Now()
	if err := pool.Ping(ctx); err != nil {
		diag := dbhealth.Classify(err)
		// Only the classification is logged. The driver error can embed the
		// host and user from the connection string, so it is not echoed.
		logger.Warn("database is not reachable at startup; reviewed catalogue endpoints will report unavailable until it recovers",
			"category", string(diag.Category),
			"sqlstate", diag.SQLState,
			"retryable", diag.Retryable,
			"elapsed", time.Since(started).Round(time.Millisecond).String(),
			"remediation", diag.Remediation,
		)
		return
	}
	logger.Info("database reachable", "elapsed", time.Since(started).Round(time.Millisecond).String())
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

	// Bind before touching the database. A platform's startup probe has a
	// deadline, so doing network I/O first lets a slow or unreachable database
	// consume it and the deployment is declared failed even though the server
	// would have started.
	addr := listenAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listener failed", "addr", addr, "error", err)
		os.Exit(1)
	}

	pool := connectPool(logger)
	if pool != nil {
		defer pool.Close()
		go warmDatabase(pool, logger)
	}

	handler, err := httpapi.New(pool, logger)
	if err != nil {
		logger.Error("api initialization failed", "error", err)
		_ = ln.Close()
		os.Exit(2)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      writeTimeout(),
		IdleTimeout:       60 * time.Second,
	}
	logger.Info("Everroot API listening", "addr", ln.Addr().String())
	if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
