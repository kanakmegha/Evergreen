// Package permission implements Everroot's source permission gate.
//
// The gate decides whether a proposed collection or publication operation on a
// source's material may proceed. It relies exclusively on recorded, human-reviewed
// permission evidence stored in the database. It does not consult an LLM for
// legal conclusions.
//
// Decisions are append-only. A human reviewer may override a blocked decision
// only by recording documented legal grounds, supporting evidence, reviewer
// identity, timestamp, and scope. Overrides are never silent.
package permission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Controlled permission states.
const (
	StateAllowedUnderLicence     = "allowed_under_licence"
	StateWrittenPermissionRequired = "written_permission_required"
	StateRestricted              = "restricted"
	StateUnknown                 = "unknown"
	StatePublicDomainVerified    = "public_domain_verified"
)

// Gate decision outcomes.
const (
	DecisionAllowed = "allowed"
	DecisionBlocked = "blocked"
)

// Material types.
const (
	MaterialBotanicalData              = "botanical_data"
	MaterialCulturalClaims             = "cultural_claims"
	MaterialOriginalProse              = "original_prose"
	MaterialCreativeCompilation        = "creative_compilation"
	MaterialTranslations               = "translations"
	MaterialPhotographsIllustrations   = "photographs_illustrations"
	MaterialCommunityTraditionalKnowledge = "community_traditional_knowledge"
)

// Allowed operations.
const (
	OpCollect      = "collect"
	OpStore        = "store"
	OpPublish      = "publish"
	OpTranslate    = "translate"
	OpAdapt        = "adapt"
	OpRedistribute = "redistribute"
	OpCommercial   = "commercial_use"
)

// PermissionRow mirrors one row of source_permission_material_types.
type PermissionRow struct {
	SourceID                        string
	MaterialType                    string
	PermissionState                 string
	LicenceName                     sql.NullString
	LicenceVersion                  sql.NullString
	LicenceURL                      sql.NullString
	LicenceClauses                  sql.NullString
	CommercialUseAllowed            bool
	ExtractionAllowed               bool
	LocalStorageAllowed             bool
	RedistributionAllowed           bool
	AdaptationAllowed               bool
	TranslationAllowed              bool
	AttributionRequired             bool
	AttributionRequirements         sql.NullString
	ShareAlikeRequired              bool
	ThirdPartyRights                sql.NullString
	AutomatedAccessPermitted        bool
	WrittenPermissionRequired       bool
	WrittenPermissionOnFile         bool
	CommunityConsentRequired        bool
	CommunityConsentOnFile          bool
	AllowedOperations               []string
	ReviewedBy                      string
	ReviewedAt                      time.Time
	ReviewNotes                     sql.NullString
	EvidenceURLs                    []string
	NextReviewAt                    sql.NullTime
}

// Decision is the output of Evaluate.
type Decision struct {
	Allowed bool
	Decision string // "allowed" or "blocked"
	Reasons []string
}

// Override records a human override of a blocked decision.
type Override struct {
	SourceID          string
	MaterialType      string
	Operation         string
	LegalGrounds      string
	SupportingEvidence string
	ReviewerID        string
	ReviewerRole      string
	Scope             string
	ExpiresAt         *time.Time
	RequestID         string
}

// Gate evaluates proposed operations against recorded permission evidence.
type Gate struct {
	db   *pgxpool.Pool
	log  *slog.Logger
}

// New creates a permission gate.
func New(db *pgxpool.Pool, log *slog.Logger) *Gate {
	return &Gate{db: db, log: log}
}

