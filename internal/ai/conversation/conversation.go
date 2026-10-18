// Package conversation implements Everroot's conversational discovery flow.
// It manages short-lived anonymous sessions and uses the Hugging Face client
// to generate empathetic, context-aware responses. The actual discovery
// (Nakshatra -> tree mapping) is performed by the HTTP API layer.
package conversation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"everroot/internal/ai"
)

// State values for the conversation flow.
const (
	StateGreeting           = "greeting"
	StateIntentSelected     = "intent_selected"
	StateCollectingDetails  = "collecting_details"
	StateReadyForDiscovery  = "ready_for_discovery"
	StateDiscoveryComplete  = "discovery_complete"
	StateNoVerifiedMapping  = "no_verified_mapping"
)

// Intent values.
const (
	IntentCelebrate = "celebrate"
	IntentRemember  = "remember"
	IntentExplore   = "explore"
	IntentGrow      = "grow"
	IntentCurious   = "curious"
)

// AllIntents lists all supported intents in display order.
var AllIntents = []struct {
	ID          string
	Label       string
	Description string
}{
	{IntentCelebrate, "Celebrate", "For a new life, a new beginning, or a moment worth remembering."},
	{IntentRemember, "Remember", "For someone whose presence continues to shape your life."},
	{IntentExplore, "Explore traditions", "For discovering the trees connected to a cultural or spiritual tradition."},
	{IntentGrow, "Grow", "For a future you want to nurture, even if you cannot see it yet."},
	{IntentCurious, "Simply curious", "For exploring trees and their meanings, with no particular occasion."},
}

// Session represents a short-lived anonymous discovery session.
type Session struct {
	ID                  string   `json:"id"`
	Intent              string   `json:"intent,omitempty"`
	TraditionSlug       string   `json:"tradition_slug,omitempty"`
	RegionName          string   `json:"region_name,omitempty"`
	Nakshatra           string   `json:"nakshatra,omitempty"`
	State               string   `json:"state"`
	DiscoveryMappingID  *string  `json:"discovery_mapping_id,omitempty"`
	DiscoveryRevisionID *string  `json:"discovery_revision_id,omitempty"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// DiscoveryResult is a subset of the API's discovery response, returned to the
// conversation layer so it can render a result.
type DiscoveryResult struct {
	Status             string `json:"status"`
	Recommendations    []struct {
		MappingID          string `json:"mappingId"`
		RevisionID         string `json:"revisionId"`
		Version            int    `json:"version"`
		Tradition          string `json:"tradition"`
		Region             string `json:"region"`
		Nakshatra          string `json:"nakshatra"`
		Claim              string `json:"claim"`
		Interpretation     string `json:"interpretation"`
		Sources            []struct {
			Title               string `json:"title"`
			URL                 string `json:"url"`
			Locator             string `json:"locator"`
			Relationship        string `json:"relationship"`
			BibliographicRef    string `json:"bibliographicReference"`
			AttributionReq      string `json:"attributionRequirements"`
			ReuseStatus         string `json:"reuseStatus"`
			VerificationStatus  string `json:"verificationStatus"`
		} `json:"sources"`
		BotanicalCandidates []struct {
			ScientificName string  `json:"scientificName"`
			Certainty      string  `json:"certainty"`
			Confidence     float64 `json:"confidence"`
			ReviewStatus   string  `json:"reviewStatus"`
			Definitive     bool    `json:"definitive"`
			Evidence       []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Locator     string `json:"locator"`
				Relationship  string `json:"relationship"`
			} `json:"evidence"`
		} `json:"botanicalCandidates"`
	} `json:"recommendations"`
}

// ConversationRequest is the incoming request to the conversation endpoint.
type ConversationRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message,omitempty"`
	Intent    string `json:"intent,omitempty"`
}

// ConversationResponse is the outgoing response.
type ConversationResponse struct {
	SessionID   string             `json:"session_id"`
	State       string             `json:"state"`
	Intent      string             `json:"intent,omitempty"`
	Message     string             `json:"message"`
	Suggestions []string           `json:"suggestions,omitempty"`
	Discovery   *DiscoveryResult   `json:"discovery,omitempty"`
}

