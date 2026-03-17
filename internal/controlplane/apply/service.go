package apply

import (
	"context"
	"fmt"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
)

// Service coordinates control-plane apply operations.
type Service struct{}

// NewService constructs a new apply service.
func NewService() *Service {
	return &Service{}
}

// NewServiceWithRepo is kept temporarily for backward compatibility.
// The repository is not used in the current validation-only implementation.
func NewServiceWithRepo(_ any) *Service {
	return &Service{}
}

// Apply validates a parsed bundle and returns an ApplyResult.
//
// Current behavior:
// - if validation fails, return validation errors only
// - if validation passes, mark each resource as created
// - no persistence is performed yet
func (s *Service) Apply(ctx context.Context, docs []parser.ParsedDocument) types.ApplyResult {
	_ = ctx // reserved for future persistence, cancellation, and tracing

	var result types.ApplyResult

	validationErrs := validate.ValidateBundle(docs)
	if len(validationErrs) > 0 {
		result.ValidationErrors = append(result.ValidationErrors, validationErrs...)
		return result
	}

	for _, doc := range docs {
		result.AddCreated(doc.Kind, doc.ID)
	}

	return result
}

// ApplyBundle parses a raw YAML bundle and applies it through the existing
// validation/apply pipeline.
//
// Behavior:
// - parse failures return an error
// - successfully parsed bundles always return an ApplyResult
// - validation failures are represented inside ApplyResult, not as an error
func (s *Service) ApplyBundle(ctx context.Context, yamlBytes []byte) (*types.ApplyResult, error) {
	docs, err := parser.ParseYAMLStream(yamlBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML bundle: %w", err)
	}

	result := s.Apply(ctx, docs)
	return &result, nil
}
