// Package dbhealth classifies PostgreSQL connection failures and reports a
// credential-free description of the database target.
//
// The point of this package is that a database outage has to be diagnosable
// from the deployment logs without ever printing the connection string. The
// connection string carries the Neon password, so nothing here renders it: the
// target is reduced to host, port, database, user and whether a password is
// present, and a failure is reduced to a category and, when PostgreSQL itself
// answered, the SQLSTATE and the server's own message. Client-side strings,
// which for pgx can embed the host and user from the DSN, are deliberately not
// carried into the report.
package dbhealth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DefaultConnectTimeout bounds a single connection attempt.
//
// Neon compute suspends when idle, so the first connection after a quiet period
// has to wait for it to wake. pgxpool's own default is two minutes, which is
// far too long for a request path, and the previous startup probe allowed only
// five seconds, which is close enough to a cold start that a sleeping project
// could be mistaken for a broken one. Ten seconds is the compromise: long
// enough for the usual wake, short enough that a genuinely unreachable host
// fails promptly.
const DefaultConnectTimeout = 10 * time.Second

// Target is a description of a PostgreSQL target with the password removed.
type Target struct {
	Host        string
	Port        uint16
	Database    string
	User        string
	HasPassword bool
	SSLMode     string
	// Pooled reports whether the host looks like a PgBouncer pooled endpoint.
	// Neon exposes both a pooled and a direct endpoint; the application must
	// use the pooled one and migrations the direct one.
	Pooled bool
}

// String renders the target so it is safe to write to a log.
func (t Target) String() string {
	return fmt.Sprintf("host=%s port=%d database=%s user=%s password=%s sslmode=%s pooled=%t",
		t.Host, t.Port, t.Database, t.User, t.passwordState(), t.sslModeOrDefault(), t.Pooled)
}

// LogArgs returns the target as slog key/value pairs. A password is reported
// only as set or unset.
func (t Target) LogArgs() []any {
	return []any{
		"host", t.Host,
		"port", t.Port,
		"database", t.Database,
		"user", t.User,
		"password", t.passwordState(),
		"sslmode", t.sslModeOrDefault(),
		"pooled", t.Pooled,
	}
}

func (t Target) passwordState() string {
	if t.HasPassword {
		return "set"
	}
	return "unset"
}

func (t Target) sslModeOrDefault() string {
	if t.SSLMode == "" {
		return "prefer"
	}
	return t.SSLMode
}

// Describe parses a connection string and returns a credential-free target.
// It never returns or embeds the password.
func Describe(dsn string) (Target, error) {
	cfg, err := pgconn.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return Target{}, err
	}
	t := Target{
		Host:        cfg.Host,
		Port:        cfg.Port,
		Database:    cfg.Database,
		User:        cfg.User,
		HasPassword: cfg.Password != "",
		SSLMode:     sslModeFromDSN(dsn),
	}
	// Neon's pooled endpoint carries "-pooler" in the host. This is only a
	// hint for the person reading the log; it is never used to decide routing.
	t.Pooled = strings.Contains(t.Host, "-pooler")
	return t, nil
}

// sslModeFromDSN reads sslmode out of the connection string as written, so the
// report shows what was configured rather than what libpq defaulted to.
func sslModeFromDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if u, err := url.Parse(dsn); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		return u.Query().Get("sslmode")
	}
	for _, field := range strings.Fields(dsn) {
		k, v, ok := strings.Cut(field, "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), "sslmode") {
			return strings.Trim(strings.TrimSpace(v), `'"`)
		}
	}
	return ""
}

// Category is a coarse, stable classification of a connection failure.
type Category string

const (
	CategoryOK                Category = "ok"
	CategoryNotConfigured     Category = "not_configured"
	CategoryMalformedDSN      Category = "malformed_dsn"
	CategoryDNS               Category = "dns"
	CategoryConnectionRefused Category = "connection_refused"
	CategoryTimeout           Category = "timeout"
	CategoryTLS               Category = "tls"
	CategoryAuthentication    Category = "authentication"
	CategoryUnknownDatabase   Category = "unknown_database"
	CategoryPermissionDenied  Category = "permission_denied"
	CategorySchemaMissing     Category = "schema_missing"
	CategoryServerError       Category = "server_error"
	CategoryUnknown           Category = "unknown"
)

