// Package ai provides a server-side client for the Hugging Face Inference
// Providers chat-completion API. The HF_TOKEN is read exclusively from the
// server environment and is never exposed to the client or browser.
//
// The client is used for two things only: extracting structured details from
// free text in the legacy conversation flow, and explaining botanical facts
// that have already been retrieved from Everroot's own dataset. It is never
// used to decide cultural associations, and nothing it returns is stored as a
// cultural claim.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default model used when HF_MODEL_ID is not set.
//
// The identifier deliberately carries no routing suffix. Hugging Face's router
// then selects the fastest available provider and fails over to another one
// when the primary is flagged unavailable. A policy suffix such as ":cheapest"
// pins inference to a single provider and gives up that failover, which turns a
// temporarily overloaded provider into a hard failure for every visitor.
// Measured against the live router: ":cheapest" answered 2 of 6 requests while
// the un-suffixed default answered 6 of 6.
const DefaultModelID = "openai/gpt-oss-120b"

// Default base URL for the Hugging Face Inference Providers API.
const DefaultBaseURL = "https://router.huggingface.co/v1"

// ProviderTimeout bounds a single inference attempt. It is exported because the
// HTTP server must leave more room than this to write a response: /api/v1/explain
// and /api/v1/questions answer inside the request, so a server write deadline
// shorter than this kills the connection and the browser receives nothing.
const ProviderTimeout = 25 * time.Second

// defaultMaxAttempts is how many times a transient provider failure is tried
// before the error is returned. Overload and 5xx responses are usually brief.
const defaultMaxAttempts = 3

// defaultRetryDelay is the base backoff between those attempts.
const defaultRetryDelay = 500 * time.Millisecond

// maxRetryDelay caps a backoff, including any Retry-After the provider sends.
// A provider that asks for a longer wait is answered with that header on the
// final failure rather than making the visitor's request hang.
const maxRetryDelay = 2 * time.Second

// WorstCaseResponseTime is the longest one request can legitimately take: every
// attempt the client will make under a transient failure, each bounded by
// ProviderTimeout, with the capped backoff between them.
//
// The HTTP server's write deadline has to be longer than this. It once was
// measured against a single attempt instead, so a provider that timed out and
// then succeeded on a retry ran past the deadline and the connection was cut
// mid-answer — the visitor was told the service could not be reached, for a
// call that had been working the whole time. A slow answer and a broken one
// have to stay different things.
const WorstCaseResponseTime = time.Duration(defaultMaxAttempts)*ProviderTimeout +
	time.Duration(defaultMaxAttempts-1)*maxRetryDelay

// maxErrorBodyBytes bounds how much of a provider error body is inspected. Only
// a whitelisted code is read from it; the body is never logged or forwarded.
const maxErrorBodyBytes = 4 << 10

// maxResponseBytes bounds a success body so a runaway provider cannot exhaust
// memory.
const maxResponseBytes = 1 << 20

// Typed errors so callers can surface a precise reason instead of collapsing
// every failure into one "unavailable" state.
var (
	// ErrNotConfigured means no token was supplied, so no request is possible.
	ErrNotConfigured = errors.New("ai client is not configured")
	// ErrUnauthorized means the provider rejected the credentials.
	ErrUnauthorized = errors.New("inference authentication failed")
	// ErrCredits means the account cannot pay for this provider, or its
	// included credits are exhausted. Retrying will not help.
	ErrCredits = errors.New("inference credits exhausted")
	// ErrRateLimited means the provider is throttling Everroot or the model is
	// busy. This is transient and worth retrying.
	ErrRateLimited = errors.New("inference rate limited")
	// ErrProvider means the provider returned an error status.
	ErrProvider = errors.New("inference provider error")
	// ErrNoCompletion means the provider returned no usable choice.
	ErrNoCompletion = errors.New("inference returned no completion")
	// ErrTimeout means the request exceeded its deadline.
	ErrTimeout = errors.New("inference request timed out")
	// ErrBadResponse means the model replied with something unusable, such as
	// empty text. It is not the same as an empty dataset.
	ErrBadResponse = errors.New("inference returned an unusable response")
)

// Category is a safe, structured classification of an inference failure. It
// carries no credentials and no message content, so it can be logged and
// forwarded to a monitor without leaking either.
type Category string

const (
	CategoryNotConfigured Category = "not_configured"
	CategoryAuth          Category = "auth"
	CategoryCredits       Category = "credits"
	CategoryRateLimited   Category = "rate_limited"
	CategoryProvider      Category = "provider_error"
	CategoryTimeout       Category = "timeout"
	CategoryNoCompletion  Category = "no_completion"
	CategoryBadResponse   Category = "bad_response"
	CategoryUnknown       Category = "unknown"
)

// Diagnosis describes an inference failure without exposing the token, the
// prompt, or the provider's response body.
type Diagnosis struct {
	// Category is the classified failure class.
	Category Category
	// Status is the provider's HTTP status, or 0 when no response arrived.
	Status int
	// ProviderCode is the provider's own machine-readable error code (for
	// example "engine_overloaded"), reduced to a safe token. Empty when absent.
	ProviderCode string
	// Detail is a short, credential-free description for the server log. It is
	// never sent to a client.
	Detail string
	// Retryable reports whether the same request could succeed later.
	Retryable bool
	// RetryAfter is the provider's requested wait, when it sent one.
	RetryAfter time.Duration
}

