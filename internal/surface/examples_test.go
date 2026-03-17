package surface_test

import (
	"time"

	"github.com/accept-io/midas/internal/surface"
)

// NewLendingSecuredOrigination creates a sample secured lending origination surface.
// High-stakes financial decision surface with multiple consequence types.
func NewLendingSecuredOrigination() *surface.DecisionSurface {
	ptrF := func(f float64) *float64 { return &f }

	return &surface.DecisionSurface{
		// Identity
		ID:      "lending-secured-origination",
		Version: 1,
		Name:    "Secured Lending Origination",
		Description: "Approve or deny secured loan applications including auto loans, " +
			"mortgages, and equipment financing. Governed decisions include credit approval, " +
			"interest rate determination, and collateral valuation acceptance.",

		// Domain Classification
		Domain:   "financial_services",
		Category: "lending",
		Taxonomy: []string{"financial", "lending", "secured", "origination"},
		Tags:     []string{"regulated", "high-value", "credit-risk"},

		// Decision Characteristics
		DecisionType:       surface.DecisionTypeStrategic,
		ReversibilityClass: surface.ReversibilityConditionallyReversible,

		// Context Schema - What must be provided to make this decision
		RequiredContext: surface.ContextSchema{
			Fields: []surface.ContextField{
				{
					Name:        "loan_amount",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Requested loan principal in USD",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(1000.0),
						Maximum: ptrF(10000000.0),
					},
					Example: 250000.0,
				},
				{
					Name:        "applicant_credit_score",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "FICO credit score of primary applicant",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(300.0),
						Maximum: ptrF(850.0),
					},
					Example: 720.0,
				},
				{
					Name:        "collateral_type",
					Type:        surface.FieldTypeString,
					Required:    true,
					Description: "Type of collateral securing the loan",
					Validation: &surface.ValidationRule{
						Enum: []string{"auto", "real_estate", "equipment", "securities"},
					},
					Example: "real_estate",
				},
				{
					Name:        "debt_to_income_ratio",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Applicant's debt-to-income ratio (0.0 to 1.0)",
					Validation: &surface.ValidationRule{
						Minimum:          ptrF(0.0),
						Maximum:          ptrF(1.0),
						ExclusiveMaximum: false,
					},
					Example: 0.35,
				},
				{
					Name:        "loan_to_value_ratio",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Loan amount as ratio of collateral value",
					Validation: &surface.ValidationRule{
						Minimum:          ptrF(0.0),
						Maximum:          ptrF(1.0),
						ExclusiveMaximum: false,
					},
					Example: 0.80,
				},
				{
					Name:        "applicant_employment_status",
					Type:        surface.FieldTypeString,
					Required:    true,
					Description: "Current employment status",
					Validation: &surface.ValidationRule{
						Enum: []string{"full_time", "part_time", "self_employed", "unemployed", "retired"},
					},
					Example: "full_time",
				},
			},
		},

		// Consequence Types - What impact can this decision have?
		ConsequenceTypes: []surface.ConsequenceType{
			{
				ID:          "financial_exposure",
				Name:        "Financial Exposure",
				Description: "Total dollar amount of credit extended",
				MeasureType: surface.MeasureTypeFinancial,
				Currency:    "USD",
				MinValue:    ptrF(0.0),
				MaxValue:    ptrF(10000000.0),
			},
			{
				ID:           "commitment_duration",
				Name:         "Commitment Duration",
				Description:  "Length of loan term in months",
				MeasureType:  surface.MeasureTypeTemporal,
				DurationUnit: surface.DurationUnitMonths,
				MinValue:     ptrF(1.0),
				MaxValue:     ptrF(360.0), // 30 years
			},
			{
				ID:          "credit_risk_rating",
				Name:        "Credit Risk Rating",
				Description: "Assessed credit risk level for this loan",
				MeasureType: surface.MeasureTypeRiskRating,
				RatingScale: []string{"minimal", "low", "moderate", "elevated", "high", "critical"},
			},
		},

		// Governance Requirements
		MinimumConfidence: 0.85, // High bar for automated lending decisions

		MandatoryEvidence: []surface.EvidenceRequirement{
			{
				ID:          "credit_report",
				Name:        "Credit Report",
				Description: "Full credit bureau report from approved provider",
				Required:    true,
				Format:      "document_reference",
			},
			{
				ID:          "income_verification",
				Name:        "Income Verification",
				Description: "Pay stubs, tax returns, or bank statements",
				Required:    true,
				Format:      "document_reference",
			},
			{
				ID:          "collateral_appraisal",
				Name:        "Collateral Appraisal",
				Description: "Independent appraisal of collateral value",
				Required:    true,
				Format:      "document_reference",
			},
		},

		// Policy Integration
		PolicyPackage: "lending.secured.origination",
		PolicyVersion: "v1.2.0",
		FailureMode:   surface.FailureModeClosed, // Fail-safe: escalate on policy errors

		// Observability & Compliance
		AuditRetentionHours:  87600, // 10 years (regulatory requirement)
		SubjectRequired:      true,  // Every loan has an identified subject (applicant)
		ComplianceFrameworks: []string{"FCRA", "ECOA", "TILA", "SOX"},

		// Lifecycle
		Status:        surface.SurfaceStatusActive,
		EffectiveFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),

		// Ownership
		BusinessOwner:  "lending-products@example.com",
		TechnicalOwner: "platform-engineering@example.com",
		Stakeholders:   []string{"risk-management@example.com", "compliance@example.com"},

		DocumentationURL: "https://docs.example.com/surfaces/lending-secured-origination",
		ExternalReferences: map[string]string{
			"jira":       "GOVERN-1001",
			"confluence": "https://wiki.example.com/lending",
			"runbook":    "https://runbooks.example.com/lending-escalations",
		},

		CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 12, 15, 0, 0, 0, 0, time.UTC),
		CreatedBy: "architect@example.com",
	}
}

