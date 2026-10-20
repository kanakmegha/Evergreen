// Package conversation implements Everroot's conversational discovery flow.
//
// It is AI-first: the suggestions a visitor receives come from the same model
// and the same validated prompt that back POST /api/v1/recommendations, reached
// through a small injected interface rather than by calling the HTTP API back
// over the loopback interface. Nothing here needs the reviewed registry, the
// botanical catalogue or a migration to produce an answer.
//
// The reviewed cultural gate is still consulted, through an optional injected
// interface, because a visitor is owed the truth about which half of what they
// are reading has been reviewed. That verdict annotates the reply; it never
// decides whether one is given.
package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"everroot/internal/ai"
	"github.com/jackc/pgx/v5/pgxpool"
)

// State values for the conversation flow.
const (
	StateGreeting          = "greeting"
	StateIntentSelected    = "intent_selected"
	StateCollectingDetails = "collecting_details"
	StateReadyForDiscovery = "ready_for_discovery"
	StateDiscoveryComplete = "discovery_complete"
	// StateNoVerifiedMapping is retained so a client comparing state strings
	// does not break. The AI-first flow no longer produces it: an absent
	// reviewed mapping annotates a successful reply instead of replacing it.
	StateNoVerifiedMapping = "no_verified_mapping"
	// StateNoSuggestions is a successful call in which the model declined to
	// suggest anything. It is a fact about the model's answer, not a failure,
	// and it is kept apart from StateRecommendationUnavailable so the visitor
	// is never told the service broke when it merely had nothing to offer.
	StateNoSuggestions = "no_suggestions"
	// StateRecommendationUnavailable is an honest failure: the model was not
	// configured, or the provider did not answer. It is never dressed up as an
	// empty success.
	StateRecommendationUnavailable = "recommendation_unavailable"
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
	ID                  string    `json:"id"`
	Intent              string    `json:"intent,omitempty"`
	TraditionSlug       string    `json:"tradition_slug,omitempty"`
	RegionName          string    `json:"region_name,omitempty"`
	Nakshatra           string    `json:"nakshatra,omitempty"`
	State               string    `json:"state"`
	DiscoveryMappingID  *string   `json:"discovery_mapping_id,omitempty"`
	DiscoveryRevisionID *string   `json:"discovery_revision_id,omitempty"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// RecommendationResult carries what the model produced, plus the provenance
// the interface prints beside it. The suggestion shape is deliberately the same
// one POST /api/v1/recommendations returns, so a result rendered from a
// conversation and one rendered from the form are the same object, and the
// results page needs no second format.
type RecommendationResult struct {
	// Status mirrors the recommendation endpoint: "ok" when suggestions were
	// made, "no_suggestions" when the model found nothing it would stand
	// behind. Both are successful calls; a failure never reaches this struct.
	Status        string          `json:"status"`
	Suggestions   []ai.Suggestion `json:"suggestions"`
	Evidence      EvidenceStatus  `json:"evidence"`
	PromptVersion string          `json:"promptVersion"`
	Model         string          `json:"model"`
	Disclaimer    string          `json:"disclaimer"`
	// FollowUpQuestion is present only when one missing fact would materially
	// change the suggestions.
	FollowUpQuestion string `json:"followUpQuestion,omitempty"`
}

// EvidenceStatus reports, per dimension, whether Everroot holds a reviewed
// record. It is what keeps three different kinds of statement apart on screen:
// what the model wrote, what a reviewer approved, and what an authority
// verified. Each is labelled from its own source, so a machine-written
// suggestion can never be read as a reviewed cultural fact.
type EvidenceStatus struct {
	CulturalAssociation string `json:"culturalAssociation"`
	BotanicalIdentity   string `json:"botanicalIdentity"`
	Habitat             string `json:"habitat"`
	EcologicalRole      string `json:"ecologicalRole"`
	DocumentedStories   string `json:"documentedStories"`
	PersonalMeaning     string `json:"personalMeaning"`
}

// Evidence vocabulary. It is the same set the HTTP API reports, so a consumer
// of one endpoint understands the other without a translation table.
const (
	AssocReviewedMatch   = "reviewed_match"
	AssocNoReviewedMatch = "no_reviewed_match"
	AssocNotQueryable    = "not_queryable"
)

// ConversationRequest is the incoming request to the conversation endpoint.
type ConversationRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message,omitempty"`
	Intent    string `json:"intent,omitempty"`
}

// ConversationResponse is the outgoing response.
type ConversationResponse struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	Intent    string `json:"intent,omitempty"`
	Message   string `json:"message"`
	// Suggestions are the short clickable replies, unrelated to the trees.
	Suggestions []string `json:"suggestions,omitempty"`
	// Recommendations is set once the model has answered. It is the only place
	// tree suggestions appear. DiscoveryMappingID and DiscoveryRevisionID stay
	// empty on the session, because a model suggestion has no reviewed mapping
	// ID and inventing one would be exactly the claim this flow must not make.
	Recommendations *RecommendationResult `json:"recommendations,omitempty"`
}

