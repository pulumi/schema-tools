package compare

// Severity is the typed severity for a canonical change item.
type Severity string

const (
	SeverityInfo   Severity = "info"
	SeverityWarn   Severity = "warn"
	SeverityDanger Severity = "danger"
)

// ChangeKind identifies the semantic class of a typed change.
type ChangeKind string

const (
	ChangeKindMissingResource    ChangeKind = "missing-resource"
	ChangeKindMissingFunction    ChangeKind = "missing-function"
	ChangeKindMissingType        ChangeKind = "missing-type"
	ChangeKindMissingInput       ChangeKind = "missing-input"
	ChangeKindMissingOutput      ChangeKind = "missing-output"
	ChangeKindMissingProperty    ChangeKind = "missing-property"
	ChangeKindTypeChanged        ChangeKind = "type-changed"
	ChangeKindOptionalToRequired ChangeKind = "optional-to-required"
	ChangeKindRequiredToOptional ChangeKind = "required-to-optional"
	ChangeKindSignatureChanged   ChangeKind = "signature-changed"
	ChangeKindTokenRemapped      ChangeKind = "token-remapped"
	ChangeKindNewResource        ChangeKind = "new-resource"
	ChangeKindNewFunction        ChangeKind = "new-function"
)

// NormalizationReason captures typed token-lookup attribution for one change.
type NormalizationReason struct {
	Outcome    NormalizationOutcome
	Lookup     string
	Token      string
	Candidates []string
}

// NormalizationOutcome identifies whether a lookup found a deterministic mapping.
type NormalizationOutcome string

const (
	NormalizationOutcomeNone      NormalizationOutcome = "none"
	NormalizationOutcomeResolved  NormalizationOutcome = "resolved"
	NormalizationOutcomeAmbiguous NormalizationOutcome = "ambiguous"
)

// Change is the canonical typed compare event emitted by the engine.
type Change struct {
	Category    string
	Name        string
	Path        []string
	Kind        ChangeKind
	Severity    Severity
	Breaking    bool
	Description string
	Reason      *NormalizationReason
}

// Report captures compare engine output.
type Report struct {
	Changes      []Change
	NewResources []string
	NewFunctions []string
}
