package dbhealth

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgx names only these two networks: "tcp" against a host, "unix" against a
// socket path. The mapping is the whole scope of this change, so it is pinned
// rather than left to be inferred from the dialer.
func TestIPv4NetworkRewritesOnlyTCP(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"tcp", "tcp4"},
		{"unix", "unix"},
		// Anything this function does not recognise is someone else's contract
		// and must survive untouched.
		{"tcp4", "tcp4"},
		{"tcp6", "tcp6"},
		{"", ""},
	} {
		if got := ipv4Network(tc.in); got != tc.want {
			t.Errorf("ipv4Network(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIPv4DialEnabledReadsTheOptOut(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", true},       // unset: the fix is the default
		{"   ", true},    // blank: still the default
		{"true", true},   //
		{"1", true},      //
		{"bogus", true},  // a typo fails toward the working behaviour
		{"false", false}, //
		{"FALSE", false}, // case-insensitive
		{"0", false},     //
		{"no", false},    //
		{"off", false},   //
		{" off ", false}, // surrounding space is trimmed
	} {
		t.Run("value="+tc.value, func(t *testing.T) {
			t.Setenv(IPv4DialEnvVar, tc.value)
			if got := IPv4DialEnabled(); got != tc.want {
				t.Fatalf("IPv4DialEnabled() = %v with %s=%q, want %v", got, IPv4DialEnvVar, tc.value, tc.want)
			}
		})
	}
}

// The reason for this whole helper is that the hostname has to survive the
// change. pgx builds the TLS configuration and the SNI server name from
// cfg.Host, not from the address it dialed, so a caller that "fixed" the dial
// by rewriting the host to an IPv4 literal would keep the TCP connection
// working and break certificate verification in production.
func TestApplyIPv4DialPreservesTheHostnameForTLSAndSNI(t *testing.T) {
	t.Setenv(IPv4DialEnvVar, "true")

	const host = "ep-cool-name-123456-pooler.us-east-2.aws.neon.tech"
	cfg := &pgconn.Config{
		Host:     host,
		Port:     5432,
		Database: "neondb",
		User:     "neondb_owner",
		Password: secretPassword,
		TLSConfig: &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		},
	}

	ApplyIPv4Dial(cfg, 10*time.Second)

	if cfg.Host != host {
		t.Fatalf("Host = %q, want %q: the hostname drives TLS verification and SNI", cfg.Host, host)
	}
	if cfg.TLSConfig == nil || cfg.TLSConfig.ServerName != host {
		t.Fatalf("TLSConfig.ServerName = %v, want %q", cfg.TLSConfig, host)
	}
	if cfg.Port != 5432 || cfg.Database != "neondb" || cfg.User != "neondb_owner" {
		t.Fatalf("conn identity changed: %+v", cfg)
	}
	if cfg.Password != secretPassword {
		t.Fatal("password was altered")
	}
	if cfg.DialFunc == nil {
		t.Fatal("DialFunc was not installed")
	}
}

// A nil config is reachable from a caller that failed to parse a DSN, so it has
// to be a no-op rather than a panic.
func TestApplyIPv4DialIgnoresNilConfig(t *testing.T) {
	ApplyIPv4Dial(nil, time.Second)
}

// The opt-out has to actually restore pgx's own dialer, not merely skip
// part of the change.
func TestApplyIPv4DialLeavesTheDialerAloneWhenDisabled(t *testing.T) {
	t.Setenv(IPv4DialEnvVar, "off")

	cfg := &pgconn.Config{Host: "example.test", Port: 5432}
	existing := cfg.DialFunc // nil here, and that is enough to detect a write

	ApplyIPv4Dial(cfg, time.Second)

	if (cfg.DialFunc == nil) != (existing == nil) {
		t.Fatal("DialFunc was replaced despite the opt-out")
	}
}

// Proving the restriction end to end: a listener reachable only over IPv6 must
// not be reachable through the configured dialer, and an IPv4 listener must be.
// The address is passed through untouched, which is what keeps DNS resolution
// and the hostname in the hands of pgx.
func TestApplyIPv4DialReachesIPv4AndSkipsIPv6(t *testing.T) {
	t.Setenv(IPv4DialEnvVar, "true")

	cfg := &pgconn.Config{Host: "example.test", Port: 5432}
	ApplyIPv4Dial(cfg, 3*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("IPv4 listener is reachable", func(t *testing.T) {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Skipf("no IPv4 loopback listener available: %v", err)
		}
		defer ln.Close()
		go func() {
			conn, err := ln.Accept()
			if err == nil {
				_ = conn.Close()
			}
		}()

		conn, err := cfg.DialFunc(ctx, "tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("dial over tcp4 to an IPv4 listener failed: %v", err)
		}
		_ = conn.Close()
	})

	t.Run("IPv6-only listener is not reachable", func(t *testing.T) {
		ln, err := net.Listen("tcp6", "[::1]:0")
		if err != nil {
			t.Skipf("IPv6 loopback not available in this environment: %v", err)
		}
		defer ln.Close()

		// A tcp4 dial cannot be answered by a tcp6-only socket, so this must
		// fail rather than silently fall back to the dual-stack dialer.
		conn, err := cfg.DialFunc(ctx, "tcp", ln.Addr().String())
		if err == nil {
			_ = conn.Close()
			t.Fatal("dial reached an IPv6-only listener; the dialer is not restricted to IPv4")
		}
	})
}

// pgx already bounds the whole connection attempt by the context, so a
// configured timeout must not shorten or remove that bound — it is a second
// line of defence for a caller that passes an undecorated context.
func TestApplyIPv4DialBoundsTheDialWhenTheContextIsUnbounded(t *testing.T) {
	t.Setenv(IPv4DialEnvVar, "true")

	cfg := &pgconn.Config{Host: "example.test", Port: 5432}
	ApplyIPv4Dial(cfg, 200*time.Millisecond)

	// TEST-NET-1 (RFC 5737) is reserved for documentation and is not routed,
	// so the dial cannot complete.
	started := time.Now()
	_, err := cfg.DialFunc(context.Background(), "tcp", "192.0.2.1:5432")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("dial to a documentation-only address unexpectedly succeeded")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("dial took %s despite a 200ms bound", elapsed)
	}
}