// Manager handles conversation state and response generation.
type Manager struct {
	db         *pgxpool.Pool
	ai         *ai.Client
	log        *slog.Logger
	httpClient *http.Client
}

// New creates a conversation manager.
func New(db *pgxpool.Pool, aiClient *ai.Client, log *slog.Logger) *Manager {
	return &Manager{
		db:         db,
		ai:         aiClient,
		log:        log,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Handle processes a user message and returns the assistant's response.
func (m *Manager) Handle(ctx context.Context, req ConversationRequest) (ConversationResponse, error) {
	resp := ConversationResponse{}

	// Load or create session.
	session, err := m.getOrCreateSession(ctx, req.SessionID)
	if err != nil {
		return resp, fmt.Errorf("get or create session: %w", err)
	}
	resp.SessionID = session.ID
	resp.Intent = session.Intent

	// Determine the effective intent: explicit intent parameter takes precedence.
	intent := req.Intent
	if intent == "" {
		intent = session.Intent
	}

	// State machine.
	switch session.State {
	case StateGreeting:
		if intent != "" && validIntent(intent) {
			session.Intent = intent
			session.State = StateIntentSelected
			if err := m.saveSession(ctx, session); err != nil {
				return resp, fmt.Errorf("save session: %w", err)
			}
			resp.State = StateIntentSelected
			resp.Intent = intent
			resp.Message = m.intentAcknowledgment(intent)
			resp.Suggestions = m.nextStepSuggestions(intent)
			return resp, nil
		}
		// Still at greeting: ask the opening question.
		resp.State = StateGreeting
		resp.Message = "What brings you here today?"
		resp.Suggestions = []string{"Celebrate", "Remember", "Explore traditions", "Grow", "Simply curious"}
		return resp, nil

	case StateIntentSelected:
		if intent != "" && validIntent(intent) && intent != session.Intent {
			session.Intent = intent
			if err := m.saveSession(ctx, session); err != nil {
				return resp, fmt.Errorf("save session: %w", err)
			}
			resp.State = StateIntentSelected
			resp.Intent = intent
			resp.Message = m.intentAcknowledgment(intent)
			resp.Suggestions = m.nextStepSuggestions(intent)
			return resp, nil
		}
		// User may be responding with free text; try to extract details.
		details, err := m.extractDetails(ctx, session.Intent, req.Message)
		if err != nil {
			m.log.Warn("detail extraction failed, falling back", "error", err)
		}
		if details.Tradition != "" {
			session.TraditionSlug = traditionNameToSlug(details.Tradition)
		}
		if details.Region != "" {
			session.RegionName = details.Region
		}
		if details.Nakshatra != "" {
			session.Nakshatra = details.Nakshatra
		}
		if err := m.saveSession(ctx, session); err != nil {
			return resp, fmt.Errorf("save session: %w", err)
		}

		// If we have tradition + nakshatra, we're ready for discovery.
		if session.TraditionSlug != "" && session.Nakshatra != "" {
			session.State = StateReadyForDiscovery
			if err := m.saveSession(ctx, session); err != nil {
				return resp, fmt.Errorf("save session: %w", err)
			}
			return m.performDiscovery(ctx, session)
		}

		// Otherwise ask for the missing detail.
		resp.State = StateCollectingDetails
		resp.Intent = session.Intent
		resp.Message = m.followUpQuestion(session)
		resp.Suggestions = m.detailSuggestions(session)
		return resp, nil

	case StateCollectingDetails:
		details, err := m.extractDetails(ctx, session.Intent, req.Message)
		if err == nil {
			if details.Tradition != "" {
				session.TraditionSlug = traditionNameToSlug(details.Tradition)
			}
			if details.Region != "" {
				session.RegionName = details.Region
			}
			if details.Nakshatra != "" {
				session.Nakshatra = details.Nakshatra
			}
		}
		if err := m.saveSession(ctx, session); err != nil {
			return resp, fmt.Errorf("save session: %w", err)
		}
		if session.TraditionSlug != "" && session.Nakshatra != "" {
			session.State = StateReadyForDiscovery
			if err := m.saveSession(ctx, session); err != nil {
				return resp, fmt.Errorf("save session: %w", err)
			}
			return m.performDiscovery(ctx, session)
		}
		resp.State = StateCollectingDetails
		resp.Intent = session.Intent
		resp.Message = m.followUpQuestion(session)
		resp.Suggestions = m.detailSuggestions(session)
		return resp, nil

	case StateReadyForDiscovery, StateDiscoveryComplete, StateNoVerifiedMapping:
		// User wants to start again.
		return m.restart(ctx, session)

	default:
		return m.restart(ctx, session)
	}
}

// performDiscovery calls the discovery logic and transitions to a result state.
func (m *Manager) performDiscovery(ctx context.Context, session *Session) (ConversationResponse, error) {
	resp := ConversationResponse{
		SessionID: session.ID,
		State:     session.State,
		Intent:    session.Intent,
	}

	discoveryResp, err := m.callDiscovery(ctx, session.TraditionSlug, session.RegionName, session.Nakshatra)
	if err != nil {
		m.log.Error("discovery call failed", "error", err)
		resp.State = StateNoVerifiedMapping
		resp.Message = "I wasn't able to reach the discovery service right now. Please try again shortly."
		resp.Suggestions = []string{"Try again", "Start over"}
		if err := m.saveSession(ctx, session); err != nil {
			return resp, fmt.Errorf("save session: %w", err)
		}
		return resp, nil
	}

	if discoveryResp.Status == "no_verified_mapping" || len(discoveryResp.Recommendations) == 0 {
		session.State = StateNoVerifiedMapping
		resp.State = StateNoVerifiedMapping
		resp.Message = m.noVerifiedMessage(session)
		resp.Suggestions = []string{"Explore another tradition", "Start over"}
	} else {
		session.State = StateDiscoveryComplete
		session.DiscoveryMappingID = &discoveryResp.Recommendations[0].MappingID
		session.DiscoveryRevisionID = &discoveryResp.Recommendations[0].RevisionID
		resp.State = StateDiscoveryComplete
		resp.Message = m.resultMessage(session, discoveryResp)
		resp.Discovery = discoveryResp
		resp.Suggestions = []string{"Explore another option", "Start over"}
	}

	if err := m.saveSession(ctx, session); err != nil {
		return resp, fmt.Errorf("save session: %w", err)
	}
	return resp, nil
}

// restart resets the session to greeting.
func (m *Manager) restart(ctx context.Context, session *Session) (ConversationResponse, error) {
	session.Intent = ""
	session.TraditionSlug = ""
	session.RegionName = ""
	session.Nakshatra = ""
	session.State = StateGreeting
	session.DiscoveryMappingID = nil
	session.DiscoveryRevisionID = nil
	if err := m.saveSession(ctx, session); err != nil {
		return ConversationResponse{}, fmt.Errorf("save session: %w", err)
	}
	return ConversationResponse{
		SessionID: session.ID,
		State:     StateGreeting,
		Message:   "What brings you here today?",
		Suggestions: []string{"Celebrate", "Remember", "Explore traditions", "Grow", "Simply curious"},
	}, nil
}

// validIntent checks whether the intent string is supported.
func validIntent(intent string) bool {
	for _, i := range AllIntents {
		if i.ID == intent {
			return true
		}
	}
	return false
}

// traditionNameToSlug maps a user-facing tradition name to the database slug.
func traditionNameToSlug(name string) string {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	switch lower {
	case "hindu tradition", "hindu", "hinduism":
		return "nakshatra-tree-association"
	case "tamil tradition", "tamil", "tamil nadu":
		return "nakshatra-tree-association"
	case "north indian tradition", "north indian", "north india":
		return "nakshatra-tree-association"
	default:
		return name // pass through; may not match any slug
	}
}

// intentAcknowledgment returns an empathetic acknowledgment for the chosen intent.
func (m *Manager) intentAcknowledgment(intent string) string {
	switch intent {
	case IntentCelebrate:
		return "Congratulations on this new beginning. We can explore a tree connected to your family's traditions, or find a meaning that represents the life ahead."
	case IntentRemember:
		return "I'm sorry for your loss. We can explore a tree associated with their traditions, or choose a living tribute that reflects what they meant to you."
	case IntentExplore:
		return "That's a meaningful way to connect. Let's explore the trees documented in your chosen tradition."
	case IntentGrow:
		return "That's a beautiful intention. We can explore a tree that represents growth, continuity, or the legacy you hope to leave."
	case IntentCurious:
		return "Curiosity is a lovely place to start. Let's explore trees and their meanings together."
	default:
		return "Thank you for sharing. Let's explore together."
	}
}

// followUpQuestion asks for the missing detail needed for discovery.
func (m *Manager) followUpQuestion(session *Session) string {
	if session.TraditionSlug == "" {
		return "Is there a particular cultural tradition you'd like to explore?"
	}
	if session.Nakshatra == "" {
		return "Do you already know your Nakshatra or another horoscope category you'd like to explore?"
	}
	return "Is there a particular region or language tradition you'd like us to consider?"
}

// nextStepSuggestions returns suggestions after intent selection.
func (m *Manager) nextStepSuggestions(intent string) []string {
	return []string{"Hindu tradition", "Tamil tradition", "North Indian tradition", "I'm not sure"}
}

// detailSuggestions returns suggestions for the detail-collection step.
func (m *Manager) detailSuggestions(session *Session) []string {
	if session.TraditionSlug == "" {
		return []string{"Hindu tradition", "Tamil tradition", "North Indian tradition", "I'm not sure"}
	}
	return []string{"Ashwini", "Bharani", "Krittika", "Rohini", "Mrigashira", "I don't know my Nakshatra"}
}

// noVerifiedMessage returns an honest message when no verified mapping exists.
func (m *Manager) noVerifiedMessage(session *Session) string {
	traditionLabel := session.TraditionSlug
	if traditionLabel == "" {
		traditionLabel = "that tradition"
	}
	return fmt.Sprintf("Everroot doesn't yet have a verified association for %s in %s. We keep the space open rather than fill it with a guess. Would you like to explore a different tradition or Nakshatra?", session.Nakshatra, traditionLabel)
}

// resultMessage returns an emotionally meaningful headline for the result.
func (m *Manager) resultMessage(session *Session, result *DiscoveryResult) string {
	if len(result.Recommendations) == 0 {
		return "No verified association was found for your selection."
	}
	name := result.Recommendations[0].Nakshatra
	switch session.Intent {
	case IntentCelebrate:
		return fmt.Sprintf("A tree for a new beginning — %s", name)
	case IntentRemember:
		return fmt.Sprintf("A living tribute — %s", name)
	case IntentExplore:
		return fmt.Sprintf("A tree from %s tradition — %s", result.Recommendations[0].Tradition, name)
	case IntentGrow:
		return fmt.Sprintf("A tree for your future — %s", name)
	case IntentCurious:
		return fmt.Sprintf("A tree worth discovering — %s", name)
	default:
		return fmt.Sprintf("A tree for you — %s", name)
	}
}

// extractedDetails holds structured details extracted from free text.
type extractedDetails struct {
	Tradition string
	Region    string
	Nakshatra string
}

func (m *Manager) extractDetails(ctx context.Context, intent, userMessage string) (extractedDetails, error) {
	var details extractedDetails
	if m.ai == nil {
		return details, errors.New("ai client not available")
	}
	systemPrompt := `You extract structured details from a user's message in the context of a tree-discovery conversation.
Return ONLY a JSON object with these fields: tradition, region, nakshatra.
- tradition: the cultural tradition named (e.g. "Hindu tradition", "Tamil tradition", "North Indian tradition"), or empty string.
- region: a region or locale named (e.g. "Tamil Nadu", "North India"), or empty string.
- nakshatra: a Nakshatra name if mentioned (e.g. "Ashwini", "Rohini"), or empty string.
If a field is not mentioned, set it to empty string. Do not invent values.`
	messages := []ai.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Intent: %s\nUser message: %s", intent, userMessage)},
	}
	reply, err := m.ai.ChatCompletion(ctx, messages)
	if err != nil {
		return details, err
	}
	reply = stripCodeFences(reply)
	if err := json.Unmarshal([]byte(reply), &details); err != nil {
		return details, fmt.Errorf("parse extraction: %w (reply: %s)", err, reply)
	}
	return details, nil
}

// stripCodeFences removes markdown code fences from a string.
func stripCodeFences(s string) string {
	s = trimPrefix(s, "```json")
	s = trimPrefix(s, "```")
	s = trimSuffix(s, "```")
	return s
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// getOrCreateSession loads an existing session or creates a new one.
func (m *Manager) getOrCreateSession(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID != "" {
		var s Session
		var intent, tradition, region, nakshatra sql.NullString
		var mappingID, revisionID sql.NullString
		err := m.db.QueryRow(ctx, `
			SELECT id, intent, tradition_slug, region_name, nakshatra, state,
			       discovery_mapping_id, discovery_revision_id, expires_at
			FROM discovery_sessions WHERE id=$1 AND expires_at > now()
		`, sessionID).Scan(
			&s.ID, &intent, &tradition, &region, &nakshatra,
			&s.State, &mappingID, &revisionID, &s.ExpiresAt,
		)
		if err == nil {
			s.Intent = intent.String
			s.TraditionSlug = tradition.String
			s.RegionName = region.String
			s.Nakshatra = nakshatra.String
			if mappingID.Valid {
				s.DiscoveryMappingID = &mappingID.String
			}
			if revisionID.Valid {
				s.DiscoveryRevisionID = &revisionID.String
			}
			return &s, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	var id string
	err := m.db.QueryRow(ctx, `
		INSERT INTO discovery_sessions (state) VALUES ('greeting')
		RETURNING id
	`).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &Session{ID: id, State: StateGreeting}, nil
}

// saveSession persists session state.
func (m *Manager) saveSession(ctx context.Context, s *Session) error {
	var mappingID, revisionID any
	if s.DiscoveryMappingID != nil {
		mappingID = *s.DiscoveryMappingID
	}
	if s.DiscoveryRevisionID != nil {
		revisionID = *s.DiscoveryRevisionID
	}
	_, err := m.db.Exec(ctx, `
		UPDATE discovery_sessions
		SET intent=$2, tradition_slug=$3, region_name=$4, nakshatra=$5,
		    state=$6, discovery_mapping_id=$7, discovery_revision_id=$8,
		    expires_at = now() + interval '24 hours'
		WHERE id=$1
	`, s.ID, s.Intent, s.TraditionSlug, s.RegionName, s.Nakshatra, s.State, mappingID, revisionID)
	return err
}

// GetSession returns a session by ID if it has not expired.
func (m *Manager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	var s Session
	var intent, tradition, region, nakshatra sql.NullString
	var mappingID, revisionID sql.NullString
	err := m.db.QueryRow(ctx, `
		SELECT id, intent, tradition_slug, region_name, nakshatra, state,
		       discovery_mapping_id, discovery_revision_id, expires_at
		FROM discovery_sessions WHERE id=$1 AND expires_at > now()
	`, sessionID).Scan(
		&s.ID, &intent, &tradition, &region, &nakshatra,
		&s.State, &mappingID, &revisionID, &s.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	s.Intent = intent.String
	s.TraditionSlug = tradition.String
	s.RegionName = region.String
	s.Nakshatra = nakshatra.String
	if mappingID.Valid {
		s.DiscoveryMappingID = &mappingID.String
	}
	if revisionID.Valid {
		s.DiscoveryRevisionID = &revisionID.String
	}
	return &s, nil
}

// callDiscovery calls the local discovery API endpoint.
func (m *Manager) callDiscovery(ctx context.Context, tradition, region, nakshatra string) (*DiscoveryResult, error) {
	body, err := json.Marshal(map[string]string{
		"tradition": tradition,
		"region":    region,
		"nakshatra": nakshatra,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:8080/api/v1/discoveries", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery returned status %d", resp.StatusCode)
	}
	var dr DiscoveryResult
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return nil, err
	}
	return &dr, nil
}