// NewPaymentsInstantExecution creates a sample instant payment execution surface.
// High-frequency, irreversible financial transaction surface.
func NewPaymentsInstantExecution() *surface.DecisionSurface {
	ptrF := func(f float64) *float64 { return &f }

	return &surface.DecisionSurface{
		// Identity
		ID:      "payments-instant-execution",
		Version: 1,
		Name:    "Instant Payment Execution",
		Description: "Execute real-time payment transfers including P2P, bill payments, " +
			"and merchant settlements. Irreversible once completed.",

		// Domain Classification
		Domain:   "financial_services",
		Category: "payments",
		Taxonomy: []string{"financial", "payments", "instant", "execution"},
		Tags:     []string{"real-time", "irreversible", "fraud-risk"},

		// Decision Characteristics
		DecisionType:       surface.DecisionTypeOperational,
		ReversibilityClass: surface.ReversibilityIrreversible, // Cannot undo once sent

		// Context Schema
		RequiredContext: surface.ContextSchema{
			Fields: []surface.ContextField{
				{
					Name:        "amount",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Payment amount in USD",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(0.01),
						Maximum: ptrF(100000.0),
					},
					Example: 1250.50,
				},
				{
					Name:        "sender_account_age_days",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Days since sender account was opened",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(0.0),
					},
					Example: 365.0,
				},
				{
					Name:        "recipient_is_new",
					Type:        surface.FieldTypeBoolean,
					Required:    true,
					Description: "True if recipient is new to sender (fraud signal)",
					Example:     false,
				},
				{
					Name:        "velocity_24h_count",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Number of payments from this account in last 24 hours",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(0.0),
					},
					Example: 3.0,
				},
				{
					Name:        "velocity_24h_amount",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Total payment volume from this account in last 24 hours",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(0.0),
					},
					Example: 5000.0,
				},
				{
					Name:        "device_fingerprint_known",
					Type:        surface.FieldTypeBoolean,
					Required:    true,
					Description: "True if device fingerprint has been seen before",
					Example:     true,
				},
			},
		},

		// Consequence Types
		ConsequenceTypes: []surface.ConsequenceType{
			{
				ID:          "financial_amount",
				Name:        "Payment Amount",
				Description: "Dollar amount being transferred",
				MeasureType: surface.MeasureTypeFinancial,
				Currency:    "USD",
				MinValue:    ptrF(0.0),
				MaxValue:    ptrF(100000.0),
			},
			{
				ID:          "fraud_risk",
				Name:        "Fraud Risk Level",
				Description: "Assessed fraud probability for this transaction",
				MeasureType: surface.MeasureTypeRiskRating,
				RatingScale: []string{"minimal", "low", "medium", "high", "critical"},
			},
			{
				ID:          "impact_scope",
				Name:        "Customer Impact Scope",
				Description: "Breadth of customer impact if fraudulent",
				MeasureType: surface.MeasureTypeImpactScope,
				ScopeScale:  []string{"individual", "household", "business", "institutional"},
			},
		},

		// Governance Requirements
		MinimumConfidence: 0.95, // Very high bar due to irreversibility

		MandatoryEvidence: []surface.EvidenceRequirement{
			{
				ID:          "fraud_score",
				Name:        "Fraud Detection Score",
				Description: "Real-time fraud model output",
				Required:    true,
				Format:      "structured_data",
			},
		},

		// Policy Integration
		PolicyPackage: "payments.fraud.detection",
		PolicyVersion: "v2.1.0",
		FailureMode:   surface.FailureModeClosed, // Fail-safe on policy errors

		// Observability & Compliance
		AuditRetentionHours:  43800, // 5 years (payments regulation)
		SubjectRequired:      true,  // Every payment has sender/recipient subjects
		ComplianceFrameworks: []string{"PCI-DSS", "NACHA", "OFAC"},

		// Lifecycle
		Status:        surface.SurfaceStatusActive,
		EffectiveFrom: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),

		// Ownership
		BusinessOwner:  "payments-product@example.com",
		TechnicalOwner: "payments-engineering@example.com",
		Stakeholders:   []string{"fraud-ops@example.com", "compliance@example.com"},

		DocumentationURL: "https://docs.example.com/surfaces/payments-instant",
		ExternalReferences: map[string]string{
			"pagerduty": "PAYMENTS-INSTANT",
			"datadog":   "payments.instant.execution",
		},

		CreatedAt: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
		CreatedBy: "payments-lead@example.com",
	}
}

