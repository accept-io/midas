package aisystem

import "errors"

// Sentinel errors for the AI System Registration substrate.
// Repository implementations return these where the contract calls
// for a "not found", "duplicate", or invariant-violation signal.
// Wrap with fmt.Errorf("...: %w", err) when adding context.
var (
	ErrAISystemNotFound      = errors.New("ai system not found")
	ErrAISystemAlreadyExists = errors.New("ai system already exists")

	ErrAISystemVersionNotFound      = errors.New("ai system version not found")
	ErrAISystemVersionAlreadyExists = errors.New("ai system version already exists")

	ErrAISystemBindingNotFound      = errors.New("ai system binding not found")
	ErrAISystemBindingAlreadyExists = errors.New("ai system binding already exists")

	ErrInvalidStatus         = errors.New("invalid status")
	ErrInvalidOrigin         = errors.New("invalid origin")
	ErrInvalidVersion        = errors.New("invalid version")
	ErrInvalidEffectiveRange = errors.New("effective_until must be after effective_from")
	ErrSelfReplace           = errors.New("ai system cannot replace itself")
	ErrBindingMissingContext = errors.New("binding requires at least one of business_service_id, capability_id, process_id, surface_id")
)
