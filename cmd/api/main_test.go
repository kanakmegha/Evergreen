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

// The AI-backed endpoints answer inside the request, so the write deadline has
// to outlast the provider's whole worst case, not one round trip. Before this
// guard the server allowed 15s while the provider client allowed 25s, and a
// slow answer was killed mid-flight: the browser received an aborted connection
// (HTTP 000) rather than a response.
//
// The second half of the guard is the same failure one level up. The deadline
// was measured against a single ProviderTimeout, but the client retries a
// transient failure, so an attempt that timed out and then succeeded ran past
// the deadline and the connection was cut anyway. The visitor was told the
// service could not be reached for a call that had been working the whole
// time, which is worse than a slow answer because it is untrue.
func TestWriteTimeoutExceedsProviderTimeout(t *testing.T) {
	if writeTimeout() <= ai.ProviderTimeout {
		t.Fatalf("write timeout %s must exceed the provider timeout %s", writeTimeout(), ai.ProviderTimeout)
	}
	if writeTimeout() <= ai.WorstCaseResponseTime {
		t.Fatalf(
			"write timeout %s must exceed the provider worst case %s, or a retry that succeeds is killed mid-answer",
			writeTimeout(), ai.WorstCaseResponseTime,
		)
	}
}

// The worst case is spelled out rather than trusted, so that raising the
// attempt count or the per-attempt timeout cannot quietly leave the server
// deadline behind.
func TestWorstCaseCoversEveryAttemptAndBackoff(t *testing.T) {
	const attempts = 3
	const backoff = 2 * time.Second
	want := attempts*ai.ProviderTimeout + (attempts-1)*backoff
	if ai.WorstCaseResponseTime < want {
		t.Fatalf("worst case %s is shorter than %d attempts with backoff %s (%s)", ai.WorstCaseResponseTime, attempts, backoff, want)
	}
	if ai.WorstCaseResponseTime <= ai.ProviderTimeout {
		t.Fatalf("worst case %s must be more than one attempt %s", ai.WorstCaseResponseTime, ai.ProviderTimeout)
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

// The revision reported by /healthz must describe the binary, not the machine
// it happens to be running on. A value baked in by the build wins; the
// platform's own metadata is consulted only when the binary carries none; and
// with neither available the answer is "unknown".
//
// That last case is the one worth testing. This endpoint exists so a deployed
// host can be matched to a commit without trusting a dashboard, so a build that
// was made without the linker flag has to admit it — a guessed revision would
// be worse than none, because it would look like an answer.
func TestResolvedRevisionPrefersTheBinaryThenTheEnvironment(t *testing.T) {
	t.Setenv("EVERROOT_REVISION", "")
	t.Setenv("VERCEL_GIT_COMMIT_SHA", "")

	withBuild := revision
	t.Cleanup(func() { revision = withBuild })

	t.Run("build-time value wins", func(t *testing.T) {
		revision = "from-the-build"
		t.Setenv("EVERROOT_REVISION", "from-the-environment")
		if got := resolvedRevision(); got != "from-the-build" {
			t.Fatalf("resolvedRevision() = %q, want the build-time value", got)
		}
	})

	t.Run("environment is the fallback", func(t *testing.T) {
		revision = ""
		t.Setenv("VERCEL_GIT_COMMIT_SHA", "from-the-platform")
		if got := resolvedRevision(); got != "from-the-platform" {
			t.Fatalf("resolvedRevision() = %q, want the platform's build metadata", got)
		}
	})

	t.Run("unknown rather than empty", func(t *testing.T) {
		revision = "   "
		if got := resolvedRevision(); got != "unknown" {
			t.Fatalf("resolvedRevision() = %q, want unknown", got)
		}
	})
}