// Recommender is the slice of the suggestion engine the conversation needs,
// and the reason this package does not have to know how a prompt is built, how
// a provider is called, or how a reply is validated. *ai.Client satisfies it,
// so the conversation and POST /api/v1/recommendations run the same code and
// cannot drift apart.
type Recommender interface {
	SuggestTrees(ctx context.Context, in ai.RecommendInput) (ai.SuggestResult, error)
}

// CulturalGate reports the reviewed publication gate's verdict on a selection.
//
// It annotates a reply; it never decides whether one is given. Nil means the
// gate is unavailable, in which case the verdict is reported as unqueryable
// rather than guessed at.
type CulturalGate interface {
	AssociationVerdict(ctx context.Context, tradition, region, nakshatra string) string
}

// detailExtractor turns a visitor's free text into the structured details this
// flow asks for. *ai.Client satisfies it; a nil extractor simply means nothing
// is extracted, and the visitor's words still reach the prompt as prose.
type detailExtractor interface {
	ChatCompletion(ctx context.Context, messages []ai.Message) (string, error)
}

// Manager handles conversation state and response generation.
type Manager struct {
	db          *pgxpool.Pool
	extractor   detailExtractor
	recommender Recommender
	gate        CulturalGate
	log         *slog.Logger
}

// New creates a conversation manager.
//
// The collaborators are passed explicitly rather than derived, so a test can
// supply a recommender without a provider and a gate without a database. A nil
// aiClient yields a nil extractor, not a typed-nil interface: storing the
// concrete type would make the nil checks below impossible, which is how an
// unconfigured deployment ends up reporting a provider failure as an absence
// of trees.
func New(db *pgxpool.Pool, aiClient *ai.Client, recommender Recommender, gate CulturalGate, log *slog.Logger) *Manager {
	var extractor detailExtractor
	if aiClient != nil {
		extractor = aiClient
	}
	return &Manager{
		db:          db,
		extractor:   extractor,
		recommender: recommender,
		gate:        gate,
		log:         log,
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
		// The visitor is answering the question we just asked, so read their
		// words for anything they chose to share. An extractor that fails is a
		// logging detail, not a reason to stop: their sentence still reaches
		// the prompt as prose.
		details, err := m.extractDetails(ctx, session.Intent, req.Message)
		if err != nil {
			m.log.Warn("detail extraction failed, carrying on with the visitor's own words", "error", err)
		}
		applyDetails(session, details)
		if err := m.saveSession(ctx, session); err != nil {
			return resp, fmt.Errorf("save session: %w", err)
		}

		// One round of detail gathering, then the suggestions. A visitor who
		// says they do not know, or who has already given both a tradition and
		// a Nakshatra, goes straight through. Nothing gathered here is a
		// prerequisite: their own words alone are enough to ask the model for
		// something, so this question is an offer of context, not a toll gate.
		if m.enoughContext(session, req.Message) {
			return m.generateRecommendations(ctx, session, req.Message)
		}

		// Otherwise ask once for the missing detail.
		resp.State = StateCollectingDetails
		resp.Intent = session.Intent
		resp.Message = m.followUpQuestion(session)
		resp.Suggestions = m.detailSuggestions(session)
		return resp, nil

	case StateCollectingDetails:
		details, err := m.extractDetails(ctx, session.Intent, req.Message)
		if err != nil {
			m.log.Warn("detail extraction failed; carrying on with the visitor's own words", "error", err)
		}
		applyDetails(session, details)
		if err := m.saveSession(ctx, session); err != nil {
			return resp, fmt.Errorf("save session: %w", err)
		}
		// The visitor has been asked once already. Asking a second time for a
		// Nakshatra they may never have heard of is how a conversation stalls,
		// so this turn always produces suggestions — built from whatever was
		// gathered, which may be nothing beyond their own words. That is
		// enough; the prompt says so itself.
		return m.generateRecommendations(ctx, session, req.Message)

	case StateReadyForDiscovery, StateDiscoveryComplete, StateNoVerifiedMapping,
		StateNoSuggestions, StateRecommendationUnavailable:
		// User wants to start again.
		return m.restart(ctx, session)

	default:
		return m.restart(ctx, session)
	}
}

