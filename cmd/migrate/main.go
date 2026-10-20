package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"everroot/internal/dbhealth"
)

// MigrationAdvisoryLockKey is a unique 64-bit key used to coordinate concurrent
// migration processes via PostgreSQL session-level advisory locks.
// "EVERROOT" in ASCII hexadecimal representation.
const MigrationAdvisoryLockKey = int64(0x45564552524f4f54)

// loadDotEnv reads key=value pairs from a .env file if it exists.
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

// resolveMigrationDir locates the directory containing migration .sql files.
func resolveMigrationDir(preferred string) (string, error) {
	candidates := []string{}
	if preferred != "" {
		candidates = append(candidates, preferred)
	}
	candidates = append(candidates, "db/migrations", "../db/migrations", "../../db/migrations")

	for _, c := range candidates {
		info, err := os.Stat(c)
		if err == nil && info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("migrations directory not found; tried %v (run from repository root or set MIGRATIONS_DIR)", candidates)
}

// findMigrationFiles returns all .sql files in dir sorted in ascending alphabetical order.
func findMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .sql migration files found in %s", dir)
	}
	sort.Strings(files)
	return files, nil
}

// acquireAdvisoryLock obtains a session-level advisory lock in PostgreSQL to prevent concurrent runs.
func acquireAdvisoryLock(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", MigrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	return nil
}

// releaseAdvisoryLock releases the session-level advisory lock.
func releaseAdvisoryLock(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", MigrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("release advisory lock: %w", err)
	}
	return nil
}

// executeMigrationTx executes a single migration script and records its version inside an atomic transaction.
// If any statement or commit fails, the transaction is rolled back cleanly.
func executeMigrationTx(ctx context.Context, conn *pgx.Conn, version string, sqlContent string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		// Rollback is a safe no-op if the transaction has already committed.
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, sqlContent); err != nil {
		return fmt.Errorf("execute SQL statements: %w", err)
	}

	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("record migration version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// verifySchema checks that the essential reference tables exist and contain required baseline records.
func verifySchema(ctx context.Context, conn *pgx.Conn) (int, int, error) {
	var nakshatraCount, authorityCount int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM nakshatras").Scan(&nakshatraCount); err != nil {
		return 0, 0, fmt.Errorf("query nakshatras table: %w", err)
	}
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM tree_species_authority").Scan(&authorityCount); err != nil {
		return 0, 0, fmt.Errorf("query tree_species_authority table: %w", err)
	}
	if nakshatraCount == 0 {
		return 0, 0, errors.New("nakshatras table has 0 rows; expected authoritative reference records")
	}
	if authorityCount == 0 {
		return 0, 0, errors.New("tree_species_authority table has 0 rows; expected botanical reference records")
	}
	return nakshatraCount, authorityCount, nil
}

func main() {
	loadDotEnv(".env")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Prefer DIRECT_URL (direct unpooled connection for Neon migrations),
	// fallback to DATABASE_URL.
	targetURL := os.Getenv("DIRECT_URL")
	if targetURL == "" {
		targetURL = os.Getenv("DATABASE_URL")
	}
	if targetURL == "" {
		logger.Error("Either DIRECT_URL or DATABASE_URL is required to run migrations")
		os.Exit(1)
	}

	// Locate migrations directory
	migrationDir, err := resolveMigrationDir(os.Getenv("MIGRATIONS_DIR"))
	if err != nil {
		logger.Error("Migration directory resolution failed", "error", err)
		os.Exit(1)
	}

	files, err := findMigrationFiles(migrationDir)
	if err != nil {
		logger.Error("Migration file discovery failed", "error", err)
		os.Exit(1)
	}

	// Parse database config safely without leaking passwords or tokens in logs
	connCfg, err := pgx.ParseConfig(targetURL)
	if err != nil {
		logger.Error("Failed to parse database connection URL (ensure format is postgres://...)")
		os.Exit(1)
	}

	// Restrict the dial to IPv4 so the migrator can reach the endpoint at all;
	// this is the same host the API and the health checker use. cfg.Host is left
	// untouched so TLS verification and SNI keep using the hostname — the
	// dialer, host and TLS settings all live in the embedded pgconn.Config, so
	// that is what the helper takes. The timeout is carried over from the parsed
	// config rather than invented here, because this command bounds the whole
	// attempt with the context below.
	dbhealth.ApplyIPv4Dial(&connCfg.Config, connCfg.ConnectTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	conn, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		logger.Error("Failed to connect to database for migration",
			"host", connCfg.Host,
			"port", connCfg.Port,
			"database", connCfg.Database,
			"user", connCfg.User,
			"error", err,
		)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Acquire advisory lock to prevent concurrent migration executions
	logger.Info("Acquiring database migration advisory lock...")
	if err := acquireAdvisoryLock(ctx, conn); err != nil {
		logger.Error("Failed to acquire migration advisory lock", "error", err)
		os.Exit(1)
	}
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer unlockCancel()
		if err := releaseAdvisoryLock(unlockCtx, conn); err != nil {
			logger.Error("Failed to release migration advisory lock", "error", err)
		}
	}()

	// Ensure schema_migrations table exists
	_, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`)
	if err != nil {
		logger.Error("Failed to ensure schema_migrations table", "error", err)
		os.Exit(1)
	}

	appliedCount := 0
	for _, file := range files {
		version := strings.TrimSuffix(file, ".sql")

		var exists bool
		err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", version).Scan(&exists)
		if err != nil {
			logger.Error("Failed to check migration status", "version", version, "error", err)
			os.Exit(1)
		}
		if exists {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationDir, file))
		if err != nil {
			logger.Error("Failed to read migration file", "file", file, "error", err)
			os.Exit(1)
		}

		if err := executeMigrationTx(ctx, conn, version, string(content)); err != nil {
			logger.Error("Migration failed and rolled back cleanly", "version", version, "error", err)
			os.Exit(1)
		}

		logger.Info("Applied migration", "version", version)
		appliedCount++
	}

	if appliedCount == 0 {
		logger.Info("All migrations already up to date")
	} else {
		logger.Info("Successfully applied migrations", "count", appliedCount)
	}

	// Verify required reference tables
	nakshatraCount, authorityCount, err := verifySchema(ctx, conn)
	if err != nil {
		logger.Error("Schema verification failed after migrations", "error", err)
		os.Exit(1)
	}

	logger.Info("Schema verified",
		"nakshatras", nakshatraCount,
		"tree_species_authority", authorityCount,
	)
}
