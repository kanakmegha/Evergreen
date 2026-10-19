package ai

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests drive the client against a mock provider. They never call Hugging
// Face, so they cost nothing and do not need HF_TOKEN.

// recorded is one request the mock provider received.
type recorded struct {
	path  string
	auth  string
	model string
}

// mockProvider is a provider stand-in that replies with a scripted sequence.
type mockProvider struct {
	mu       sync.Mutex
	seen     []recorded
	statuses []int
	bodies   []string
	delay    time.Duration
	server   *httptest.Server
}

// newMockProvider starts a server that answers the nth call with statuses[n]
// (the last entry repeats) and the matching body.
func newMockProvider(t *testing.T, statuses []int, bodies []string) *mockProvider {
	t.Helper()
	m := &mockProvider{statuses: statuses, bodies: bodies}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		m.mu.Lock()
		n := len(m.seen)
		m.seen = append(m.seen, recorded{path: r.URL.Path, auth: r.Header.Get("Authorization"), model: req.Model})
		status := m.statuses[min(n, len(m.statuses)-1)]
		body := m.bodies[min(n, len(m.bodies)-1)]
		delay := m.delay
		m.mu.Unlock()

		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(m.server.Close)
	return m
}

func (m *mockProvider) requests() []recorded {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]recorded(nil), m.seen...)
}

// newTestClient builds a client pointed at a mock provider with instant retries.
func newTestClient(t *testing.T, provider *mockProvider, model string) *Client {
	t.Helper()
	t.Setenv("HF_TOKEN", "hf_test_token_not_real")
	t.Setenv("HF_MODEL_ID", model)
	t.Setenv("HF_BASE_URL", provider.server.URL+"/v1")
	c, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.retryDelay = time.Millisecond
	return c
}

const okBody = `{"id":"cmpl-1","choices":[{"message":{"role":"assistant","content":"Ficus religiosa is in the family Moraceae."}}]}`

