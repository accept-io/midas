package apply

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
	"github.com/accept-io/midas/internal/surface"
)

// Service coordinates control-plane apply operations.
type Service struct {
	surfaceRepo surface.SurfaceRepository
}

// NewService constructs a new apply service with no repositories configured.
// In this mode, apply remains validation-only and marks valid resources as created.
func NewService() *Service {
	return &Service{}
}

// NewServiceWithRepo constructs a new apply service with a surface repository.
// This is kept for backward compatibility with existing tests/callers.
func NewServiceWithRepo(surfaceRepo surface.SurfaceRepository) *Service {
	return &Service{
		surfaceRepo: surfaceRepo,
	}
}

// Apply validates a parsed bundle and applies it.
//
// Current behavior:
//   - if validation fails, return validation errors only
//   - if validation passes and no repositories are configured, mark each resource as created
//   - if a surface repository is configured, persist Surface resources as governed review-state versions
//   - non-surface resources remain validation-only for now
func (s *Service) Apply(ctx context.Context, docs []parser.ParsedDocument) types.ApplyResult {
	var result types.ApplyResult

	validationErrs := validate.ValidateBundle(docs)
	if len(validationErrs) > 0 {
		result.ValidationErrors = append(result.ValidationErrors, validationErrs...)
		return result
	}

	// Validation-only mode: preserve existing milestone behavior.
	if s.surfaceRepo == nil {
		for _, doc := range docs {
			result.AddCreated(doc.Kind, doc.ID)
		}
		return result
	}

	now := time.Now().UTC()
	createdBy := "system" // TODO: derive from auth/context when available.

	for _, doc := range docs {
		switch doc.Kind {
		case types.KindSurface:
			if err := s.applySurface(ctx, doc, now, createdBy, &result); err != nil {
				result.AddError(doc.Kind, doc.ID, err.Error())
			}
		default:
			// Preserve existing behavior for non-surface resources until their
			// persistence-backed apply paths are implemented.
			result.AddCreated(doc.Kind, doc.ID)
		}
	}

	return result
}

// applySurface creates a governed surface version in review state.
// New applies always create a new versioned record; they do not update in place.
func (s *Service) applySurface(
	ctx context.Context,
	doc parser.ParsedDocument,
	now time.Time,
	createdBy string,
	result *types.ApplyResult,
) error {
	surfaceDoc, ok := doc.Doc.(types.SurfaceDocument)
	if !ok {
		return fmt.Errorf("invalid document payload for kind %q", types.KindSurface)
	}

	latest, err := s.surfaceRepo.FindLatestByID(ctx, surfaceDoc.Metadata.ID)
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("find latest surface by id: %w", err)
	}

	version := 1
	if latest != nil {
		version = latest.Version + 1
	}

	ds, err := mapSurfaceDocumentToDecisionSurface(surfaceDoc, now, createdBy, version)
	if err != nil {
		return fmt.Errorf("map surface document: %w", err)
	}

	if err := s.surfaceRepo.Create(ctx, ds); err != nil {
		return fmt.Errorf("create surface version: %w", err)
	}

	// A new version is always a creation event in the apply result.
	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// ApplyBundle parses a raw YAML bundle and applies it through the existing
// validation/apply pipeline.
//
// Behavior:
//   - parse failures return an error
//   - successfully parsed bundles always return an ApplyResult
//   - validation failures are represented inside ApplyResult, not as an error
func (s *Service) ApplyBundle(ctx context.Context, yamlBytes []byte) (*types.ApplyResult, error) {
	docs, err := parser.ParseYAMLStream(yamlBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML bundle: %w", err)
	}

	result := s.Apply(ctx, docs)
	return &result, nil
}

// isNotFoundError is a temporary compatibility helper until repository-layer
// not-found errors are standardized as typed sentinels.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return msg == "not found" ||
		msg == "surface not found" ||
		strings.Contains(msg, "not found")
}
