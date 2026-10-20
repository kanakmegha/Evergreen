package dbhealth

import (
	"context"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// IPv4DialEnvVar opts a deployment out of IPv4-only dialing.
//
// The default is to dial IPv4, because that is the network this project's
// PostgreSQL endpoint is reachable over; see ApplyIPv4Dial for the failure it
// fixes. Setting this to 0, false, no or off restores pgx's dual-stack dialer
// for an IPv6-only network, rather than leaving that operator to patch the
// binary.
const IPv4DialEnvVar = "DATABASE_FORCE_IPV4"

// IPv4DialEnabled reports whether PostgreSQL connections should be dialed over
// IPv4 only.
//
// An unset variable means enabled, so the fix reaches a deployment that has not
// been reconfigured. Only the four spellings of "no" disable it: a typo in a
// security-relevant network setting should fail toward the behaviour that is
// known to work, not toward a silent fallback that reproduces the original
// timeout.
func IPv4DialEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(IPv4DialEnvVar))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// ApplyIPv4Dial makes cfg open PostgreSQL connections over IPv4 only, when the
// deployment has not opted out.
//
// Why this exists: with pgx's default dialer the connection timed out while
// dialing this endpoint. `nc -4` reached the host and `nc -6` did not, and a
// build whose DialFunc was restricted to tcp4 connected in about two and a half
// seconds. The dual-stack dialer spends the connect budget on addresses that
// never answer, so the attempt fails even though an IPv4 route is available the
// whole time.
//
// What this changes, and deliberately nothing else:
//
//   - Only the network passed to net.Dialer is rewritten, tcp to tcp4. The
//     address, and therefore the resolved IP actually contacted, still comes
//     from pgx.
//   - cfg.Host is left exactly as parsed. That matters beyond tidiness: pgx
//     derives TLS certificate verification and the SNI server name from
//     cfg.Host, not from the address it dialed. Rewriting the host to an IPv4
//     literal would keep the TCP connection working and silently break the
//     certificate check, which is the sort of fix that holds up in a smoke test
//     and fails in production.
//   - The resolver is untouched, so DNS still returns every record; the
//     restriction applies to the connection attempt.
//   - Anything that is not tcp — pgx asks for unix against a socket path — is
//     passed through unchanged.
//
// The connect timeout keeps its existing meaning. pgx enforces ConnectTimeout
// as a deadline over the whole connection attempt, dialing included, so this
// does not weaken it; the dialer is also given the same bound so that a caller
// passing an undecorated context cannot hang in the dial. SSL mode, channel
// binding, the TLS configuration and the CA pool are configuration that arrives
// after this call and are therefore unaffected.
//
// A nil config is accepted and ignored, so a caller that failed to parse a DSN
// can call this unconditionally.
func ApplyIPv4Dial(cfg *pgconn.Config, connectTimeout time.Duration) {
	if cfg == nil || !IPv4DialEnabled() {
		return
	}
	dialer := &net.Dialer{}
	if connectTimeout > 0 {
		dialer.Timeout = connectTimeout
	}
	cfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, ipv4Network(network), addr)
	}
}

// ipv4Network maps a network name onto its IPv4-only form.
//
// pgx asks for "tcp" against a host and "unix" against a socket path, and never
// names an address family itself. Only "tcp" is rewritten: a unix socket has no
// address family to prefer, and a name this function does not recognise is
// someone else's contract, so it is passed through rather than guessed at.
func ipv4Network(network string) string {
	if network == "tcp" {
		return "tcp4"
	}
	return network
}