func TestChatCompletionSendsBearerTokenAndModel(t *testing.T) {
	m := newMockProvider(t, []int{200}, []string{okBody})
	c := newTestClient(t, m, "openai/gpt-oss-120b:cheapest")

	got, err := c.ChatCompletion(t.Context(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if !strings.Contains(got, "Moraceae") {
		t.Fatalf("unexpected answer: %q", got)
	}

	reqs := m.requests()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly 1 provider request, got %d", len(reqs))
	}
	if reqs[0].auth != "Bearer hf_test_token_not_real" {
		t.Fatalf("expected a Bearer token on the request, got %q", reqs[0].auth)
	}
	if reqs[0].path != "/v1/chat/completions" {
		t.Fatalf("unexpected path: %q", reqs[0].path)
	}
	if reqs[0].model != "openai/gpt-oss-120b:cheapest" {
		t.Fatalf("expected the configured model on the first attempt, got %q", reqs[0].model)
	}
}

func TestChatCompletionAuthFailureIsNotRetried(t *testing.T) {
	m := newMockProvider(t, []int{401}, []string{`{"error":{"message":"Invalid credentials"}}`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if n := len(m.requests()); n != 1 {
		t.Fatalf("auth failures must not be retried; got %d requests", n)
	}
	d := Diagnose(err)
	if d.Category != CategoryAuth || d.Status != 401 || d.Retryable {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

func TestChatCompletionForbiddenIsAuth(t *testing.T) {
	m := newMockProvider(t, []int{403}, []string{`{"error":{"code":"forbidden"}}`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for 403, got %v", err)
	}
	if d := Diagnose(err); d.Category != CategoryAuth || d.Status != 403 {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

func TestChatCompletionCreditsFailureIsNotRetried(t *testing.T) {
	m := newMockProvider(t, []int{402}, []string{`{"error":"Pay-as-you go is not enabled for provider together yet."}`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrCredits) {
		t.Fatalf("expected ErrCredits for 402, got %v", err)
	}
	if n := len(m.requests()); n != 1 {
		t.Fatalf("a billing failure must not be retried; got %d requests", n)
	}
	d := Diagnose(err)
	if d.Category != CategoryCredits || d.Status != 402 || d.Retryable {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

// This is the reported production failure: the pinned cheapest provider replies
// 429 "engine_overloaded". One retry, routed without the provider pin, succeeds.
func TestChatCompletionRetriesOverloadedProviderAndFailsOver(t *testing.T) {
	overloaded := `{"error":{"message":"Model busy, retry later","type":"invalid_request_error","code":"engine_overloaded"}}`
	m := newMockProvider(t, []int{429, 200}, []string{overloaded, okBody})
	c := newTestClient(t, m, "openai/gpt-oss-120b:cheapest")

	got, err := c.ChatCompletion(t.Context(), nil)
	if err != nil {
		t.Fatalf("expected the retry to succeed, got %v", err)
	}
	if !strings.Contains(got, "Moraceae") {
		t.Fatalf("unexpected answer: %q", got)
	}

	reqs := m.requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 provider requests, got %d", len(reqs))
	}
	if reqs[0].model != "openai/gpt-oss-120b:cheapest" {
		t.Fatalf("first attempt should honour the configured pin, got %q", reqs[0].model)
	}
	// The retry drops the pin so the router can pick any healthy provider,
	// instead of asking the same overloaded one again.
	if reqs[1].model != "openai/gpt-oss-120b" {
		t.Fatalf("retry should fail over to automatic routing, got %q", reqs[1].model)
	}
}

func TestChatCompletionGivesUpAfterBoundedRetries(t *testing.T) {
	overloaded := `{"error":{"message":"Model busy, retry later","code":"engine_overloaded"}}`
	m := newMockProvider(t, []int{429}, []string{overloaded})
	c := newTestClient(t, m, "openai/gpt-oss-120b:cheapest")

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if n := len(m.requests()); n != defaultMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", defaultMaxAttempts, n)
	}
	d := Diagnose(err)
	if d.Category != CategoryRateLimited {
		t.Fatalf("expected rate_limited, got %+v", d)
	}
	if d.Status != 429 || d.ProviderCode != "engine_overloaded" || !d.Retryable {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

func TestChatCompletionRetriesServerError(t *testing.T) {
	m := newMockProvider(t, []int{503, 200}, []string{`{"error":"upstream unavailable"}`, okBody})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	if _, err := c.ChatCompletion(t.Context(), nil); err != nil {
		t.Fatalf("expected recovery after a 5xx, got %v", err)
	}
	if n := len(m.requests()); n != 2 {
		t.Fatalf("expected 2 attempts, got %d", n)
	}
}

func TestChatCompletionServerErrorDiagnosisIsRetryable(t *testing.T) {
	m := newMockProvider(t, []int{502}, []string{`{"error":"bad gateway"}`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("expected ErrProvider, got %v", err)
	}
	d := Diagnose(err)
	if d.Category != CategoryProvider || d.Status != 502 || !d.Retryable {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

func TestChatCompletionTimeoutIsClassified(t *testing.T) {
	m := newMockProvider(t, []int{200}, []string{okBody})
	m.delay = 300 * time.Millisecond
	c := newTestClient(t, m, "openai/gpt-oss-120b")
	c.http = &http.Client{Timeout: 40 * time.Millisecond}
	c.maxAttempts = 1

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	d := Diagnose(err)
	if d.Category != CategoryTimeout || !d.Retryable {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

func TestChatCompletionMalformedResponse(t *testing.T) {
	m := newMockProvider(t, []int{200}, []string{`this is not json`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("expected ErrBadResponse, got %v", err)
	}
	if d := Diagnose(err); d.Category != CategoryBadResponse || d.Retryable {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

func TestChatCompletionNoChoices(t *testing.T) {
	m := newMockProvider(t, []int{200}, []string{`{"id":"x","choices":[]}`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.ChatCompletion(t.Context(), nil)
	if !errors.Is(err, ErrNoCompletion) {
		t.Fatalf("expected ErrNoCompletion, got %v", err)
	}
	if d := Diagnose(err); d.Category != CategoryNoCompletion {
		t.Fatalf("unexpected diagnosis: %+v", d)
	}
}

// A reasoning provider can return an empty visible answer. That is an unusable
// response, not a cultural gap and not a database problem.
func TestAnswerQuestionEmptyContentIsBadResponse(t *testing.T) {
	m := newMockProvider(t, []int{200}, []string{`{"id":"x","choices":[{"message":{"role":"assistant","content":""}}]}`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.AnswerQuestion(t.Context(), QuestionInput{Question: "What family does Sal belong to?"})
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("expected ErrBadResponse, got %v", err)
	}
}

// A diagnosis is what reaches the server log, so it must never carry the token,
// even when the provider echoes it back in a human-readable message.
func TestDiagnosisNeverCarriesTokenOrProviderProse(t *testing.T) {
	const token = "hf_SUPERSECRET123"
	t.Setenv("HF_TOKEN", token)
	body := `{"error":{"message":"token ` + token + ` is invalid","code":"engine_overloaded"}}`
	m := newMockProvider(t, []int{429}, []string{body})
	t.Setenv("HF_MODEL_ID", "openai/gpt-oss-120b")
	t.Setenv("HF_BASE_URL", m.server.URL+"/v1")
	c, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.retryDelay = time.Millisecond

	_, reqErr := c.ChatCompletion(t.Context(), nil)
	if reqErr == nil {
		t.Fatal("expected an error")
	}
	d := Diagnose(reqErr)
	for name, s := range map[string]string{
		"error":         reqErr.Error(),
		"detail":        d.Detail,
		"provider_code": d.ProviderCode,
	} {
		if strings.Contains(s, token) {
			t.Fatalf("%s leaked the token: %q", name, s)
		}
	}
	// The machine-readable code is still surfaced, because it is safe.
	if d.ProviderCode != "engine_overloaded" {
		t.Fatalf("expected the provider code to survive, got %q", d.ProviderCode)
	}
}

func TestProviderErrorCodeIsSanitised(t *testing.T) {
	m := newMockProvider(t, []int{429},
		[]string{`{"error":{"code":"engine_overloaded<script>alert(1)</script> and more text"}}`})
	c := newTestClient(t, m, "openai/gpt-oss-120b")

	_, err := c.ChatCompletion(t.Context(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	code := Diagnose(err).ProviderCode
	for _, bad := range []string{"<", ">", "/", " ", "(", ")"} {
		if strings.Contains(code, bad) {
			t.Fatalf("code %q still contains %q", code, bad)
		}
	}
	if len(code) > 64 {
		t.Fatalf("code was not capped: %d", len(code))
	}
}

func TestProviderErrorCodeFallsBackAndIgnoresStrings(t *testing.T) {
	if got := providerErrorCode(strings.NewReader(`{"error":{"type":"invalid_request_error"}}`)); got != "invalid_request_error" {
		t.Fatalf("expected the type fallback, got %q", got)
	}
	// Some providers return {"error":"prose"}; the schema does not match, so no
	// code is claimed rather than echoing the prose.
	if got := providerErrorCode(strings.NewReader(`{"error":"some human readable mistake"}`)); got != "" {
		t.Fatalf("expected no code for a prose error, got %q", got)
	}
}

func TestDefaultModelHasNoRoutingPin(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_test_token_not_real")
	t.Setenv("HF_MODEL_ID", "")
	c, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Model() != DefaultModelID {
		t.Fatalf("expected default model %q, got %q", DefaultModelID, c.Model())
	}
	if strings.Contains(c.Model(), ":") {
		t.Fatalf("the default model must not pin a provider: %q", c.Model())
	}
}

func TestConfiguredModelIsHonouredVerbatim(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_test_token_not_real")
	t.Setenv("HF_MODEL_ID", "openai/gpt-oss-120b:cheapest")
	c, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// An operator's explicit choice is preserved; only a retry drops the pin.
	if c.Model() != "openai/gpt-oss-120b:cheapest" {
		t.Fatalf("unexpected model: %q", c.Model())
	}
}

func TestAutoRoutedModelStripsSuffix(t *testing.T) {
	for in, want := range map[string]string{
		"openai/gpt-oss-120b:cheapest": "openai/gpt-oss-120b",
		"openai/gpt-oss-120b:fastest":  "openai/gpt-oss-120b",
		"openai/gpt-oss-120b:groq":     "openai/gpt-oss-120b",
		"openai/gpt-oss-120b":          "openai/gpt-oss-120b",
	} {
		if got := autoRoutedModel(in); got != want {
			t.Errorf("autoRoutedModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBaseURLDefaultsAndTrimsTrailingSlash(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_test_token_not_real")
	t.Setenv("HF_MODEL_ID", "openai/gpt-oss-120b")
	t.Setenv("HF_BASE_URL", "")
	c, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.baseURL != DefaultBaseURL {
		t.Fatalf("expected default base URL, got %q", c.baseURL)
	}

	m := newMockProvider(t, []int{200}, []string{okBody})
	t.Setenv("HF_BASE_URL", m.server.URL+"/v1/")
	c2, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c2.ChatCompletion(t.Context(), nil); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if got := m.requests()[0].path; got != "/v1/chat/completions" {
		t.Fatalf("a trailing slash produced a bad path: %q", got)
	}
}

func TestRetryDelayHonoursRetryAfterButStaysBounded(t *testing.T) {
	short := &providerError{sentinel: ErrRateLimited, status: 429, retryAfter: 100 * time.Millisecond}
	if got := retryDelayFor(short, time.Second, 1); got != 100*time.Millisecond {
		t.Fatalf("expected the provider's short Retry-After, got %v", got)
	}
	long := &providerError{sentinel: ErrRateLimited, status: 429, retryAfter: time.Hour}
	if got := retryDelayFor(long, time.Second, 1); got != maxRetryDelay {
		t.Fatalf("a long Retry-After must be capped, got %v", got)
	}
	// Without a Retry-After the backoff grows but never exceeds the cap.
	if got := retryDelayFor(ErrRateLimited, time.Second, 1); got < time.Second || got > maxRetryDelay {
		t.Fatalf("unexpected backoff: %v", got)
	}
	if got := retryDelayFor(ErrRateLimited, time.Second, 4); got > maxRetryDelay {
		t.Fatalf("backoff exceeded the cap: %v", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("3"); got != 3*time.Second {
		t.Fatalf("expected 3s, got %v", got)
	}
	if got := parseRetryAfter("0"); got != 0 {
		t.Fatalf("expected 0 for a non-positive value, got %v", got)
	}
	if got := parseRetryAfter("not-a-value"); got != 0 {
		t.Fatalf("expected 0 for garbage, got %v", got)
	}
	if got := parseRetryAfter(time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)); got <= 0 {
		t.Fatalf("expected a positive duration for an HTTP date, got %v", got)
	}
}

// An error from outside this package is classified but never described, so
// nothing an unrelated code path writes into an error string can reach a log.
func TestDiagnosisDoesNotQuoteForeignErrors(t *testing.T) {
	d := Diagnose(errors.New("hf_SUPERSECRET123 leaked"))
	if d.Category != CategoryUnknown {
		t.Fatalf("expected unknown, got %+v", d)
	}
	if d.Detail != "" {
		t.Fatalf("a foreign error must not be described, got %q", d.Detail)
	}
}