// NewFraudAccountSuspension creates a sample fraud account suspension surface.
// Tactical security decision with customer impact.
func NewFraudAccountSuspension() *surface.DecisionSurface {
	ptrF := func(f float64) *float64 { return &f }

	return &surface.DecisionSurface{
		// Identity
		ID:      "fraud-account-suspension",
		Version: 1,
		Name:    "Fraud Account Suspension",
		Description: "Temporarily suspend user accounts suspected of fraudulent activity " +
			"to prevent further unauthorized transactions while investigation proceeds.",

		// Domain Classification
		Domain:   "security",
		Category: "fraud_prevention",
		Taxonomy: []string{"security", "fraud", "account_management"},
		Tags:     []string{"fraud", "customer-impact", "reversible"},

		// Decision Characteristics
		DecisionType:       surface.DecisionTypeTactical,
		ReversibilityClass: surface.ReversibilityReversible, // Can be unsuspended

		// Context Schema
		RequiredContext: surface.ContextSchema{
			Fields: []surface.ContextField{
				{
					Name:        "fraud_score",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Fraud detection model score (0.0 to 1.0)",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(0.0),
						Maximum: ptrF(1.0),
					},
					Example: 0.87,
				},
				{
					Name:        "suspicious_behaviors",
					Type:        surface.FieldTypeArray,
					Required:    true,
					Description: "List of detected suspicious behavior patterns",
					Validation: &surface.ValidationRule{
						MinItems: func() *int { i := 1; return &i }(),
					},
					Example: []string{"velocity_spike", "unusual_location", "credential_stuffing"},
				},
				{
					Name:        "account_age_days",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Days since account was created",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(0.0),
					},
					Example: 180.0,
				},
				{
					Name:        "account_value_usd",
					Type:        surface.FieldTypeNumber,
					Required:    true,
					Description: "Estimated account value in USD (balance + assets)",
					Validation: &surface.ValidationRule{
						Minimum: ptrF(0.0),
					},
					Example: 5000.0,
				},
			},
		},

		// Consequence Types
		ConsequenceTypes: []surface.ConsequenceType{
			{
				ID:           "suspension_duration",
				Name:         "Suspension Duration",
				Description:  "Expected suspension period in hours",
				MeasureType:  surface.MeasureTypeTemporal,
				DurationUnit: surface.DurationUnitHours,
				MinValue:     ptrF(1.0),
				MaxValue:     ptrF(720.0), // Max 30 days
			},
			{
				ID:          "customer_impact",
				Name:        "Customer Impact Level",
				Description: "Severity of service disruption to customer",
				MeasureType: surface.MeasureTypeRiskRating,
				RatingScale: []string{"minimal", "moderate", "significant", "severe"},
			},
			{
				ID:          "false_positive_risk",
				Name:        "False Positive Risk",
				Description: "Risk this is a legitimate account being incorrectly flagged",
				MeasureType: surface.MeasureTypeRiskRating,
				RatingScale: []string{"very_low", "low", "medium", "high"},
			},
		},

		// Governance Requirements
		MinimumConfidence: 0.75, // Moderate bar (reversible action, but customer impact)

		MandatoryEvidence: []surface.EvidenceRequirement{
			{
				ID:          "fraud_signals",
				Name:        "Fraud Detection Signals",
				Description: "Structured log of all detected fraud indicators",
				Required:    true,
				Format:      "structured_data",
			},
		},

		// Policy Integration
		PolicyPackage: "fraud.suspension.rules",
		PolicyVersion: "v1.0.0",
		FailureMode:   surface.FailureModeOpen, // Fail-open: manual review if policy fails

		// Observability & Compliance
		AuditRetentionHours:  26280, // 3 years
		SubjectRequired:      true,  // Every suspension targets a specific account
		ComplianceFrameworks: []string{"SOC2", "ISO27001"},

		// Lifecycle
		Status:        surface.SurfaceStatusActive,
		EffectiveFrom: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),

		// Ownership
		BusinessOwner:  "fraud-ops@example.com",
		TechnicalOwner: "security-engineering@example.com",
		Stakeholders:   []string{"customer-support@example.com", "legal@example.com"},

		DocumentationURL: "https://docs.example.com/surfaces/fraud-suspension",
		ExternalReferences: map[string]string{
			"playbook": "https://runbooks.example.com/fraud-response",
			"sla":      "https://wiki.example.com/fraud-sla",
		},

		CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
		CreatedBy: "fraud-lead@example.com",
	}
}
