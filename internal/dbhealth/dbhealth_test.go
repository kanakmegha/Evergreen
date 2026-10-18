package dbhealth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// The connection string carries the Neon password. Nothing this package
// renders may contain it, so these tests assert on the rendered output rather
// than trusting the type.
const secretPassword = "sup3r-s3cret-pw"

const neonDSN = "postgres://neondb_owner:" + secretPassword + "@ep-cool-name-123456-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require"

func TestDescribeNeverExposesPassword(t *testing.T) {
	target, err := Describe(neonDSN)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if target.Host != "ep-cool-name-123456-pooler.us-east-2.aws.neon.tech" {
		t.Fatalf("host = %q", target.Host)
	}
	if target.Port != 5432 {
		t.Fatalf("port = %d", target.Port)
	}
	if target.Database != "neondb" {
		t.Fatalf("database = %q", target.Database)
	}
	if target.User != "neondb_owner" {
		t.Fatalf("user = %q", target.User)
	}
	if !target.HasPassword {
		t.Fatalf("HasPassword = false")
	}
	if target.SSLMode != "require" {
		t.Fatalf("sslmode = %q", target.SSLMode)
	}
	if !target.Pooled {
		t.Fatalf("a -pooler host must be reported as pooled")
	}

	for _, rendered := range []string{
		target.String(),
		fmt.Sprintf("%v", target),
		fmt.Sprintf("%+v", target),
		fmt.Sprint(target.LogArgs()),
	} {
		if strings.Contains(rendered, secretPassword) {
			t.Fatalf("rendered target leaked the password: %s", rendered)
		}
	}
	if !strings.Contains(target.String(), "password=set") {
		t.Fatalf("target should report the password as set: %s", target.String())
	}
}

func TestDescribeReadsSSLModeAsWritten(t *testing.T) {
	cases := []struct {
		dsn  string
		want string
	}{
		{"postgres://u:p@h.example/db?sslmode=disable", "disable"},
		{"postgresql://u:p@h.example/db?sslmode=verify-full", "verify-full"},
		{"host=h.example user=u password=p dbname=db sslmode=require", "require"},
		{"host=h.example user=u sslmode='require'", "require"},
		{"host=h.example user=u", ""},
	}
	for _, c := range cases {
		target, err := Describe(c.dsn)
		if err != nil {
			t.Fatalf("Describe(%q): %v", c.dsn, err)
		}
		if target.SSLMode != c.want {
			t.Fatalf("Describe(%q) sslmode = %q, want %q", c.dsn, target.SSLMode, c.want)
		}
	}
}

func TestDescribeReportsDirectEndpointAsUnpooled(t *testing.T) {
	target, err := Describe("postgres://u:p@ep-cool-name-123456.us-east-2.aws.neon.tech/neondb?sslmode=require")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if target.Pooled {
		t.Fatalf("a direct host must not be reported as pooled")
	}
}

func TestClassifyCategories(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		category Category
		sqlstate string
		retry    bool
	}{
		{"nil", nil, CategoryOK, "", true},
		{"invalid password", &pgconn.PgError{Code: "28P01", Message: "password authentication failed"}, CategoryAuthentication, "28P01", false},
		{"invalid authorization", &pgconn.PgError{Code: "28000", Message: "no pg_hba entry"}, CategoryAuthentication, "28000", false},
		{"unknown database", &pgconn.PgError{Code: "3D000", Message: "database does not exist"}, CategoryUnknownDatabase, "3D000", false},
		{"undefined table", &pgconn.PgError{Code: "42P01", Message: `relation "nakshatras" does not exist`}, CategorySchemaMissing, "42P01", false},
		{"insufficient privilege", &pgconn.PgError{Code: "42501", Message: "permission denied"}, CategoryPermissionDenied, "42501", false},
		{"server starting", &pgconn.PgError{Code: "57P03", Message: "the database system is starting up"}, CategoryServerError, "57P03", true},
		{"too many connections", &pgconn.PgError{Code: "53300", Message: "too many clients"}, CategoryServerError, "53300", true},
		{"deadline", context.DeadlineExceeded, CategoryTimeout, "", true},
		{"dns", &net.DNSError{Err: "no such host", Name: "gone.neon.tech"}, CategoryDNS, "", false},
		{"refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, CategoryConnectionRefused, "", true},
		{"tls record", tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}, CategoryTLS, "", false},
		{"generic", errors.New("something else entirely"), CategoryUnknown, "", true},
	}
	for _, c := range cases {
		got := Classify(c.err)
		if got.Category != c.category {
			t.Fatalf("%s: category = %q, want %q", c.name, got.Category, c.category)
		}
		if got.SQLState != c.sqlstate {
			t.Fatalf("%s: sqlstate = %q, want %q", c.name, got.SQLState, c.sqlstate)
		}
		if got.Retryable != c.retry {
			t.Fatalf("%s: retryable = %t, want %t", c.name, got.Retryable, c.retry)
		}
		if got.Remediation == "" && got.Category != CategoryOK {
			t.Fatalf("%s: every diagnosis must suggest a next step", c.name)
		}
	}
}

func TestClassifyNeverEchoesClientErrorString(t *testing.T) {
	// A pgx connection error can embed the host and user from the DSN. The
	// diagnosis must not copy that string into ServerMessage.
	err := errors.New("failed to connect to `user=neondb_owner database=neondb`: dial error")
	d := Classify(err)
	if d.ServerMessage != "" {
		t.Fatalf("client error text must not be stored: %q", d.ServerMessage)
	}
}

func TestProbeReportsNotConfigured(t *testing.T) {
	report := Probe(context.Background(), "   ", time.Second)
	if report.Reachable {
		t.Fatalf("an empty DSN must not be reachable")
	}
	if report.Diag.Category != CategoryNotConfigured {
		t.Fatalf("category = %q", report.Diag.Category)
	}
}

func TestProbeReportsMalformedDSN(t *testing.T) {
	report := Probe(context.Background(), "https://not-a-postgres-dsn", time.Second)
	if report.Reachable {
		t.Fatalf("a non-postgres DSN must not be reachable")
	}
	if report.Diag.Category != CategoryMalformedDSN {
		t.Fatalf("category = %q", report.Diag.Category)
	}
}

// Probe must be bounded: an unreachable host has to fail quickly and must not
// leak the password through any path it renders.
func TestProbeIsBoundedAndSafe(t *testing.T) {
	dsn := "postgres://u:" + secretPassword + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=2"
	started := time.Now()
	report := Probe(context.Background(), dsn, 2*time.Second)
	elapsed := time.Since(started)

	if elapsed > 5*time.Second {
		t.Fatalf("probe took %s; it must be bounded", elapsed)
	}
	if report.Reachable {
		t.Fatalf("port 1 must not be reachable")
	}
	if report.Diag.Category == CategoryOK {
		t.Fatalf("a failed probe must not report ok")
	}
	if strings.Contains(report.Summary(), secretPassword) {
		t.Fatalf("summary leaked the password:\n%s", report.Summary())
	}
	if !strings.Contains(report.Summary(), "next step:") {
		t.Fatalf("summary must suggest a next step:\n%s", report.Summary())
	}
}
