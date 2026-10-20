package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newHealthAPI builds the smallest API that can answer a liveness probe: no
// database, no AI client, no conversation manager. That is the point of the
// test below rather than a shortcut around one — if liveness survived the loss
// of every dependency, it cannot be reading any of them.
func newHealthAPI(revision string) *API {
	api := &API{
		db:       nil,
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		mux:      http.NewServeMux(),
		revision: revision,
	}
	api.routes()
	return api
}

// /healthz is the documented liveness endpoint, and it has to keep two promises
// that a single check on a 200 status would not catch.
//
// It answers as the Go API. A rewrite misconfigured toward the web app answers
// this path with an HTML page and a 404, which a status-code-only probe reads
// as "down" and a human reads as a mystery; the content type is what tells the
// two apart.
//
// It identifies the build. This endpoint is the only place a running host can
// be matched to a commit without trusting a deployment dashboard, and the
// incident that made it necessary was exactly that: a hostname serving a
// revision nobody could name.
func TestHealthzReportsTheGoAPIAndItsBuild(t *testing.T) {
	api := newHealthAPI("1285d9c")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	api.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("healthz content type = %q, want JSON: this path must belong to the Go API, not the web app", got)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("healthz body is not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
	if body["service"] != "everroot-api" {
		t.Errorf("service = %q, want everroot-api", body["service"])
	}
	if body["revision"] != "1285d9c" {
		t.Errorf("revision = %q, want the injected build 1285d9c", body["revision"])
	}
}

// A build that was made without the linker flag must still report something a
// caller can compare against. An empty field is the one unacceptable answer: it
// is indistinguishable from a field that was never sent, so a probe comparing
// revisions could not fail loudly.
func TestHealthzNeverReportsAnEmptyRevision(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	for _, tc := range []struct{ name, revision string }{
		{"unset", ""},
		{"blank", "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			newHealthAPI(tc.revision).mux.ServeHTTP(w, req)

			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("healthz body is not JSON: %v", err)
			}
			if body["revision"] == "" {
				t.Fatal(`revision is empty; an unversioned build must say "unknown" rather than send nothing`)
			}
		})
	}
}

// Liveness must not reach for a dependency. A deployment whose database is
// unreachable, or whose AI provider is unconfigured, is still a deployment that
// is running, and the probe that reports otherwise makes an operator chase the
// wrong layer first.
func TestHealthzAnswersWithoutADatabaseOrAIProvider(t *testing.T) {
	api := newHealthAPI("deadbeef")
	if api.db != nil || api.aiClient != nil {
		t.Fatal("test precondition: this API must have no database and no AI client")
	}

	w := httptest.NewRecorder()
	api.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("healthz status = %d with no dependencies, want 200: liveness must not depend on them", w.Code)
	}
}