// Client is a server-side Hugging Face Inference API client.
type Client struct {
	http    *http.Client
	token   string
	model   string
	baseURL string
	log     Logger

	// maxAttempts and retryDelay are configurable so tests can retry without
	// waiting on a real backoff.
	maxAttempts int
	retryDelay  time.Duration
}

// Logger is the small logging surface the client needs.
type Logger interface {
	Warn(msg string, args ...any)
}

// Message represents a single chat message.
type Message struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// ChatRequest is the request body for the chat-completion API.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatChoice is a single completion choice.
type ChatChoice struct {
	Message Message `json:"message"`
}

// ChatResponse is the response from the chat-completion API.
type ChatResponse struct {
	ID      string       `json:"id"`
	Choices []ChatChoice `json:"choices"`
}

// CompletionOptions tunes a single completion request.
type CompletionOptions struct {
	MaxTokens   int
	Temperature float64
}

// New creates a Hugging Face client. It reads HF_TOKEN from the environment.
// If HF_MODEL_ID is set it is used verbatim; otherwise DefaultModelID is used.
func New(log Logger) (*Client, error) {
	token := strings.TrimSpace(os.Getenv("HF_TOKEN"))
	if token == "" {
		return nil, errors.New("HF_TOKEN is required for the AI client")
	}
	model := strings.TrimSpace(os.Getenv("HF_MODEL_ID"))
	if model == "" {
		model = DefaultModelID
	}
	// Trim a trailing slash so a configured base URL cannot produce a doubled
	// separator in "...//chat/completions".
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("HF_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		http:        &http.Client{Timeout: ProviderTimeout},
		token:       token,
		model:       model,
		baseURL:     baseURL,
		log:         log,
		maxAttempts: defaultMaxAttempts,
		retryDelay:  defaultRetryDelay,
	}, nil
}

// Model reports the model identifier in use, for display next to generated text.
func (c *Client) Model() string { return c.model }

// ChatCompletion sends a chat-completion request and returns the assistant's
// reply. It never logs the token or message contents.
func (c *Client) ChatCompletion(ctx context.Context, messages []Message) (string, error) {
	return c.ChatCompletionWithOptions(ctx, messages, CompletionOptions{MaxTokens: 500, Temperature: 0.7})
}

// ChatCompletionWithOptions sends a chat-completion request with explicit
// generation settings.
//
// Transient failures (an overloaded model, a 5xx, a timeout) are retried a
// bounded number of times. Each retry drops any routing suffix from the model
// so the router can fail over to a different provider instead of asking the
// same busy one again. Authentication, credits and unusable responses are
// never retried: they cannot be fixed by waiting.
func (c *Client) ChatCompletionWithOptions(ctx context.Context, messages []Message, opts CompletionOptions) (string, error) {
	if c == nil {
		return "", ErrNotConfigured
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 500
	}
	attempts := c.maxAttempts
	if attempts < 1 {
		attempts = 1
	}
	base := c.retryDelay
	if base <= 0 {
		base = defaultRetryDelay
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		model := c.model
		if attempt > 0 {
			model = autoRoutedModel(c.model)
			if err := sleepContext(ctx, retryDelayFor(lastErr, base, attempt)); err != nil {
				return "", ctxError(err)
			}
		}

		text, err := c.complete(ctx, model, messages, opts)
		if err == nil {
			return text, nil
		}
		lastErr = err

		if !isRetryable(err) {
			return "", err
		}
		if ctx.Err() != nil {
			return "", ctxError(ctx.Err())
		}
		if attempt < attempts-1 {
			c.logRetry(attempt+1, model, err)
		}
	}
	return "", lastErr
}

// complete performs one provider request.
func (c *Client) complete(ctx context.Context, model string, messages []Message, opts CompletionOptions) (string, error) {
	req := ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
		Stream:      false,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		// A deadline or client timeout is reported separately from a plain
		// connection failure so the UI can tell them apart.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || os.IsTimeout(err) {
			return "", ErrTimeout
		}
		// Only the transport error itself is kept, never the full URL, so a log
		// line can never carry a credential.
		return "", fmt.Errorf("%w: %s", ErrProvider, transportDetail(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", providerFailure(resp)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	if len(chatResp.Choices) == 0 {
		return "", ErrNoCompletion
	}
	return chatResp.Choices[0].Message.Content, nil
}

// providerError carries the provider's status and a safe error code while still
// unwrapping to a sentinel so errors.Is keeps working.
type providerError struct {
	sentinel   error
	status     int
	code       string
	retryAfter time.Duration
}

func (e *providerError) Error() string {
	if e.code != "" {
		return fmt.Sprintf("%s (status %d, code %s)", e.sentinel, e.status, e.code)
	}
	return fmt.Sprintf("%s (status %d)", e.sentinel, e.status)
}