// Evaluate returns whether the proposed operation on the given source and
// material type may proceed. It never mutates state; it only reads recorded
// evidence and appends a decision audit row.
func (g *Gate) Evaluate(ctx context.Context, sourceID, materialType, operation string,
	isCommercial, attributionMet, communityConsentMet bool, requestID string) (Decision, error) {

	decision := Decision{Allowed: false, Decision: DecisionBlocked}

	row, err := g.lookupPermission(ctx, sourceID, materialType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			decision.Reasons = append(decision.Reasons, "no permission record on file for source and material type")
			if werr := g.recordDecision(ctx, sourceID, materialType, operation, isCommercial, attributionMet, communityConsentMet, DecisionBlocked, decision.Reasons, requestID); werr != nil {
				g.log.Error("permission gate: failed to record decision", "error", werr)
			}
			return decision, nil
		}
		return decision, fmt.Errorf("lookup permission: %w", err)
	}

	// 1. Permission state must not be unknown or restricted.
	switch row.PermissionState {
	case StateUnknown:
		decision.Reasons = append(decision.Reasons, "permission state is unknown; rights not assessed")
	case StateRestricted:
		decision.Reasons = append(decision.Reasons, "permission state is restricted by rightsholder")
	default:
		// proceed to further checks
	}

	// 2. Written permission.
	if row.WrittenPermissionRequired && !row.WrittenPermissionOnFile {
		decision.Reasons = append(decision.Reasons, "written permission required but not on file")
	}

	// 3. Commercial use.
	if isCommercial && !row.CommercialUseAllowed {
		decision.Reasons = append(decision.Reasons, "commercial use not allowed for this source and material type")
	}

	// 4. Operation within licence scope.
	if !contains(row.AllowedOperations, operation) {
		decision.Reasons = append(decision.Reasons, fmt.Sprintf("operation %q not within recorded allowed operations", operation))
	}

	// Map operation to specific capability checks.
	switch operation {
	case OpCollect:
		if !row.ExtractionAllowed {
			decision.Reasons = append(decision.Reasons, "extraction/collection not allowed")
		}
	case OpStore:
		if !row.LocalStorageAllowed {
			decision.Reasons = append(decision.Reasons, "local storage not allowed")
		}
	case OpPublish:
		if !row.RedistributionAllowed {
			decision.Reasons = append(decision.Reasons, "redistribution/publication not allowed")
		}
	case OpTranslate:
		if !row.TranslationAllowed {
			decision.Reasons = append(decision.Reasons, "translation not allowed")
		}
	case OpAdapt:
		if !row.AdaptationAllowed {
			decision.Reasons = append(decision.Reasons, "adaptation not allowed")
		}
	case OpRedistribute:
		if !row.RedistributionAllowed {
			decision.Reasons = append(decision.Reasons, "redistribution not allowed")
		}
	case OpCommercial:
		if !row.CommercialUseAllowed {
			decision.Reasons = append(decision.Reasons, "commercial use not allowed")
		}
	}

	// 5. Attribution conditions.
	if row.AttributionRequired && !attributionMet {
		decision.Reasons = append(decision.Reasons, "attribution required but not met")
	}

	// 6. Community consent.
	if row.CommunityConsentRequired && !communityConsentMet {
		decision.Reasons = append(decision.Reasons, "community consent required but unresolved")
	}

	// Short-circuit: any unknown/restricted state blocks regardless of other flags.
	if row.PermissionState == StateUnknown || row.PermissionState == StateRestricted {
		decision.Reasons = append(decision.Reasons, fmt.Sprintf("permission state %q blocks collection", row.PermissionState))
	}

	if len(decision.Reasons) > 0 {
		decision.Allowed = false
		decision.Decision = DecisionBlocked
	} else {
		decision.Allowed = true
		decision.Decision = DecisionAllowed
	}

	if err := g.recordDecision(ctx, sourceID, materialType, operation, isCommercial, attributionMet, communityConsentMet, decision.Decision, decision.Reasons, requestID); err != nil {
		g.log.Error("permission gate: failed to record decision", "error", err)
		// Do not fail the evaluation itself if audit recording fails; log and continue.
	}

	return decision, nil
}

