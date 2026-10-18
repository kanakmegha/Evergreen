package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"everroot/internal/ai"
	"everroot/internal/dbhealth"
)

// /api/v1/explain and /api/v1/questions answer inside the request, so the write
// deadline has to outlast a single inference round trip. Before this guard the
// server allowed 15s while the provider client allowed 25s, and a slow answer
// was killed mid-flight: the browser received an aborted connection (HTTP 000)
// rather than a response.
func TestWriteTimeoutExceedsProviderTimeout(t *testing.T) {
	if writeTimeout() <= ai.ProviderTimeout {
		t.Fatalf("write timeout %s must exceed the provider timeout %s", writeTimeout(), ai.ProviderTimeout)
	}
}

// The platform assigns the port and probes it; the deployment is declared
// failed if nothing listens there. API_ADDR is a local-development convenience,
// so PORT must win. When API_ADDR won, a deployment that inherited
// API_ADDR=127.0.0.1:8080 from a local .env bound the wrong port and was
// reported as "server did not start listening".
func TestListenAddrPrefersPlatformPort(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("API_ADDR", "")
	if got := listenAddr(); got != "127.0.0.1:8080" {
		t.Fatalf("default listen address = %q", got)
	}

	t.Setenv("API_ADDR", "127.0.0.1:9000")
	if got := listenAddr(); got != "127.0.0.1:9000" {
		t.Fatalf("API_ADDR was not used for local development: %q", got)
	}

	// The platform's port must override a stale local API_ADDR.
	t.Setenv("PORT", "3000")
	if got := listenAddr(); got != "0.0.0.0:3000" {
		t.Fatalf("PORT did not override API_ADDR: %q", got)
	}
	if !strings.HasPrefix(listenAddr(), "0.0.0.0:") {
		t.Fatalf("a platform port must bind all interfaces, not loopback: %q", listenAddr())
	}
}

// A failed startup ping must not throw the pool away. pgxpool connects lazily,
// so a Neon compute that is still waking, or a brief network fault, recovers on
// the next request. An earlier version closed the pool here, which turned a
// temporary outage into a deployment that could never reach the database again.
func TestPoolSurvivesFailedStartupPing(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:secret@127.0.0.1:1/db?sslmode=disable")
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "1s")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	pool := connectPool(logger)
	if pool == nil {
		t.Fatal("connectPool must return a lazy pool even when the database is unreachable")
	}
	defer pool.Close()

	// warmDatabase must return promptly rather than block on the startup path,
	// and must leave the pool open.
	warmDatabase(pool, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := pool.Ping(ctx)
	if err == nil {
		t.Fatal("expected an unreachable database to fail the ping")
	}
	if strings.Contains(err.Error(), "closed pool") {
		t.Fatalf("warmDatabase must not close the pool: %v", err)
	}
}

func TestConnectTimeoutDefaultAndOverride(t *testing.T) {
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "")
	if got := connectTimeout(); got != dbhealth.DefaultConnectTimeout {
		t.Fatalf("default connect timeout = %s, want %s", got, dbhealth.DefaultConnectTimeout)
	}
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "3s")
	if got := connectTimeout(); got != 3*time.Second {
		t.Fatalf("override connect timeout = %s", got)
	}
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "not-a-duration")
	if got := connectTimeout(); got != dbhealth.DefaultConnectTimeout {
		t.Fatalf("an invalid override must fall back to the default, got %s", got)
	}
}
