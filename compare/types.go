package compare

// Options configures compare behavior.
type Options struct {
	Provider string
}

// SummaryItem is one summary category/count entry for compare output.
type SummaryItem struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	// Entries are concrete diagnostics in "path + message" form.
	Entries []string `json:"entries,omitempty"`
}

// ChangeScope identifies the top-level schema object family for a change.
type ChangeScope string

const (
	ScopeResource ChangeScope = "resource"
	ScopeFunction ChangeScope = "function"
	ScopeType     ChangeScope = "type"
	ScopeUnknown  ChangeScope = "unknown"
)

// ChangeSeverity is the normalized severity label used in structured output.
type ChangeSeverity string

const (
	SeverityError ChangeSeverity = "error"
	SeverityWarn  ChangeSeverity = "warn"
	SeverityInfo  ChangeSeverity = "info"
)

// ChangeSource identifies where a change record originated.
type ChangeSource string

const (
	SourceEngine    ChangeSource = "engine"
	SourceNormalize ChangeSource = "normalize"
)

// ImpactRef describes a direct token reference impacted by a type change.
type ImpactRef struct {
	Scope    ChangeScope `json:"scope"`
	Token    string      `json:"token"`
	Location string      `json:"location,omitempty"`
	Path     string      `json:"path,omitempty"`
}

// Change is the canonical compare leaf change record.
type Change struct {
	Scope    ChangeScope    `json:"scope"`
	Token    string         `json:"token"`
	Location string         `json:"location,omitempty"`
	Path     string         `json:"path"`
	Kind     string         `json:"kind"`
	Severity ChangeSeverity `json:"severity"`
	Breaking bool           `json:"breaking"`
	Source   ChangeSource   `json:"source"`
	Message  string         `json:"message,omitempty"`
	// ImpactedBy currently records direct references only.
	// TODO: Evaluate whether transitive references are needed.
	ImpactedBy  []ImpactRef `json:"impactedBy,omitempty"`
	ImpactCount int         `json:"impactCount,omitempty"`
}

// GroupedChanges is a grouped view over canonical leaf changes.
// Keys are token -> location -> leaf changes.
type GroupedChanges struct {
	Resources map[string]map[string][]Change `json:"resources"`
	Functions map[string]map[string][]Change `json:"functions"`
	Types     map[string]map[string][]Change `json:"types"`
}

// Result is the structured output of schema comparison.
type Result struct {
	Summary      []SummaryItem  `json:"summary"`
	Changes      []Change       `json:"changes"`
	Grouped      GroupedChanges `json:"grouped"`
	NewResources []string       `json:"new_resources"`
	NewFunctions []string       `json:"new_functions"`
}