func (e *providerError) Unwrap() error { return e.sentinel }

// providerFailure classifies an error status and reads only the provider's
// machine-readable error code from a bounded prefix of the body. The human
// message is deliberately not read: it can echo the request we sent.
func providerFailure(resp *http.Response) error {
	pe := &providerError{
		status:     resp.StatusCode,
		code:       providerErrorCode(resp.Body),
		retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		pe.sentinel = ErrUnauthorized
	case resp.StatusCode == http.StatusPaymentRequired:
		pe.sentinel = ErrCredits
	case resp.StatusCode == http.StatusTooManyRequests:
		pe.sentinel = ErrRateLimited
	default:
		pe.sentinel = ErrProvider
	}
	return pe
}

// providerErrorCode extracts error.code (falling back to error.type) and
// reduces it to a safe token, so nothing a provider writes in prose can reach a
// log line.
func providerErrorCode(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var parsed struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}
	code := parsed.Error.Code
	if code == "" {
		code = parsed.Error.Type
	}
	return sanitizeCode(code)
}

// sanitizeCode keeps only characters a machine-readable code can contain, and
// caps the length.
func sanitizeCode(code string) string {
	var b strings.Builder
	for _, r := range code {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

// parseRetryAfter reads the Retry-After header as seconds or an HTTP date.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// transportDetail returns the underlying transport error without the request
// URL, which is where a client-side error string would otherwise carry it.
func transportDetail(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		return truncateDetail(uerr.Err.Error())
	}
	return truncateDetail(err.Error())
}

func truncateDetail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

// isRetryable reports whether waiting could fix the failure.
func isRetryable(err error) bool {
	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrTimeout) {
		return true
	}
	var pe *providerError
	if errors.As(err, &pe) {
		return pe.status >= 500
	}
	return false
}

// retryDelayFor prefers the provider's own Retry-After, then an exponential
// backoff with jitter. It is always capped so a slow provider cannot outlast
// the caller's deadline.
func retryDelayFor(err error, base time.Duration, attempt int) time.Duration {
	var pe *providerError
	if errors.As(err, &pe) && pe.retryAfter > 0 {
		if pe.retryAfter < base {
			return pe.retryAfter
		}
		return min(pe.retryAfter, maxRetryDelay)
	}
	d := base << (attempt - 1)
	if d > maxRetryDelay || d <= 0 {
		d = maxRetryDelay
	}
	jitter := time.Duration(rand.IntN(250)) * time.Millisecond
	if d+jitter > maxRetryDelay {
		return maxRetryDelay
	}
	return d + jitter
}

// autoRoutedModel drops a trailing routing policy or provider suffix so a retry
// can be served by any available provider (":cheapest" -> automatic selection).
func autoRoutedModel(model string) string {
	if i := strings.LastIndexByte(model, ':'); i > 0 {
		return model[:i]
	}
	return model
}

// sleepContext waits, or returns early when the context ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ctxError converts a context failure into the client's typed timeout error.
func ctxError(err error) error {
	return fmt.Errorf("%w: %v", ErrTimeout, err)
}

func (c *Client) logRetry(attempt int, model string, err error) {
	if c.log == nil {
		return
	}
	diag := Diagnose(err)
	c.log.Warn("retrying inference request",
		"attempt", attempt,
		"model", model,
		"category", string(diag.Category),
		"status", diag.Status,
		"provider_code", diag.ProviderCode,
	)
}

// Diagnose classifies an inference error into a safe, structured report. It
// never includes the token, the prompt, or the provider's response body, so it
// is safe to log.
func Diagnose(err error) Diagnosis {
	if err == nil {
		return Diagnosis{}
	}
	d := Diagnosis{Category: CategoryUnknown}

	var pe *providerError
	if errors.As(err, &pe) {
		d.Status = pe.status
		d.ProviderCode = pe.code
		d.RetryAfter = pe.retryAfter
	}

	switch {
	case errors.Is(err, ErrNotConfigured):
		d.Category = CategoryNotConfigured
	case errors.Is(err, ErrUnauthorized):
		d.Category = CategoryAuth
	case errors.Is(err, ErrCredits):
		d.Category = CategoryCredits
	case errors.Is(err, ErrRateLimited):
		d.Category = CategoryRateLimited
		d.Retryable = true
	case errors.Is(err, ErrTimeout):
		d.Category = CategoryTimeout
		d.Retryable = true
	case errors.Is(err, ErrNoCompletion):
		d.Category = CategoryNoCompletion
	case errors.Is(err, ErrBadResponse):
		d.Category = CategoryBadResponse
	case errors.Is(err, ErrProvider):
		d.Category = CategoryProvider
		// A transport failure (no status) is worth another try; a 4xx is not.
		d.Retryable = d.Status == 0 || d.Status >= 500
	}

	// Detail is only populated for errors this package built, whose text is
	// known to contain no credential. An error from anywhere else is classified
	// as unknown and described no further, so its text can never reach a log.
	if d.Category != CategoryUnknown {
		d.Detail = truncateDetail(err.Error())
	}
	return d
}