// intentionFor maps a conversation intent onto the suggestion engine's
// intention vocabulary. The two grew up apart: the conversation offers human
// occasions, the prompt takes the visitor's own account of what the tree is
// for. Only the identifier travels across, and the label beside it is the
// recommendation interface's own wording, so a suggestion reached through a
// conversation and one reached through the form describe the same intention in
// the same words.
func intentionFor(intent string) (id, label string) {
	switch intent {
	case IntentCelebrate:
		return "personal", "something about their own life"
	case IntentRemember:
		return "remember", "someone they want to remember"
	case IntentExplore:
		return "culture", "their own culture, traditions or beliefs"
	case IntentGrow:
		return "future", "something meant to last for someone else"
	case IntentCurious:
		return "explore", "open curiosity, nothing personal shared"
	}
	return "", ""
}

// enoughContext reports whether this turn should produce suggestions rather
// than another question. A visitor who says they do not know, or who has
// already offered both a tradition and a Nakshatra, has said everything the
// prompt needs. Nothing here can refuse anyone: the worst outcome of reading
// it wrong is one question too few or too many, never a turned-away visitor.
func (m *Manager) enoughContext(session *Session, message string) bool {
	if wantsSuggestionsNow(message) {
		return true
	}
	return session.TraditionSlug != "" && session.Nakshatra != ""
}

// wantsSuggestionsNow recognises what a visitor says when they would rather be
// shown something than be interviewed about it. It is deliberately loose and
// case-insensitive: a false positive costs a single question, while a false
// negative costs the conversation its momentum.
func wantsSuggestionsNow(message string) bool {
	lower := strings.ToLower(message)
	for _, phrase := range []string{
		"i don't know", "dont know", "not sure", "no idea", "no nakshatra",
		"i don't have", "skip", "just suggest", "just show", "surprise me",
		"you choose", "up to you", "whatever you", "don't remember",
		"dont remember", "anything you", "no preference",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// applyDetails folds whatever the extractor found into the session. A field it
// could not fill is left as it was, so a turn that mentions a region does not
// erase the tradition offered on an earlier one.
func applyDetails(session *Session, details extractedDetails) {
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

// answerFor builds the prompt's Answer: the visitor's own words, followed by
// whatever context earlier turns produced, bounded to what the prompt will
// actually send. It is written from their side of the exchange — the model is
// told what they said, never a conclusion Everroot drew about them.
func answerFor(session *Session, message string) string {
	var parts []string
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		parts = append(parts, trimmed)
	}
	var gathered []string
	if session.TraditionSlug != "" {
		gathered = append(gathered, "tradition: "+session.TraditionSlug)
	}
	if session.RegionName != "" {
		gathered = append(gathered, "region: "+session.RegionName)
	}
	if session.Nakshatra != "" {
		gathered = append(gathered, "Nakshatra: "+session.Nakshatra)
	}
	if len(gathered) > 0 {
		parts = append(parts, "(from earlier: "+strings.Join(gathered, "; ")+")")
	}
	return truncateAt(strings.Join(parts, " — "), ai.MaxPromptAnswerChars)
}

// truncateAt bounds a string by bytes without splitting a rune, mirroring the
// prompt layer's own trimming. The bound is a backstop; the prompt trims again
// to whatever it sends.
func truncateAt(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut])
}

// Statuses this flow reports alongside a successful call, mirroring the
// recommendation endpoint so a consumer of one understands the other.
const (
	recommendStatusOK            = "ok"
	recommendStatusNoSuggestions = "no_suggestions"
)

