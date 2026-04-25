package apply

import (
	"strings"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/processbusinessservice"
	"github.com/accept-io/midas/internal/processcapability"
)

func mapCapabilityDocumentToCapability(doc types.CapabilityDocument, now time.Time, createdBy string) *capability.Capability {
	now = now.UTC()
	return &capability.Capability{
		ID:                 strings.TrimSpace(doc.Metadata.ID),
		Name:               strings.TrimSpace(doc.Metadata.Name),
		Description:        strings.TrimSpace(doc.Spec.Description),
		Status:             strings.TrimSpace(doc.Spec.Status),
		Origin:             "manual",
		Managed:            true,
		Owner:              strings.TrimSpace(doc.Spec.Owner),
		ParentCapabilityID: strings.TrimSpace(doc.Spec.ParentCapabilityID),
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          strings.TrimSpace(createdBy),
	}
}

func mapProcessDocumentToProcess(doc types.ProcessDocument, now time.Time, createdBy string) *process.Process {
	now = now.UTC()
	return &process.Process{
		ID:                strings.TrimSpace(doc.Metadata.ID),
		Name:              strings.TrimSpace(doc.Metadata.Name),
		CapabilityID:      strings.TrimSpace(doc.Spec.CapabilityID),
		ParentProcessID:   strings.TrimSpace(doc.Spec.ParentProcessID),
		BusinessServiceID: strings.TrimSpace(doc.Spec.BusinessServiceID),
		Description:       strings.TrimSpace(doc.Spec.Description),
		Status:            strings.TrimSpace(doc.Spec.Status),
		Origin:            "manual",
		Managed:           true,
		Owner:             strings.TrimSpace(doc.Spec.Owner),
		CreatedAt:         now,
		UpdatedAt:         now,
		CreatedBy:         strings.TrimSpace(createdBy),
	}
}

func mapProcessCapabilityDocumentToProcessCapability(doc types.ProcessCapabilityDocument, now time.Time) *processcapability.ProcessCapability {
	return &processcapability.ProcessCapability{
		ProcessID:    strings.TrimSpace(doc.Spec.ProcessID),
		CapabilityID: strings.TrimSpace(doc.Spec.CapabilityID),
		CreatedAt:    now.UTC(),
	}
}

func mapProcessBusinessServiceDocumentToProcessBusinessService(doc types.ProcessBusinessServiceDocument, now time.Time) *processbusinessservice.ProcessBusinessService {
	return &processbusinessservice.ProcessBusinessService{
		ProcessID:         strings.TrimSpace(doc.Spec.ProcessID),
		BusinessServiceID: strings.TrimSpace(doc.Spec.BusinessServiceID),
		CreatedAt:         now.UTC(),
	}
}

func mapBusinessServiceDocumentToBusinessService(doc types.BusinessServiceDocument, now time.Time) *businessservice.BusinessService {
	now = now.UTC()
	return &businessservice.BusinessService{
		ID:              strings.TrimSpace(doc.Metadata.ID),
		Name:            strings.TrimSpace(doc.Metadata.Name),
		Description:     strings.TrimSpace(doc.Spec.Description),
		ServiceType:     businessservice.ServiceType(strings.TrimSpace(doc.Spec.ServiceType)),
		RegulatoryScope: strings.TrimSpace(doc.Spec.RegulatoryScope),
		Status:          strings.TrimSpace(doc.Spec.Status),
		OwnerID:         strings.TrimSpace(doc.Spec.OwnerID),
		Origin:          "manual",
		Managed:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
