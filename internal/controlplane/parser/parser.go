package parser

import (
	"bytes"
	"fmt"
	"io"

	"github.com/accept-io/midas/internal/controlplane/types"
	"gopkg.in/yaml.v3"
)

// ParsedDocument is a generic wrapper around a parsed control-plane resource.
type ParsedDocument struct {
	Kind string         // Surface | Agent | Profile | Grant | Capability | Process
	ID   string         // metadata.id
	Doc  types.Document // typed document
}

func wrapDocument(doc types.Document) ParsedDocument {
	return ParsedDocument{
		Kind: doc.GetKind(),
		ID:   doc.GetID(),
		Doc:  doc,
	}
}

// ParseYAML parses a single YAML document into a typed control-plane document.
func ParseYAML(data []byte) (ParsedDocument, error) {
	var meta struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}

	if err := yaml.Unmarshal(data, &meta); err != nil {
		return ParsedDocument{}, fmt.Errorf("failed to parse YAML header: %w", err)
	}

	if meta.APIVersion == "" {
		return ParsedDocument{}, fmt.Errorf("missing apiVersion")
	}
	if meta.Kind == "" {
		return ParsedDocument{}, fmt.Errorf("missing kind")
	}
	if meta.APIVersion != types.APIVersionV1 {
		return ParsedDocument{}, fmt.Errorf("unsupported apiVersion: %q", meta.APIVersion)
	}

	switch meta.Kind {
	case types.KindSurface:
		var doc types.SurfaceDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse Surface document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindAgent:
		var doc types.AgentDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse Agent document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindProfile:
		var doc types.ProfileDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse Profile document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindGrant:
		var doc types.GrantDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse Grant document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindCapability:
		var doc types.CapabilityDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse Capability document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindProcess:
		var doc types.ProcessDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse Process document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindBusinessService:
		var doc types.BusinessServiceDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse BusinessService document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindProcessCapability:
		var doc types.ProcessCapabilityDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse ProcessCapability document: %w", err)
		}
		return wrapDocument(doc), nil

	case types.KindProcessBusinessService:
		var doc types.ProcessBusinessServiceDocument
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ParsedDocument{}, fmt.Errorf("failed to parse ProcessBusinessService document: %w", err)
		}
		return wrapDocument(doc), nil

	default:
		return ParsedDocument{}, fmt.Errorf("unsupported kind: %q (must be Surface, Agent, Profile, Grant, Capability, Process, BusinessService, ProcessCapability, or ProcessBusinessService)", meta.Kind)
	}
}

// ParseYAMLStream parses multiple YAML documents separated by ---.
func ParseYAMLStream(data []byte) ([]ParsedDocument, error) {
	var docs []ParsedDocument
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	for {
		var node yaml.Node
		if err := decoder.Decode(&node); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to decode YAML document %d: %w", len(docs)+1, err)
		}

		rawBytes, err := yaml.Marshal(&node)
		if err != nil {
			return nil, fmt.Errorf("failed to re-marshal document %d: %w", len(docs)+1, err)
		}

		trimmed := bytes.TrimSpace(rawBytes)
		if len(trimmed) == 0 || string(trimmed) == "null" || string(trimmed) == "{}" {
			continue
		}

		var meta struct {
			APIVersion string `yaml:"apiVersion"`
			Kind       string `yaml:"kind"`
		}
		if err := yaml.Unmarshal(rawBytes, &meta); err != nil {
			return nil, fmt.Errorf("failed to inspect YAML document %d: %w", len(docs)+1, err)
		}

		// Treat comment-only / effectively empty documents as skippable.
		if meta.APIVersion == "" && meta.Kind == "" {
			continue
		}

		doc, err := ParseYAML(rawBytes)
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", len(docs)+1, err)
		}

		docs = append(docs, doc)
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("no YAML documents found")
	}

	return docs, nil
}
