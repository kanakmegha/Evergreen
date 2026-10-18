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
	"net/http"
	"os"
	"time"
)

// Default model used when HF_MODEL_ID is not set.
const DefaultModelID = "openai/gpt-oss-120b:cheapest"

// Default base URL for the Hugging Face Inference Providers API.
const DefaultBaseURL = "https://router.huggingface.co/v1"

// ProviderTimeout bounds a single inference request. It is exported because the
// HTTP server must leave more room than this to write a response: /api/v1/explain
// and /api/v1/questions answer inside the request, so a server write deadline
// shorter than this kills the connection and the browser receives nothing.
const ProviderTimeout = 25 * time.Second

// Typed errors so callers can surface a precise reason instead of collapsing
// every failure into one "unavailable" state.
var (
	// ErrUnauthorized means the provider rejected the credentials.
	ErrUnauthorized = errors.New("inference authentication failed")
	// ErrRateLimited means the provider is throttling Everroot.
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

// Client is a server-side Hugging Face Inference API client.
type Client struct {
	http    *http.Client
	token   string
	model   string
	baseURL string
	log     Logger
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
// If HF_MODEL_ID is set it is used; otherwise DefaultModelID is used.
func New(log Logger) (*Client, error) {
	token := os.Getenv("HF_TOKEN")
	if token == "" {
		return nil, errors.New("HF_TOKEN is required for the AI client")
	}
	model := os.Getenv("HF_MODEL_ID")
	if model == "" {
		model = DefaultModelID
	} else if model == "openai/gpt-oss-120b" {
		// When querying the Hugging Face Router without a routing suffix, the
		// router defaults to :fastest (Cerebras), which rejects requests with
		// Cloudflare Error 1010. Route via :cheapest (DeepInfra) to ensure reliable
		// inference.
		model = "openai/gpt-oss-120b:cheapest"
	}
	baseURL := os.Getenv("HF_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		http:    &http.Client{Timeout: ProviderTimeout},
		token:   token,
		model:   model,
		baseURL: baseURL,
		log:     log,
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
func (c *Client) ChatCompletionWithOptions(ctx context.Context, messages []Message, opts CompletionOptions) (string, error) {
	if c == nil {
		return "", errors.New("ai client is not configured")
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 500
	}
	req := ChatRequest{
		Model:       c.model,
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
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return "", ErrTimeout
		}
		return "", fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", ErrRateLimited
	case resp.StatusCode >= 400:
		// The provider's body is deliberately not read, echoed, or logged: it
		// can contain the request we sent.
		return "", fmt.Errorf("%w (status %d)", ErrProvider, resp.StatusCode)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	if len(chatResp.Choices) == 0 {
		return "", ErrNoCompletion
	}
	return chatResp.Choices[0].Message.Content, nil
}