// generateRecommendations asks the model for trees and moves the session to a
// result state.
//
// The reviewed registry is consulted for a verdict and never for permission.
// A visitor whose tradition Everroot has never heard of still receives
// suggestions, labelled with the truth that no reviewer has confirmed that
// particular cultural link. What is refused here is only dishonesty: an
// unconfigured or failing engine is reported as exactly that, never as "no
// trees matched you".
func (m *Manager) generateRecommendations(ctx context.Context, session *Session, userText string) (ConversationResponse, error) {
	resp := ConversationResponse{
		SessionID: session.ID,
		State:     session.State,
		Intent:    session.Intent,
	}
	session.State = StateReadyForDiscovery

	if m.recommender == nil {
		// A deployment without a configured engine is a fact about the
		// deployment, not an absence of trees that suit the visitor. Saying
		// the latter would be a lie they have no way to detect.
		m.log.Error("conversation reached the suggestion step with no recommender configured")
		return m.unavailable(ctx, session, &resp)
	}

	// The verdict is the model's only licence to mention verification. When
	// there is no gate, nothing was checked, and that is reported as
	// "not_queryable" rather than assumed in either direction.
	verdict := AssocNotQueryable
	if m.gate != nil {
		verdict = m.gate.AssociationVerdict(ctx, session.TraditionSlug, session.RegionName, session.Nakshatra)
	}

	id, label := intentionFor(session.Intent)
	result, err := m.recommender.SuggestTrees(ctx, ai.RecommendInput{
		IntentionID:       id,
		IntentionLabel:    label,
		Answer:            answerFor(session, userText),
		Tradition:         session.TraditionSlug,
		Region:            session.RegionName,
		Nakshatra:         session.Nakshatra,
		AssociationStatus: verdict,
		Count:             ai.DefaultSuggestions,
	})
	if err != nil {
		// Only the classification is logged: the request carries the
		// visitor's own words and the reply carries model output, and
		// neither belongs in a log line. Diagnosis.Detail is the one piece
		// built to be credential-free.
		diag := ai.Diagnose(err)
		m.log.Error("suggestion engine failed",
			"category", string(diag.Category),
			"status", diag.Status,
			"retryable", diag.Retryable,
			"detail", diag.Detail,
		)
		return m.unavailable(ctx, session, &resp)
	}

	suggestions := result.Suggestions
	if suggestions == nil {
		suggestions = []ai.Suggestion{}
	}
	payload := &RecommendationResult{
		Status:           recommendStatusOK,
		Suggestions:      suggestions,
		Evidence:         evidenceFor(verdict),
		PromptVersion:    result.PromptVersion,
		Model:            result.Model,
		Disclaimer:       result.Disclaimer,
		FollowUpQuestion: result.FollowUpQuestion,
	}
	resp.Recommendations = payload

	if len(suggestions) == 0 {
		// A model that found nothing says so. This is a successful call with
		// an honest empty answer, deliberately kept apart from the failure
		// above so a broken service is never read as a lack of trees.
		session.State = StateNoSuggestions
		resp.State = StateNoSuggestions
		payload.Status = recommendStatusNoSuggestions
		resp.Message = "I don't have a tree I'd stand behind for that just yet, and I would rather say so than suggest something that doesn't fit. If you tell me a little more about the place or the person, I will gladly try again."
		resp.Suggestions = []string{"Tell you more", "Start over"}
	} else {
		session.State = StateDiscoveryComplete
		resp.State = StateDiscoveryComplete
		resp.Message = m.resultMessage(session, suggestions[0])
		resp.Suggestions = []string{"Explore another option", "Start over"}
	}

	if err := m.saveSession(ctx, session); err != nil {
		return resp, fmt.Errorf("save session: %w", err)
	}
	return resp, nil
}

// unavailable records an honest failure. It is a separate state from
// "no_suggestions" because the two say opposite things about the same
// conversation: one is the model declining, the other is the model never
// being asked.
func (m *Manager) unavailable(ctx context.Context, session *Session, resp *ConversationResponse) (ConversationResponse, error) {
	session.State = StateRecommendationUnavailable
	resp.State = StateRecommendationUnavailable
	resp.Message = "I wasn't able to reach the suggestion engine just now, and I would rather tell you that than invent an answer. Please try again in a moment."
	resp.Suggestions = []string{"Try again", "Start over"}
	if err := m.saveSession(ctx, session); err != nil {
		return *resp, fmt.Errorf("save session: %w", err)
	}
	return *resp, nil
}

// evidenceFor labels each dimension from its own source, so the three kinds of
// statement on screen stay separable: what the model wrote, what a reviewer
// approved, and what an authority verified. A machine-written suggestion is
// never given a reviewed label, whatever the model said.
func evidenceFor(verdict string) EvidenceStatus {
	if verdict == "" {
		verdict = AssocNotQueryable
	}
	return EvidenceStatus{
		CulturalAssociation: verdict,
		BotanicalIdentity:   "unavailable",
		Habitat:             "no_reviewed_record",
		EcologicalRole:      "no_reviewed_record",
		DocumentedStories:   "no_reviewed_record",
		PersonalMeaning:     "user_owned",
	}
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
		SessionID:   session.ID,
		State:       StateGreeting,
		Message:     "What brings you here today?",
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

// resultMessage returns an emotionally meaningful headline naming the tree the
// visitor is actually being offered. It claims no cultural association: the
// headline is about the tree and their intention, and the evidence panel is
// the only place verification is ever stated.
func (m *Manager) resultMessage(session *Session, suggestion ai.Suggestion) string {
	name := suggestion.Name
	switch session.Intent {
	case IntentCelebrate:
		return fmt.Sprintf("A tree for a new beginning — %s", name)
	case IntentRemember:
		return fmt.Sprintf("A living tribute — %s", name)
	case IntentExplore:
		return fmt.Sprintf("A tree worth exploring — %s", name)
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
	if m.extractor == nil {
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
	reply, err := m.extractor.ChatCompletion(ctx, messages)
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
	if m.db == nil {
		id := sessionID
		if id == "" {
			id = fmt.Sprintf("anon-%d", time.Now().UnixNano())
		}
		return &Session{ID: id, State: StateGreeting, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
	}
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
	if m.db == nil {
		return nil
	}
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
	if m.db == nil {
		return nil, sql.ErrNoRows
	}
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