// Diagnosis is a credential-free explanation of a connection failure.
type Diagnosis struct {
	Category Category
	// SQLState is set only when PostgreSQL itself reported the error.
	SQLState string
	// ServerMessage is PostgreSQL's own message, when the server answered.
	// Client-side error strings are not copied here, because a pgx connection
	// error embeds the host and user from the DSN.
	ServerMessage string
	Retryable     bool
	Remediation   string
}

// Classify explains a connection or query failure without exposing the
// connection string.
func Classify(err error) Diagnosis {
	if err == nil {
		return Diagnosis{Category: CategoryOK, Retryable: true}
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return classifyServerError(pgErr)
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return Diagnosis{
			Category:  CategoryTimeout,
			Retryable: true,
			Remediation: "The connection attempt ran out of time. Neon suspends idle compute, so the first " +
				"connection after a quiet period can be slow; a retry usually succeeds. If it never does, " +
				"check that the project and branch are active and that the host is reachable.",
		}
	case errors.Is(err, context.Canceled):
		return Diagnosis{
			Category:    CategoryTimeout,
			Retryable:   true,
			Remediation: "The connection attempt was cancelled by the caller, not by the database.",
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Diagnosis{
			Category:    CategoryTimeout,
			Retryable:   true,
			Remediation: "The network connection timed out. Retry; if it persists, confirm the host and that egress is allowed.",
		}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return Diagnosis{
			Category:  CategoryDNS,
			Retryable: false,
			Remediation: "The hostname did not resolve. The Neon endpoint is probably wrong, or the project or " +
				"branch was deleted. Re-copy the connection string from the Neon dashboard.",
		}
	}

	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return Diagnosis{
			Category:  CategoryConnectionRefused,
			Retryable: true,
			Remediation: "The host refused the connection. The endpoint exists but is not accepting connections: " +
				"check the port and that the Neon compute is not suspended or disabled.",
		}
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		return Diagnosis{
			Category:    CategoryConnectionRefused,
			Retryable:   true,
			Remediation: "The network route to the database failed. Retry; if it persists, check the endpoint and egress rules.",
		}
	}

	if isTLS(err) {
		return Diagnosis{
			Category:  CategoryTLS,
			Retryable: false,
			Remediation: "TLS negotiation failed. Neon requires TLS, so append sslmode=require to DATABASE_URL " +
				"and make sure nothing is terminating or inspecting the connection.",
		}
	}

	if strings.Contains(strings.ToLower(err.Error()), "no such host") {
		return Diagnosis{
			Category:    CategoryDNS,
			Retryable:   false,
			Remediation: "The hostname did not resolve. Re-copy the connection string from the Neon dashboard.",
		}
	}

	return Diagnosis{
		Category:    CategoryUnknown,
		Retryable:   true,
		Remediation: "The failure is not one of the recognised categories. Run the bounded connection test (cmd/dbcheck) for a fuller report.",
	}
}

// classifyServerError maps a PostgreSQL SQLSTATE to a category and a next step.
func classifyServerError(pgErr *pgconn.PgError) Diagnosis {
	d := Diagnosis{SQLState: pgErr.Code, ServerMessage: pgErr.Message}
	code := pgErr.Code

	switch {
	case code == "3D000":
		d.Category = CategoryUnknownDatabase
		d.Remediation = "The database named in DATABASE_URL does not exist on this branch. Re-copy the connection string."
	case code == "42P01":
		d.Category = CategorySchemaMissing
		d.Remediation = "The connection works but a required table is missing. Apply the migrations with " +
			"`DIRECT_URL='…' go run ./cmd/migrate`; the API never migrates on start-up."
	case code == "42501":
		d.Category = CategoryPermissionDenied
		d.Remediation = "The role connected successfully but lacks a privilege. Check the role in the Neon dashboard."
	case strings.HasPrefix(code, "28"):
		d.Category = CategoryAuthentication
		d.Remediation = "Authentication failed. The user or password in DATABASE_URL is wrong or was rotated; " +
			"re-copy the connection string. This is not a schema problem."
	case strings.HasPrefix(code, "08"):
		d.Category = CategoryServerError
		d.Retryable = true
		d.Remediation = "PostgreSQL reported a connection exception. Retry; the server may still be starting."
	case code == "53300":
		d.Category = CategoryServerError
		d.Retryable = true
		d.Remediation = "The server has too many connections. Lower DATABASE_MAX_CONNS or wait and retry."
	case code == "57P03":
		d.Category = CategoryServerError
		d.Retryable = true
		d.Remediation = "PostgreSQL is starting up and cannot accept connections yet. Retry shortly."
	default:
		d.Category = CategoryServerError
		d.Retryable = pgconn.SafeToRetry(pgErr)
		d.Remediation = "PostgreSQL rejected the connection. The SQLSTATE and message above identify the cause."
	}
	return d
}