// Override records a human override of a blocked gate decision. It requires
// documented legal grounds, supporting evidence, reviewer identity, timestamp,
// and scope. The override is appended (never silent, never in-place).
func (g *Gate) Override(ctx context.Context, o Override) error {
	if strings.TrimSpace(o.LegalGrounds) == "" ||
		strings.TrimSpace(o.SupportingEvidence) == "" ||
		strings.TrimSpace(o.ReviewerID) == "" ||
		strings.TrimSpace(o.Scope) == "" {
		return errors.New("override requires legal grounds, supporting evidence, reviewer id, and scope")
	}
	now := time.Now()
	_, err := g.db.Exec(ctx, `
		INSERT INTO permission_overrides
			(source_id, material_type, operation, legal_grounds, supporting_evidence,
			 reviewer_id, reviewer_role, scope, expires_at, recorded_at, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		o.SourceID, o.MaterialType, o.Operation, o.LegalGrounds, o.SupportingEvidence,
		o.ReviewerID, o.ReviewerRole, o.Scope, o.ExpiresAt, now, o.RequestID,
	)
	if err != nil {
		return fmt.Errorf("record override: %w", err)
	}
	g.log.Info("permission override recorded", "source_id", o.SourceID, "material_type", o.MaterialType, "reviewer_id", o.ReviewerID, "scope", o.Scope)
	return nil
}

// IsOverrideActive checks whether an active, unexpired override exists for the
// given source, material type, and operation.
func (g *Gate) IsOverrideActive(ctx context.Context, sourceID, materialType, operation string) (bool, error) {
	var count int
	err := g.db.QueryRow(ctx, `
		SELECT count(*) FROM permission_overrides
		WHERE source_id=$1 AND material_type=$2 AND operation=$3
		  AND (expires_at IS NULL OR expires_at > now())
	`, sourceID, materialType, operation).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check override: %w", err)
	}
	return count > 0, nil
}

func (g *Gate) lookupPermission(ctx context.Context, sourceID, materialType string) (*PermissionRow, error) {
	row := &PermissionRow{}
	var allowedOps []string
	err := g.db.QueryRow(ctx, `
		SELECT source_id, material_type, permission_state, licence_name, licence_version, licence_url,
			licence_clauses, commercial_use_allowed, extraction_allowed, local_storage_allowed,
			redistribution_allowed, adaptation_allowed, translation_allowed, attribution_required,
			attribution_requirements, share_alike_required, third_party_rights,
			automated_access_permitted, written_permission_required, written_permission_on_file,
			community_consent_required, community_consent_on_file, allowed_operations,
			reviewed_by, reviewed_at, review_notes, evidence_urls, next_review_at
		FROM source_permission_material_types
		WHERE source_id=$1 AND material_type=$2
	`, sourceID, materialType).Scan(
		&row.SourceID, &row.MaterialType, &row.PermissionState,
		&row.LicenceName, &row.LicenceVersion, &row.LicenceURL, &row.LicenceClauses,
		&row.CommercialUseAllowed, &row.ExtractionAllowed, &row.LocalStorageAllowed,
		&row.RedistributionAllowed, &row.AdaptationAllowed, &row.TranslationAllowed,
		&row.AttributionRequired, &row.AttributionRequirements, &row.ShareAlikeRequired,
		&row.ThirdPartyRights, &row.AutomatedAccessPermitted,
		&row.WrittenPermissionRequired, &row.WrittenPermissionOnFile,
		&row.CommunityConsentRequired, &row.CommunityConsentOnFile, &allowedOps,
		&row.ReviewedBy, &row.ReviewedAt, &row.ReviewNotes,
		&row.EvidenceURLs, &row.NextReviewAt,
	)
	if err != nil {
		return nil, err
	}
	if len(allowedOps) > 0 {
		row.AllowedOperations = allowedOps
	}
	return row, nil
}

func (g *Gate) recordDecision(ctx context.Context, sourceID, materialType, operation string,
	isCommercial, attributionMet, communityConsentMet bool, decision string,
	reasons []string, requestID string) error {

	_, err := g.db.Exec(ctx, `
		INSERT INTO permission_gate_decisions
			(source_id, material_type, operation, is_commercial, attribution_met,
			 community_consent_met, decision, reasons, decided_by, decided_at, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'system',now(),$9)`,
		sourceID, materialType, operation, isCommercial, attributionMet,
		communityConsentMet, decision, reasons, requestID,
	)
	return err
}

func contains(s []string, e string) bool {
	for _, x := range s {
		if x == e {
			return true
		}
	}
	return false
}
