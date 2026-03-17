package controlplane

import (
	"context"
	"fmt"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// Service provides the top-level control-plane workflow for MIDAS.
//
// It is the orchestration façade over parsing and apply. Callers provide either
// raw YAML or pre-parsed documents and receive a structured ApplyResult.
type Service struct {
	applier *apply.Service
}

// NewService constructs a control-plane service using the default apply behavior.
func NewService() *Service {
	return &Service{
		applier: apply.NewService(),
	}
}

// NewServiceWithRepository is kept temporarily for backward compatibility.
// The repository is not used in the current validation-only implementation.
func NewServiceWithRepository(_ any) *Service {
	return &Service{
		applier: apply.NewServiceWithRepo(nil),
	}
}

// ApplyYAML parses a YAML bundle and applies it through the control plane.
func (s *Service) ApplyYAML(ctx context.Context, data []byte) types.ApplyResult {
	docs, err := parser.ParseYAMLStream(data)
	if err != nil {
		var result types.ApplyResult
		result.AddValidationError("", "", fmt.Sprintf("YAML parse error: %v", err))
		return result
	}

	return s.applier.Apply(ctx, docs)
}

// ApplyDocuments applies an already-parsed bundle of control-plane documents.
func (s *Service) ApplyDocuments(ctx context.Context, docs []parser.ParsedDocument) types.ApplyResult {
	return s.applier.Apply(ctx, docs)
}
