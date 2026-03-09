package audit

import "context"

type AuditEventRepository interface {
	Append(ctx context.Context, ev *AuditEvent) error
	ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*AuditEvent, error)
	ListByRequestID(ctx context.Context, requestID string) ([]*AuditEvent, error)
}