// isTLS reports whether an error looks like a TLS handshake or certificate
// failure. It is checked after the structured cases because a TLS failure can
// arrive wrapped in several layers.
func isTLS(err error) bool {
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return true
	}
	var certVerifyErr *tls.CertificateVerificationError
	if errors.As(err, &certVerifyErr) {
		return true
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return true
	}
	var authorityErr x509.UnknownAuthorityError
	if errors.As(err, &authorityErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"tls:", "x509:", "ssl is not enabled", "server does not support ssl", "ssl error", "certificate"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// Report is the result of a bounded connection test. Every field is safe to
// print; the raw driver error is deliberately not retained.
type Report struct {
	Target    Target
	Reachable bool
	SchemaOK  bool
	Latency   time.Duration
	Diag      Diagnosis
}

// Summary renders the report as the lines a person should read in a terminal.
func (r Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "target: %s\n", r.Target)
	fmt.Fprintf(&b, "result: reachable=%t schema_present=%t latency=%s\n", r.Reachable, r.SchemaOK, r.Latency.Round(time.Millisecond))
	fmt.Fprintf(&b, "diagnosis: category=%s sqlstate=%s retryable=%t\n", r.Diag.Category, orNone(r.Diag.SQLState), r.Diag.Retryable)
	if r.Diag.ServerMessage != "" {
		fmt.Fprintf(&b, "server: %s\n", r.Diag.ServerMessage)
	}
	if r.Diag.Remediation != "" {
		fmt.Fprintf(&b, "next step: %s\n", r.Diag.Remediation)
	}
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// Probe runs a bounded, read-only connection test.
//
// It opens a single short-lived connection rather than a pool, so it can be run
// against a production database without leaving anything behind and without
// changing any data. It answers two separate questions: can we connect at all,
// and does the schema exist. Those are different failures and the report keeps
// them apart.
func Probe(ctx context.Context, dsn string, connectTimeout time.Duration) Report {
	started := time.Now()
	if strings.TrimSpace(dsn) == "" {
		return Report{
			Diag: Diagnosis{
				Category:    CategoryNotConfigured,
				Retryable:   false,
				Remediation: "DATABASE_URL is not set. Add it to the backend project; the reviewed catalogue cannot be served without it.",
			},
		}
	}

	target, err := Describe(dsn)
	if err != nil {
		return Report{
			Diag: Diagnosis{
				Category:    CategoryMalformedDSN,
				Retryable:   false,
				Remediation: "DATABASE_URL is not a valid PostgreSQL connection string. Re-copy it from the Neon dashboard; it should begin with postgres:// or postgresql://.",
			},
		}
	}

	if connectTimeout <= 0 {
		connectTimeout = DefaultConnectTimeout
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return Report{
			Target: target,
			Diag: Diagnosis{
				Category:    CategoryMalformedDSN,
				Retryable:   false,
				Remediation: "DATABASE_URL could not be parsed. Re-copy it from the Neon dashboard.",
			},
		}
	}
	cfg.ConnectTimeout = connectTimeout

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return Report{
			Target:  target,
			Latency: time.Since(started),
			Diag:    Classify(err),
		}
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	defer func() { _ = conn.Close(closeCtx) }()

	report := Report{Target: target, Reachable: true, Latency: time.Since(started)}

	var schemaPresent bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.nakshatras') IS NOT NULL`).Scan(&schemaPresent); err != nil {
		report.Diag = Classify(err)
		return report
	}
	report.SchemaOK = schemaPresent
	if !schemaPresent {
		report.Diag = Diagnosis{
			Category:    CategorySchemaMissing,
			Retryable:   false,
			Remediation: "The database is reachable and authentication works, but the required tables are missing. Apply the migrations with `DIRECT_URL='…' go run ./cmd/migrate`.",
		}
		return report
	}
	report.Diag = Diagnosis{Category: CategoryOK, Retryable: true}
	return report
}
