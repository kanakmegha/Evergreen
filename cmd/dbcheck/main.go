// Command dbcheck runs a bounded, read-only connection test against the
// database named by an environment variable and prints a credential-free
// diagnosis.
//
// It exists because "the database is unreachable" is too coarse to act on. The
// report separates the failure classes that need different fixes: a wrong or
// deleted host, a rejected password, a connection that timed out while Neon's
// compute was waking, a TLS mismatch, and a reachable database whose schema was
// never migrated. It never writes, never migrates, and never prints the
// connection string.
//
// Usage:
//
//	go run ./cmd/dbcheck              # tests DATABASE_URL, the application target
//	go run ./cmd/dbcheck DIRECT_URL   # tests the unpooled migration target
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"everroot/internal/dbhealth"
)

// loadDotEnv reads key=value pairs from a .env file if it exists and sets them
// in the environment only if they are not already set, matching cmd/api and
// cmd/migrate.
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

// connectTimeout reads the same bound the server uses, so a diagnosis here
// describes the deployed behaviour rather than a different one.
func connectTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("DATABASE_CONNECT_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return dbhealth.DefaultConnectTimeout
}

func main() {
	loadDotEnv(".env")

	envName := "DATABASE_URL"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		envName = strings.TrimSpace(os.Args[1])
	}

	timeout := connectTimeout()
	// Bound the whole test, not just one dial: the probe also runs the schema
	// query, and neither may hang a terminal.
	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()

	report := dbhealth.Probe(ctx, strings.TrimSpace(os.Getenv(envName)), timeout)

	fmt.Printf("source env: %s\n", envName)
	fmt.Print(report.Summary())

	if !report.Reachable || !report.SchemaOK {
		os.Exit(1)
	}
}
